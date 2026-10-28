package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arnabmitra/eth-proxy/internal/handler/gex"
)

type HeatmapHandler struct {
	logger *slog.Logger
	tmpl   *template.Template
	cache  sync.Map // key: string (symbol), value: *heatmapCacheEntry
}

type heatmapCacheEntry struct {
	symbol      string
	spotPrice   float64
	expirations []string
	chains      map[string][]gex.Option // key: expiration date
	fetchedAt   time.Time
}

type ExpirationInfo struct {
	Date  string `json:"date"`
	Label string `json:"label"` // e.g. "Oct 30"
	DTE   int    `json:"dte"`   // e.g. 6
}

type HeatmapCell struct {
	Strike     float64 `json:"strike"`
	Expiration string  `json:"expiration"`
	DTE        int     `json:"dte"`
	OptionType string  `json:"option_type"`
	Bid        float64 `json:"bid"`
	Ask        float64 `json:"ask"`
	Mark       float64 `json:"mark"`
	Premium    float64 `json:"premium"`    // Mark * 100
	PerDay     float64 `json:"per_day"`    // Premium / DTE
	ThetaDay   float64 `json:"theta_day"`  // |Theta| * 100
	Delta      float64 `json:"delta"`
	AbsDelta   float64 `json:"abs_delta"`
	Theta      float64 `json:"theta"`
	Gamma      float64 `json:"gamma"`
	Vega       float64 `json:"vega"`
	IV         float64 `json:"iv"`         // IV in percentage (e.g. 53.5)
	RiskTier   string  `json:"risk_tier"`  // "safe", "low", "moderate", "risky", "aggressive"
}

type HeatmapResponse struct {
	Symbol      string                 `json:"symbol"`
	SpotPrice   float64                `json:"spot_price"`
	OptionType  string                 `json:"option_type"`
	Expirations []ExpirationInfo       `json:"expirations"`
	Strikes     []float64              `json:"strikes"`
	Matrix      map[string]HeatmapCell `json:"matrix"`
	AtmIV       float64                `json:"atm_iv"`
	UpdatedAt   string                 `json:"updated_at"`
}

func NewHeatmapHandler(logger *slog.Logger, tmpl *template.Template) *HeatmapHandler {
	return &HeatmapHandler{
		logger: logger,
		tmpl:   tmpl,
	}
}

// ServeHTTP renders the main heatmap page
func (h *HeatmapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if symbol == "" {
		symbol = "SPY"
	}

	data := map[string]interface{}{
		"Symbol": symbol,
	}

	err := h.tmpl.ExecuteTemplate(w, "heatmap.html", data)
	if err != nil {
		h.logger.Error("Failed to render heatmap page", slog.Any("error", err))
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// GetHeatmapData returns JSON matrix data for the given symbol, type, and expirations count
func (h *HeatmapHandler) GetHeatmapData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if symbol == "" {
		symbol = "SPY"
	}

	optType := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("type")))
	if optType != "CALL" && optType != "PUT" {
		if strings.HasPrefix(strings.ToLower(optType), "c") {
			optType = "CALL"
		} else {
			optType = "PUT"
		}
	}

	numExp := 6
	if nStr := r.URL.Query().Get("expirations"); nStr != "" {
		if n, err := strconv.Atoi(nStr); err == nil && n > 0 && n <= 16 {
			numExp = n
		}
	}

	apiKey := os.Getenv("ALPACA_API_KEY")
	apiSecret := os.Getenv("ALPACA_API_SECRET")

	entry, err := h.getOrFetchChains(symbol, numExp, apiKey, apiSecret)
	if err != nil {
		h.logger.Error("Failed to fetch heatmap data", slog.String("symbol", symbol), slog.Any("error", err))
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	loc, _ := time.LoadLocation("America/New_York")
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	// Filter down to the requested number of expirations
	availableExps := entry.expirations
	if len(availableExps) > numExp {
		availableExps = availableExps[:numExp]
	}

	expInfos := make([]ExpirationInfo, 0, len(availableExps))
	for _, expDateStr := range availableExps {
		t, err := time.ParseInLocation("2006-01-02", expDateStr, loc)
		dte := 0
		label := expDateStr
		if err == nil {
			dte = int(t.Sub(today).Hours() / 24)
			if dte < 0 {
				dte = 0
			}
			label = t.Format("Jan 02")
		}
		expInfos = append(expInfos, ExpirationInfo{
			Date:  expDateStr,
			Label: label,
			DTE:   dte,
		})
	}

	// Collect unique strikes and calculate ATM IV
	strikeSet := make(map[float64]bool)
	var atmIVTotal float64
	var atmIVCount int

	matrix := make(map[string]HeatmapCell)

	for _, expInfo := range expInfos {
		options, ok := entry.chains[expInfo.Date]
		if !ok {
			continue
		}

		for _, opt := range options {
			// Normalize option type match
			isMatch := false
			if optType == "PUT" && (strings.ToUpper(opt.OptionType) == "PUT" || strings.ToUpper(opt.OptionType) == "P") {
				isMatch = true
			} else if optType == "CALL" && (strings.ToUpper(opt.OptionType) == "CALL" || strings.ToUpper(opt.OptionType) == "C") {
				isMatch = true
			}

			if !isMatch {
				continue
			}

			strikeSet[opt.Strike] = true

			mark := opt.Mark
			if mark == 0 && opt.Bid > 0 && opt.Ask > 0 {
				mark = (opt.Bid + opt.Ask) / 2.0
			} else if mark == 0 && opt.Ask > 0 {
				mark = opt.Ask
			} else if mark == 0 && opt.Bid > 0 {
				mark = opt.Bid
			}

			effectiveDTE := expInfo.DTE
			if effectiveDTE <= 0 {
				effectiveDTE = 1
			}

			// Black-Scholes theoretical fallback if mark is missing
			if mark == 0 && entry.spotPrice > 0 {
				iv := opt.Greeks.IV
				if iv <= 0 {
					iv = 0.25
				} else if iv > 2.5 {
					iv = iv / 100.0
				}
				isCall := optType == "CALL"
				tYears := float64(effectiveDTE) / 365.0
				bsPrice := gex.BlackScholesPrice(entry.spotPrice, opt.Strike, iv, tYears, 0.05, isCall)
				if bsPrice > 0.01 {
					mark = bsPrice
				}
			}

			premium := mark * 100.0
			perDay := premium / float64(effectiveDTE)
			thetaDay := math.Abs(opt.Greeks.Theta) * 100.0
			if thetaDay == 0 && perDay > 0 {
				thetaDay = perDay * 0.75
			}

			absDelta := math.Abs(opt.Greeks.Delta)
			// If delta is 0 or unprovided, estimate reasonable delta from spot/strike if needed
			if absDelta == 0 && entry.spotPrice > 0 {
				// Rough approximation based on moneyness if delta was missing
				moneyness := opt.Strike / entry.spotPrice
				if optType == "CALL" {
					if moneyness < 1.0 {
						absDelta = math.Min(0.95, 0.5+0.5*(1.0-moneyness)*5)
					} else {
						absDelta = math.Max(0.05, 0.5-0.5*(moneyness-1.0)*5)
					}
				} else {
					if moneyness > 1.0 {
						absDelta = math.Min(0.95, 0.5+0.5*(moneyness-1.0)*5)
					} else {
						absDelta = math.Max(0.05, 0.5-0.5*(1.0-moneyness)*5)
					}
				}
			}

			var riskTier string
			switch {
			case absDelta < 0.15:
				riskTier = "safe"
			case absDelta < 0.30:
				riskTier = "low"
			case absDelta < 0.45:
				riskTier = "moderate"
			case absDelta < 0.60:
				riskTier = "risky"
			default:
				riskTier = "aggressive"
			}

			ivPercent := opt.Greeks.IV
			if ivPercent > 0 && ivPercent <= 2.5 {
				// Convert decimal (e.g. 0.535) to percentage (53.5%)
				ivPercent = ivPercent * 100.0
			}

			// ATM IV calculation
			if entry.spotPrice > 0 && math.Abs(opt.Strike-entry.spotPrice)/entry.spotPrice < 0.03 && ivPercent > 0 {
				atmIVTotal += ivPercent
				atmIVCount++
			}

			cellKey := fmt.Sprintf("%.2f_%s", opt.Strike, expInfo.Date)
			matrix[cellKey] = HeatmapCell{
				Strike:     opt.Strike,
				Expiration: expInfo.Date,
				DTE:        expInfo.DTE,
				OptionType: optType,
				Bid:        opt.Bid,
				Ask:        opt.Ask,
				Mark:       mark,
				Premium:    premium,
				PerDay:     perDay,
				ThetaDay:   thetaDay,
				Delta:      opt.Greeks.Delta,
				AbsDelta:   absDelta,
				Theta:      opt.Greeks.Theta,
				Gamma:      opt.Greeks.Gamma,
				Vega:       opt.Greeks.Vega,
				IV:         ivPercent,
				RiskTier:   riskTier,
			}
		}
	}

	// Sort strikes
	allStrikes := make([]float64, 0, len(strikeSet))
	for s := range strikeSet {
		allStrikes = append(allStrikes, s)
	}
	sort.Float64s(allStrikes)

	// Focus strike window around spot price (around ±18 strikes)
	focusedStrikes := allStrikes
	if len(allStrikes) > 40 && entry.spotPrice > 0 {
		closestIdx := 0
		minDiff := math.Abs(allStrikes[0] - entry.spotPrice)
		for i, s := range allStrikes {
			diff := math.Abs(s - entry.spotPrice)
			if diff < minDiff {
				minDiff = diff
				closestIdx = i
			}
		}
		startIdx := closestIdx - 18
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := closestIdx + 19
		if endIdx > len(allStrikes) {
			endIdx = len(allStrikes)
		}
		focusedStrikes = allStrikes[startIdx:endIdx]
	}

	atmIV := 0.0
	if atmIVCount > 0 {
		atmIV = atmIVTotal / float64(atmIVCount)
	}

	resp := HeatmapResponse{
		Symbol:      symbol,
		SpotPrice:   entry.spotPrice,
		OptionType:  optType,
		Expirations: expInfos,
		Strikes:     focusedStrikes,
		Matrix:      matrix,
		AtmIV:       atmIV,
		UpdatedAt:   entry.fetchedAt.In(loc).Format("15:04:05 MST"),
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// getOrFetchChains retrieves data from memory cache or fetches it in parallel
func (h *HeatmapHandler) getOrFetchChains(symbol string, numExp int, apiKey, apiSecret string) (*heatmapCacheEntry, error) {
	if val, ok := h.cache.Load(symbol); ok {
		entry := val.(*heatmapCacheEntry)
		if time.Since(entry.fetchedAt) < 60*time.Second && len(entry.expirations) >= numExp {
			return entry, nil
		}
	}

	// Fetch spot price
	spotPrice, err := gex.GetSpotPrice(apiKey, apiSecret, symbol)
	if err != nil {
		h.logger.Warn("Failed to get spot price, continuing with 0", slog.Any("error", err))
	}

	// Fetch expiration dates
	expDates, err := gex.GetExpirationDates(apiKey, apiSecret, symbol)
	if err != nil || len(expDates) == 0 {
		return nil, fmt.Errorf("no expiration dates available for %s: %v", symbol, err)
	}

	// Take up to 10 expirations for cache
	fetchExpCount := numExp
	if fetchExpCount < 8 && len(expDates) >= 8 {
		fetchExpCount = 8
	}
	if fetchExpCount > len(expDates) {
		fetchExpCount = len(expDates)
	}
	targetExpirations := expDates[:fetchExpCount]

	// Parallel fetch across expirations
	chains := make(map[string][]gex.Option)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, exp := range targetExpirations {
		wg.Add(1)
		go func(expiration string) {
			defer wg.Done()
			options, _, _, err := gex.FetchOptionsChain(symbol, expiration, apiKey, apiSecret)
			if err == nil && len(options) > 0 {
				mu.Lock()
				chains[expiration] = options
				mu.Unlock()
			}
		}(exp)
	}
	wg.Wait()

	if len(chains) == 0 {
		return nil, fmt.Errorf("failed to fetch option chain data for %s", symbol)
	}

	entry := &heatmapCacheEntry{
		symbol:      symbol,
		spotPrice:   spotPrice,
		expirations: targetExpirations,
		chains:      chains,
		fetchedAt:   time.Now(),
	}

	h.cache.Store(symbol, entry)
	return entry, nil
}
