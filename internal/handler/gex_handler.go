package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"

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
