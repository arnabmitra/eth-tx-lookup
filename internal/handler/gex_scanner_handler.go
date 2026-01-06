package handler

import (
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/arnabmitra/eth-proxy/internal/repository"
	"github.com/dustin/go-humanize"
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
	Symbol         string
	CurrentGEX     float64
	CurrentGEXFmt  string
	PreviousGEX    float64
	PreviousGEXFmt string
	GEXChange      float64
	GEXChangeFmt   string
	GEXChangePct   float64
	CurrentPrice   float64
	ExpiryDate     string
	Direction      string // "up" or "down"
}

func (h *GEXScannerHandler) HandleGEXScanner(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get GEX changes comparing last hour to previous hour
	now := time.Now()
	currentWindowStart := now.Add(-1 * time.Hour)
	previousWindowStart := now.Add(-2 * time.Hour)

	results, err := h.repo.GetGEXChangeForSymbols(ctx, repository.GetGEXChangeForSymbolsParams{
		RecordedAt:   currentWindowStart,
		RecordedAt_2: previousWindowStart,
	})

	if err != nil {
		h.logger.Error("failed to get GEX changes", "error", err)
		http.Error(w, "Failed to load GEX scanner data", http.StatusInternalServerError)
		return
	}

	// Convert to display items
	items := make([]GEXScanItem, 0, len(results))
	for _, result := range results {
		// Filter out symbols not in our allowed list
		if !h.allowedSymbols[result.Symbol] {
			continue
		}

		currentGEX := 0.0
		if result.CurrentGex.Valid {
			if val, err := result.CurrentGex.Float64Value(); err == nil && val.Valid {
				currentGEX = val.Float64
			}
		}

		previousGEX := 0.0
		if result.PreviousGex.Valid {
			if val, err := result.PreviousGex.Float64Value(); err == nil && val.Valid {
				previousGEX = val.Float64
			}
		}

		gexChange := 0.0
		if result.GexChange.Valid {
			if val, err := result.GexChange.Float64Value(); err == nil && val.Valid {
				gexChange = val.Float64
			}
		}

		gexChangePct := 0.0
		if result.GexChangePct.Valid {
			if val, err := result.GexChangePct.Float64Value(); err == nil && val.Valid {
				gexChangePct = val.Float64
			}
		}

		direction := "neutral"
		if gexChange > 0 {
			direction = "up"
		} else if gexChange < 0 {
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
			Symbol:         result.Symbol,
			CurrentGEX:     currentGEX,
			CurrentGEXFmt:  humanize.CommafWithDigits(currentGEX, 0),
			PreviousGEX:    previousGEX,
			PreviousGEXFmt: humanize.CommafWithDigits(previousGEX, 0),
			GEXChange:      gexChange,
			GEXChangeFmt:   humanize.CommafWithDigits(gexChange, 0),
			GEXChangePct:   gexChangePct,
			CurrentPrice:   currentPrice,
			ExpiryDate:     expiryDate,
			Direction:      direction,
		})
	}

	data := map[string]interface{}{
		"Items":       items,
		"LastUpdated": now.Format("Jan 02, 2006 3:04 PM MST"),
	}

	err = h.tmpl.ExecuteTemplate(w, "gex_scanner.html", data)
	if err != nil {
		h.logger.Error("failed to render template", "error", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}
