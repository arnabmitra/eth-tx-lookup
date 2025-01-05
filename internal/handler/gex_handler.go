package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"html/template"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/arnabmitra/eth-proxy/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dustin/go-humanize"

	"log/slog"

	"github.com/arnabmitra/eth-proxy/internal/handler/gex"
)

type GEXHandler struct {
	logger *slog.Logger
	tmpl   *template.Template
	repo   *repository.Queries
}

func NewGEXHandler(logger *slog.Logger, tmpl *template.Template, db *pgxpool.Pool) *GEXHandler {
	return &GEXHandler{
		logger: logger,
		tmpl:   tmpl,
		repo:   repository.New(db),
	}
}

func (h *GEXHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		symbol := r.FormValue("symbol")
		expiration := r.FormValue("expiration")

		apiKey := os.Getenv("TRADIER_API_KEY")
		if apiKey == "" {
			http.Error(w, "TRADIER_API_KEY environment variable is not set", http.StatusInternalServerError)
			return
		}

		expirationDates, err := gex.GetExpirationDates(apiKey, symbol)
		if err != nil {
			return
		}

		expirationDatesJSON, err := json.MarshalIndent(expirationDates, "", "  ")
		if err != nil {
			fmt.Printf("Error marshalling expiration dates to JSON: %v\n", err)
			return
		}
		fmt.Println(string(expirationDatesJSON))

		err = h.storeExpiryDatesInOptionExpiryDates(r.Context(), symbol, expirationDatesJSON)
		if err != nil {
			return
		}

		price, err := gex.GetSpotPrice(apiKey, symbol)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching price: %v", err), http.StatusInternalServerError)
			return
		}

		options, jsonOption, err := gex.FetchOptionsChain(symbol, expiration, apiKey)

		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching options chain: %v", err), http.StatusInternalServerError)
			return
		}
		err = h.storeExpiryDates(r.Context(), options, symbol, *jsonOption)
		if err != nil {
			return
		}
		//print the options chain
		fmt.Println("Options Chain:")
		for _, option := range options {
			fmt.Printf("Strike: %.2f, Type: %s, Expiration: %s, Open Interest: %d, Expiration Type : %s\n",
				option.Strike, option.OptionType, option.ExpirationDate, option.OpenInterest, option.ExpirationType)
		}

		gexByStrike := gex.CalculateGEXPerStrike(options, price)

		strikePrices := make([]float64, 0, len(gexByStrike))
		for strike := range gexByStrike {
			strikePrices = append(strikePrices, strike)
		}
		sort.Float64s(strikePrices)
		// Print GEX per strike in sorted order
		fmt.Println("Gamma Exposure (GEX) per Strike Price (Sorted):")
		for _, strike := range strikePrices {
			fmt.Printf("Strike: %.2f, GEX: %.2f\n", strike, gexByStrike[strike])
		}
		// Create a slice of structs to hold strike prices and GEX values
		type GEXEntry struct {
			Strike float64
			GEX    float64
		}

		gexEntries := make([]GEXEntry, len(strikePrices))
		for i, strike := range strikePrices {
			gexEntries[i] = GEXEntry{
				Strike: strike,
				GEX:    gexByStrike[strike],
			}
		}

		// Sort the slice by GEX values in descending order
		sort.Slice(gexEntries, func(i, j int) bool {
			return math.Abs(gexEntries[i].GEX) > math.Abs(gexEntries[j].GEX)
		})

		// Print the sorted GEX data
		fmt.Println("Gamma Exposure (GEX) per Strike Price (Sorted by GEX):")
		for _, entry := range gexEntries {
			fmt.Printf("Strike: %.2f, GEX: %.2f\n", entry.Strike, entry.GEX)
		}

		// Prepare data for template
		gexData := make([]map[string]string, len(gexEntries))
		for i, entry := range gexEntries {
			gexData[i] = map[string]string{
				"Strike": fmt.Sprintf("%.2f", entry.Strike),
				"GEX":    humanize.Commaf(entry.GEX),
			}
		}
		outputPath := filepath.Join("static", "gex_chart_"+symbol+".png")
		err = gex.CreateGEXPlot(gexByStrike, symbol, outputPath, price)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error creating bar chart: %v", err), http.StatusInternalServerError)
			return
		}

		err = h.tmpl.ExecuteTemplate(w, "gex.html", map[string]interface{}{
			"ImagePath": fmt.Sprintf("/%s?nocache=%d", outputPath, time.Now().Unix()),
			"GEXData":   gexData,
		})
		if err != nil {
			h.renderError(w, fmt.Sprintf("Error fetching options chain: %v", err))
			return
		}
		return
	}

	err := h.tmpl.ExecuteTemplate(w, "gex.html", nil)
	if err != nil {
		return
	}
}

func (h *GEXHandler) renderError(w http.ResponseWriter, errMsg string) {
	err := h.tmpl.ExecuteTemplate(w, "error.html", map[string]interface{}{
		"Error": errMsg,
	})

	if err != nil {
		h.renderError(w, fmt.Sprintf("Error executing template: %v", err))
		return
	}
}

func (h *GEXHandler) storeExpiryDates(ctx context.Context, options []gex.Option, symbol string, jsonData string) error {

	var expirationType string
	var expiryDate pgtype.Date
	if len(options) > 0 {
		expirationType = options[0].ExpirationType
		date, err := stringToPgDate(options[0].ExpirationDate)
		if err != nil {
			return err
		}
		expiryDate = date
	}

	_, err := h.repo.UpsertOptionExpiry(ctx, repository.UpsertOptionExpiryParams{
		Symbol:      symbol,
		ExpiryDate:  expiryDate,
		ExpiryType:  expirationType,
		OptionChain: []byte(jsonData),
	})
	if err != nil {
		h.logger.Error("failed to store option expiry", "error", err)
		return err
	}
	return nil
}

func stringToPgDate(dateStr string) (pgtype.Date, error) {
	// Parse string to time.Time
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("parse date: %w", err)
	}

	// Convert to pgtype.Date
	date := pgtype.Date{
		Time:  t,
		Valid: true,
	}

	return date, nil
}

func (h *GEXHandler) storeExpiryDatesInOptionExpiryDates(ctx context.Context, symbol string, expiryDates []byte) error {
	_, err := h.repo.UpsertOptionExpiryDates(ctx, repository.UpsertOptionExpiryDatesParams{
		Symbol:      symbol,
		ExpiryDates: expiryDates,
	})
	if err != nil {
		h.logger.Error("failed to store option expiry dates", "error", err)
		return err
	}
	return nil
}
