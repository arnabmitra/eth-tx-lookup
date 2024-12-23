package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/arnabmitra/eth-proxy/internal/handler/gex"
	"log/slog"
)

type GEXHandler struct {
	logger *slog.Logger
	tmpl   *template.Template
}

func NewGEXHandler(logger *slog.Logger, tmpl *template.Template) *GEXHandler {
	return &GEXHandler{
		logger: logger,
		tmpl:   tmpl,
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

		price, err := gex.GetSpotPrice(apiKey, symbol)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching price: %v", err), http.StatusInternalServerError)
			return
		}

		options, err := gex.FetchOptionsChain(symbol, expiration, apiKey)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching options chain: %v", err), http.StatusInternalServerError)
			return
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

		outputPath := filepath.Join("static", "images", "gex_chart_"+symbol+".png")
		err = gex.CreateGEXPlot(gexByStrike, symbol, outputPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error creating bar chart: %v", err), http.StatusInternalServerError)
			return
		}

		h.tmpl.ExecuteTemplate(w, "gex.html", map[string]string{
			"ImagePath": "/" + outputPath,
		})
		return
	}

	h.tmpl.ExecuteTemplate(w, "gex.html", nil)
}
