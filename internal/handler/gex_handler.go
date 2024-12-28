package handler

import (
	"fmt"
	"github.com/dustin/go-humanize"
	"html/template"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

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

		gex.GetExpirationDates(apiKey, symbol)

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
		err = gex.CreateGEXPlot(gexByStrike, symbol, outputPath)
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

	h.tmpl.ExecuteTemplate(w, "gex.html", nil)
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
