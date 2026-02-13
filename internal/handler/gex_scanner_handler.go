package handler

import (
	"html/template"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/arnabmitra/eth-proxy/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GEXScannerHandler struct {
	logger         *slog.Logger
	tmpl           *template.Template
	repo           *repository.Queries
	allowedSymbols map[string]bool
}

func NewGEXScannerHandler(logger *slog.Logger, tmpl *template.Template, db *pgxpool.Pool, allowedSymbols []string) *GEXScannerHandler {
	allowed := make(map[string]bool)
	for _, s := range allowedSymbols {
		allowed[s] = true
	}
	return &GEXScannerHandler{
		logger:         logger,
		tmpl:           tmpl,
		repo:           repository.New(db),
		allowedSymbols: allowed,
	}
}

type GEXScanItem struct {
	Symbol       string
	CurrentGEX   float64
	PreviousGEX  float64
	GEXChange    float64
	GEXChangePct float64
	CurrentPrice float64
	ExpiryDate   string
	Direction    string // "up" or "down"
	ZScore       float64
}

func (h *GEXScannerHandler) HandleGEXScanner(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()
	loc, _ := time.LoadLocation("America/New_York")
	nowInET := now.In(loc)

	sortParam := r.URL.Query().Get("sort")

	var items []GEXScanItem
	var err error

	// Market hours: 9:30 AM to 4:00 PM ET
	marketOpen := time.Date(nowInET.Year(), nowInET.Month(), nowInET.Day(), 9, 30, 0, 0, loc)
	marketClose := time.Date(nowInET.Year(), nowInET.Month(), nowInET.Day(), 16, 0, 0, 0, loc)

	// Check if today is a weekday
	isWeekday := nowInET.Weekday() >= time.Monday && nowInET.Weekday() <= time.Friday

	if isWeekday && nowInET.After(marketOpen) && nowInET.Before(marketClose) {
		// Market is open, get GEX changes
		currentWindowStart := now.Add(-30 * time.Minute)
		previousWindowStart := now.Add(-60 * time.Minute)

		results, err_ := h.repo.GetGEXChangeForSymbols(ctx, repository.GetGEXChangeForSymbolsParams{
			RecordedAt:   currentWindowStart,
			RecordedAt_2: previousWindowStart,
		})
		err = err_
		if err == nil {
			items = h.processGEXChangeResults(results)
		}

		// If no items found during market hours (e.g. just opened or delay), fall back to latest changes
		if len(items) == 0 {
			results, err_ := h.repo.GetLatestGEXChanges(ctx)
			err = err_
			if err == nil {
				items = h.processLatestGEXChangesResults(results)
			}
		}
	} else {
		// Market is closed, get the last known GEX changes
		results, err_ := h.repo.GetLatestGEXChanges(ctx)
		err = err_
		if err == nil {
			items = h.processLatestGEXChangesResults(results)
		}
	}

	if err != nil {
		h.logger.Error("failed to get GEX data", "error", err)
		http.Error(w, "Failed to load GEX scanner data", http.StatusInternalServerError)
		return
	}

	// Fetch anomalies/z-scores
	anomalies, err := h.repo.GetGEXAnomalies(ctx)
	if err != nil {
		h.logger.Error("failed to fetch GEX anomalies", "error", err)
	} else {
		// Create a map for fast lookup
		zScoreMap := make(map[string]float64)
		for _, anomaly := range anomalies {
			z, _ := anomaly.ZScore.Float64Value()
			zScoreMap[anomaly.Symbol] = z.Float64
		}

		// Enhance items with ZScore
		for i := range items {
			if z, ok := zScoreMap[items[i].Symbol]; ok {
				items[i].ZScore = z
			}
		}
	}

	// Sort items
	if sortParam == "gex_asc" {
		sort.Slice(items, func(i, j int) bool {
			return items[i].CurrentGEX < items[j].CurrentGEX
		})
	} else if sortParam == "gex_desc" {
		sort.Slice(items, func(i, j int) bool {
			return items[i].CurrentGEX > items[j].CurrentGEX
		})
	} else if sortParam == "zscore_abs_desc" {
		sort.Slice(items, func(i, j int) bool {
			return math.Abs(items[i].ZScore) > math.Abs(items[j].ZScore)
		})
	}

	data := map[string]interface{}{
		"Items":       items,
		"LastUpdated": now.Format("Jan 02, 2006 3:04 PM MST"),
		"Sort":        sortParam,
	}

	if r.Header.Get("HX-Request") == "true" {
		err = h.tmpl.ExecuteTemplate(w, "gex_scanner_table.html", data)
	} else {
		err = h.tmpl.ExecuteTemplate(w, "gex_scanner.html", data)
	}

	if err != nil {
		h.logger.Error("failed to render template", "error", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

func (h *GEXScannerHandler) processGEXChangeResults(results []repository.GetGEXChangeForSymbolsRow) []GEXScanItem {
	items := make([]GEXScanItem, 0, len(results))
	for _, result := range results {
		if !h.allowedSymbols[result.Symbol] {
			continue
		}

		currentGEX, _ := result.CurrentGex.Float64Value()
		previousGEX, _ := result.PreviousGex.Float64Value()
		gexChange, _ := result.GexChange.Float64Value()
		gexChangePct, _ := result.GexChangePct.Float64Value()

		direction := "neutral"
		if gexChange.Float64 > 0 {
			direction = "up"
		} else if gexChange.Float64 < 0 {
			direction = "down"
		}

		currentPrice := 0.0
		if result.CurrentPrice.Valid {
			if price, err := strconv.ParseFloat(result.CurrentPrice.String, 64); err == nil {
				currentPrice = price
			}
		}

		expiryDate := ""
		if result.ExpiryDate.Valid {
			expiryDate = result.ExpiryDate.Time.Format("2006-01-02")
		}

		items = append(items, GEXScanItem{
			Symbol:       result.Symbol,
			CurrentGEX:   currentGEX.Float64,
			PreviousGEX:  previousGEX.Float64,
			GEXChange:    gexChange.Float64,
			GEXChangePct: gexChangePct.Float64,
			CurrentPrice: currentPrice,
			ExpiryDate:   expiryDate,
			Direction:    direction,
		})
	}
	return items
}

func (h *GEXScannerHandler) processLatestGEXChangesResults(results []repository.GetLatestGEXChangesRow) []GEXScanItem {
	items := make([]GEXScanItem, 0, len(results))
	for _, result := range results {
		if !h.allowedSymbols[result.Symbol] {
			continue
		}

		currentGEX, _ := result.CurrentGex.Float64Value()
		previousGEX, _ := result.PreviousGex.Float64Value()
		gexChange, _ := result.GexChange.Float64Value()
		gexChangePct, _ := result.GexChangePct.Float64Value()

		direction := "neutral"
		if gexChange.Float64 > 0 {
			direction = "up"
		} else if gexChange.Float64 < 0 {
			direction = "down"
		}

		currentPrice := 0.0
		if result.CurrentPrice.Valid {
			if price, err := strconv.ParseFloat(result.CurrentPrice.String, 64); err == nil {
				currentPrice = price
			}
		}

		expiryDate := ""
		if result.ExpiryDate.Valid {
			expiryDate = result.ExpiryDate.Time.Format("2006-01-02")
		}

		items = append(items, GEXScanItem{
			Symbol:       result.Symbol,
			CurrentGEX:   currentGEX.Float64,
			PreviousGEX:  previousGEX.Float64,
			GEXChange:    gexChange.Float64,
			GEXChangePct: gexChangePct.Float64,
			CurrentPrice: currentPrice,
			ExpiryDate:   expiryDate,
			Direction:    direction,
		})
	}
	return items
}
