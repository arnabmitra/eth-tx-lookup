package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"html/template"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// CalculateGEXForAllExpiries calculates GEX across all expiry dates for a symbol
func (h *GEXHandler) CalculateGEXForAllExpiries(ctx context.Context, symbol string) (map[float64]float64, error) {
	apiKey := os.Getenv("TRADIER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TRADIER_API_KEY environment variable is not set")
	}
	expiryDates, err := h.getExpiryDates(ctx, symbol)

	if err != nil || expiryDates == nil || len(expiryDates) == 0 {

		expiryDates, err = gex.GetExpirationDates(apiKey, symbol)
		if err != nil {
			return nil, fmt.Errorf("cannot get expiration dates: %v", err)
		}

		expirationDatesJSON, err := json.MarshalIndent(expiryDates, "", "  ")
		if err != nil {
			fmt.Printf("Error marshalling expiration dates to JSON: %v\n", err)
			return nil, fmt.Errorf("cannot get expiration dates: %v", err)
		}

		err = h.storeExpiryDatesInOptionExpiryDates(ctx, symbol, expirationDatesJSON)

	}

	// Get current price
	price, err := gex.GetSpotPrice(apiKey, symbol)
	if err != nil {
		return nil, fmt.Errorf("error fetching price: %v", err)
	}

	// Initialize combined GEX map
	combinedGEXByStrike := make(map[float64]float64)

	h.logger.Info(fmt.Sprintf("Processing %d expiry dates for %s", len(expiryDates), symbol))

	// Process each expiry date
	for _, expiryDate := range expiryDates {
		var options []gex.Option

		expirationDatePgType, err := stringToPgDate(expiryDate)
		if err != nil {
			continue
		}

		// Try to get cached options data first
		expiry, err := h.repo.GetOptionChainBySymbolAndExpiry(ctx, repository.GetOptionChainBySymbolAndExpiryParams{
			Symbol:     symbol,
			ExpiryDate: expirationDatePgType,
		})

		if err == nil && time.Since(expiry.UpdatedAt) <= 10*time.Minute {
			// Use cached data
			var response gex.Response
			err = json.Unmarshal(expiry.OptionChain, &response)
			if err == nil {
				options = response.Options.Option
			}
		} else {
			// Fetch fresh data
			optionsFromApi, jsonOption, errFromApi := gex.FetchOptionsChain(symbol, expiryDate, apiKey)
			if errFromApi != nil || jsonOption == nil {
				continue // Skip this expiry if there's an error
			}
			// Use cached data
			var response gex.Response
			if expiry.OptionChain != nil {
				errFromUnMarshal := json.Unmarshal(expiry.OptionChain, &response)
				if errFromUnMarshal == nil {
					options = response.Options.Option
				}
			} else {
				h.logger.Debug("No cached option chain available")
			}
			err = h.storeOptionChain(ctx, optionsFromApi, symbol, *jsonOption, fmt.Sprintf("%.2f", price))
			if err != nil {
				continue
			}
		}

		// Calculate GEX for this expiry and add to combined total
		gexByStrike := gex.CalculateGEXPerStrike(options, price)
		for strike, gexValue := range gexByStrike {
			combinedGEXByStrike[strike] += gexValue
		}
	}

	return combinedGEXByStrike, nil
}

func (h *GEXHandler) AllGEXHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		symbol := r.FormValue("symbol")

		apiKey := os.Getenv("TRADIER_API_KEY")
		if apiKey == "" {
			http.Error(w, "TRADIER_API_KEY environment variable is not set", http.StatusInternalServerError)
			return
		}

		// Calculate GEX for all expiry dates
		gexByStrike, err := h.CalculateGEXForAllExpiries(r.Context(), symbol)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error calculating GEX for all expiries: %v", err), http.StatusInternalServerError)
			return
		}

		// Get current price
		price, err := gex.GetSpotPrice(apiKey, symbol)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching price: %v", err), http.StatusInternalServerError)
			return
		}

		// Process GEX data
		strikePrices := make([]float64, 0, len(gexByStrike))
		for strike := range gexByStrike {
			strikePrices = append(strikePrices, strike)
		}
		sort.Float64s(strikePrices)

		type GEXEntry struct {
			Strike float64
			GEX    float64
		}

		gexEntries := make([]GEXEntry, len(strikePrices))
		for i, strike := range strikePrices {
			gexEntries[i] = GEXEntry{Strike: strike, GEX: gexByStrike[strike]}
		}

		// Sort by absolute GEX value and limit to top 20
		sort.Slice(gexEntries, func(i, j int) bool {
			return math.Abs(gexEntries[i].GEX) > math.Abs(gexEntries[j].GEX)
		})

		// Filter out entries with very small GEX values (insignificant)
		minSignificantGEX := 1000.0 // Adjust this threshold as needed
		filteredEntries := make([]GEXEntry, 0)
		for _, entry := range gexEntries {
			if math.Abs(entry.GEX) >= minSignificantGEX {
				filteredEntries = append(filteredEntries, entry)
			}
		}
		gexEntries = filteredEntries

		// Still limit to top 20 if we have too many
		if len(gexEntries) > 20 {
			gexEntries = gexEntries[:20]
		}

		// Prepare template data
		gexData := make([]map[string]string, len(gexEntries))
		for i, entry := range gexEntries {
			gexData[i] = map[string]string{
				"Strike": fmt.Sprintf("%.2f", entry.Strike),
				"GEX":    humanize.Commaf(entry.GEX),
			}
		}

		outputPath := filepath.Join("static", "all_gex_chart_"+symbol+".png")
		err = gex.CreateGEXPlot(gexByStrike, symbol+" (All Expiries)", outputPath, price)
		if err != nil {
			h.renderError(w, fmt.Sprintf("Error getting data for this SYMBOL: %v", err))
			return
		}

		// Add a small delay to ensure the file is completely written
		time.Sleep(10 * time.Millisecond)

		// Ensure the file exists and is accessible
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			http.Error(w, "Image file not ready yet", http.StatusInternalServerError)
			return
		}

		// Set proper content type for HTML response
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		err = h.tmpl.ExecuteTemplate(w, "all_gex_chart.html", map[string]interface{}{
			"ImagePath": fmt.Sprintf("/%s?nocache=%d", outputPath, time.Now().Unix()),
			"GEXData":   gexData,
			"Symbol":    symbol,
		})
		if err != nil {
			h.renderError(w, fmt.Sprintf("Error rendering template: %v", err))
			return
		}
		return
	}

	err := h.tmpl.ExecuteTemplate(w, "all_gex.html", nil)
	if err != nil {
		h.renderError(w, fmt.Sprintf("Error rendering template: %v", err))
		return
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

		expiryDates, err := h.getExpiryDates(r.Context(), symbol)

		if err != nil || len(expiryDates) == 0 {
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
			expiryDates, err = h.getExpiryDates(r.Context(), symbol)
			if err != nil {
				return
			}
		}

		expirationDatePgType, err := stringToPgDate(expiration)
		if err != nil {
			return
		}
		var options []gex.Option
		var jsonOption *string
		var price float64
		expiry, err := h.repo.GetOptionChainBySymbolAndExpiry(r.Context(), repository.GetOptionChainBySymbolAndExpiryParams{Symbol: symbol, ExpiryDate: expirationDatePgType})
		if err != nil {
			h.logger.Error("failed to get option chain", "error", err)
		}

		if &expiry != nil && time.Since(expiry.UpdatedAt) <= 10*time.Minute {
			var response gex.Response
			err = json.Unmarshal(expiry.OptionChain, &response)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error unmarshalling options chain: %v", err), http.StatusInternalServerError)
				return
			}
			options = response.Options.Option
			priceFloat, err := strconv.ParseFloat(expiry.SpotPrice, 64)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error converting spot price to float64: %v", err), http.StatusInternalServerError)
				return
			}
			price = priceFloat
		} else {
			//always get the spot price
			price, err = gex.GetSpotPrice(apiKey, symbol)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error fetching price: %v", err), http.StatusInternalServerError)
				return
			}
			options, jsonOption, err = gex.FetchOptionsChain(symbol, expiration, apiKey)

			if err != nil {
				http.Error(w, fmt.Sprintf("Error fetching options chain: %v", err), http.StatusInternalServerError)
				return
			}
			err = h.storeOptionChain(r.Context(), options, symbol, *jsonOption, fmt.Sprintf("%.2f", price))
			if err != nil {
				return
			}
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

		// Limit to top 20 strike prices
		if len(gexEntries) > 20 {
			gexEntries = gexEntries[:20]
		}

		// // Print the sorted and limited GEX data

		fmt.Println("Top 20 Gamma Exposure (GEX) per Strike Price (Sorted by GEX):")
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

		err = h.tmpl.ExecuteTemplate(w, "gex_chart.html", map[string]interface{}{
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

func (h *GEXHandler) storeOptionChain(ctx context.Context, options []gex.Option, symbol string, jsonData string, price string) error {

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

	_, err := h.repo.UpsertOptionChain(ctx, repository.UpsertOptionChainParams{
		Symbol:      symbol,
		ExpiryDate:  expiryDate,
		ExpiryType:  expirationType,
		OptionChain: []byte(jsonData),
		SpotPrice:   price,
	})
	if err != nil {
		h.logger.Error("failed to store option expiry", "error", err)
		return err
	}

	var gexValue pgtype.Numeric
	// Set a string value
	err = gexValue.Scan(price)
	if err != nil {
		fmt.Println("Error setting value:", err)
	} else {
		fmt.Println("GexValue set successfully:", gexValue)
	}
	recordedAt := time.Now()
	_, err = h.repo.InsertGexHistory(ctx, repository.InsertGexHistoryParams{
		ID:          uuid.New(),
		Symbol:      symbol,
		ExpiryDate:  expiryDate,
		ExpiryType:  expirationType,
		OptionChain: []byte(jsonData),
		GexValue:    gexValue,
		RecordedAt:  recordedAt,
	})
	if err != nil {
		h.logger.Error("failed to insert GEX history", "error", err)
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

func (h *GEXHandler) getExpiryDates(ctx context.Context, symbol string) ([]string, error) {
	expiryDates, err := h.repo.GetOptionExpiryDatesBySymbol(ctx, symbol)
	if err != nil {
		// Check if it's a "no rows" error, and return empty slice instead of error
		if err.Error() == "no rows in result set" {
			return []string{}, nil
		}
		return nil, err
	}

	var dates []string

	if expiryDates.UpdatedAt.Before(time.Now().Add(-1 * 24 * time.Hour)) {
		return dates, nil
	}
	err = json.Unmarshal(expiryDates.ExpiryDates, &dates)
	if err != nil {
		return nil, err
	}

	return dates, nil
}

func (h *GEXHandler) GetExpiryDatesHandler(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		http.Error(w, "Symbol is required", http.StatusBadRequest)
		return
	}

	expiryDates, err := h.getExpiryDates(r.Context(), symbol)

	if err != nil || expiryDates == nil || len(expiryDates) == 0 {
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

		err = h.storeExpiryDatesInOptionExpiryDates(r.Context(), symbol, expirationDatesJSON)
		if err != nil {
			return
		}
		expiryDates, err = h.getExpiryDates(r.Context(), symbol)
		if err != nil {
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(expiryDates)
}
