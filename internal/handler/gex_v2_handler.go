package handler

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/arnabmitra/eth-proxy/internal/handler/gex"
	"github.com/arnabmitra/eth-proxy/internal/repository"
	"github.com/dustin/go-humanize"
)

func (h *GEXHandler) ServeHTTPV2(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		symbol := strings.ToUpper(strings.TrimSpace(r.FormValue("symbol")))
		expiration := r.FormValue("expiration")

		apiKey := os.Getenv("ALPACA_API_KEY")
		apiSecret := os.Getenv("ALPACA_API_SECRET")
		if apiKey == "" || apiSecret == "" {
			http.Error(w, "ALPACA_API_KEY or ALPACA_API_SECRET environment variable is not set", http.StatusInternalServerError)
			return
		}

		expiryDates, err := h.GetExpiryDates(r.Context(), symbol)

		if err != nil || len(expiryDates) == 0 {
			expirationDates, err := gex.GetExpirationDates(apiKey, apiSecret, symbol)
			if err != nil {
				return
			}

			expirationDatesJSON, err := json.MarshalIndent(expirationDates, "", "  ")
			if err != nil {
				fmt.Printf("Error marshalling expiration dates to JSON: %v\n", err)
				return
			}
			fmt.Println(string(expirationDatesJSON))

			err = h.StoreExpiryDatesInOptionExpiryDates(r.Context(), symbol, expirationDatesJSON)
			if err != nil {
				return
			}
			expiryDates, err = h.GetExpiryDates(r.Context(), symbol)
			if err != nil {
				return
			}
		}

		// If no expiration provided, use the next available expiration date
		if expiration == "" {
			if len(expiryDates) > 0 {
				expiration = expiryDates[0]
				h.logger.Info("No expiration provided, using next available", "expiration", expiration)
			} else {
				http.Error(w, "No expiration dates available", http.StatusInternalServerError)
				return
			}
		}

		expirationDatePgType, err := stringToPgDate(expiration)
		if err != nil {
			h.logger.Error("failed to parse expiration date", "error", err, "expiration", expiration)
			http.Error(w, fmt.Sprintf("Invalid expiration date: %v", err), http.StatusBadRequest)
			return
		}
		var options []gex.Option
		var jsonOption *string
		var price float64
		var warning string
		expiry, err := h.repo.GetOptionChainBySymbolAndExpiry(r.Context(), repository.GetOptionChainBySymbolAndExpiryParams{Symbol: symbol, ExpiryDate: expirationDatePgType})
		
		if err == nil && time.Since(expiry.UpdatedAt) <= 1*time.Minute {
			var response gex.Response
			err = json.Unmarshal(expiry.OptionChain, &response)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error unmarshalling options chain: %v", err), http.StatusInternalServerError)
				return
			}
			options = response.Options.Option
			warning = response.Warning
			priceFloat, err := strconv.ParseFloat(expiry.SpotPrice, 64)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error converting spot price to float64: %v", err), http.StatusInternalServerError)
				return
			}
			price = priceFloat
		} else {
			//always get the spot price
			price, err = gex.GetSpotPrice(apiKey, apiSecret, symbol)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error fetching price: %v", err), http.StatusInternalServerError)
				return
			}
			options, jsonOption, warning, err = gex.FetchOptionsChain(symbol, expiration, apiKey, apiSecret)

			if err != nil {
				h.logger.Error("failed to fetch options chain", "error", err, "symbol", symbol, "expiration", expiration)
				http.Error(w, fmt.Sprintf("Error fetching options chain: %v", err), http.StatusInternalServerError)
				return
			}
			h.logger.Info("Fetched options chain", "count", len(options), "symbol", symbol, "expiration", expiration)

			// Calculate GEX for the nearest expiry
			gexByStrike := gex.CalculateGEXPerStrike(options, price)
			h.logger.Info("Calculated GEX per strike", "count", len(gexByStrike), "symbol", symbol)

			// Only store if we actually got options
			if len(options) > 0 {
				// Calculate total GEX (sum of all strikes)
				totalGEX := 0.0
				for _, gexValue := range gexByStrike {
					totalGEX += gexValue
				}
				err = h.StoreOptionChain(r.Context(), options, symbol, *jsonOption, fmt.Sprintf("%.2f", price), fmt.Sprintf("%.2f", totalGEX))
				if err != nil {
					h.logger.Error("failed to store option chain", "error", err)
					return
				}
			}
		}

		gexByStrike := gex.CalculateGEXPerStrike(options, price)

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

		// Prepare data for template
		gexData := make([]map[string]interface{}, len(gexEntries))
		for i, entry := range gexEntries {
			gexData[i] = map[string]interface{}{
				"Strike":     fmt.Sprintf("%.2f", entry.Strike),
				"GEX":        humanize.FormatFloat("#,###.##", entry.GEX),
				"IsPositive": entry.GEX >= 0,
			}
		}
		
		// Prepare chart data for D3.js
		chartData := make([]map[string]interface{}, 0)
		for strike, gexValue := range gexByStrike {
			if gexValue != 0 {
				chartData = append(chartData, map[string]interface{}{
					"strike": strike,
					"gex":    gexValue,
				})
			}
		}

		// Sort by strike price
		sort.Slice(chartData, func(i, j int) bool {
			return chartData[i]["strike"].(float64) < chartData[j]["strike"].(float64)
		})

		// Calculate gamma flip level
		gammaFlipLevel := gex.CalculateGammaFlipLevel(gexByStrike)

		// Calculate total GEX
		totalGEX := 0.0
		for _, gexValue := range gexByStrike {
			totalGEX += gexValue
		}

		// Format total GEX properly
		totalGEXFormatted := formatCurrency(totalGEX)

		h.logger.Info("GEX calculation complete for single expiry",
			"symbol", symbol,
			"expiration", expiration,
			"totalGEX", totalGEX,
			"totalGEXFormatted", totalGEXFormatted,
			"gammaFlipLevel", gammaFlipLevel,
			"chartDataCount", len(chartData))

		// Calculate regime summary metrics
		totalCallGEX := 0.0
		totalPutGEX := 0.0
		for _, opt := range options {
			if opt.Greeks.Gamma != 0 {
				gexVal := float64(opt.OpenInterest) * opt.Greeks.Gamma * 100 * price
				if strings.ToLower(opt.OptionType) == "call" {
					totalCallGEX += gexVal
				} else {
					totalPutGEX += math.Abs(gexVal)
				}
			}
		}

		netGEX := totalCallGEX - totalPutGEX
		totalAbsGEX := totalCallGEX + totalPutGEX
		putCallRatio := 0.0
		if totalCallGEX > 0 {
			putCallRatio = totalPutGEX / totalCallGEX
		}

		regimeSummary := map[string]interface{}{
			"NetGEX":          netGEX,
			"NetGEXFormatted": formatCurrency(netGEX),
			"TotalGEX":        totalAbsGEX,
			"TotalGEXFormatted": formatCurrency(totalAbsGEX),
			"Condition":       func() string { if netGEX > 0 { return "Positive" }; return "Negative" }(),
			"PCRatio":         fmt.Sprintf("%.2f", putCallRatio),
		}

		err = h.tmpl.ExecuteTemplate(w, "gex_chart_v2.html", map[string]interface{}{
			"Symbol":            symbol,
			"Expiration":        expiration,
			"SpotPrice":         price,
			"GEXData":           gexData,
			"ChartData":         chartData,
			"GammaFlipLevel":    gammaFlipLevel,
			"TotalGEX":          totalGEX,
			"TotalGEXFormatted": totalGEXFormatted,
			"RegimeSummary":     regimeSummary,
			"Warning":           warning,
		})
		if err != nil {
			h.renderError(w, fmt.Sprintf("Error fetching options chain: %v", err))
			return
		}
		return
	}

	err := h.tmpl.ExecuteTemplate(w, "gex_v2.html", map[string]interface{}{
		"ChartData": nil,
	})
	if err != nil {
		return
	}
}
