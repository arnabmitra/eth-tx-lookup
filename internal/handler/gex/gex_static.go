package gex

import (
	"encoding/json"
	"fmt"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"image/color"
	"sort"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"cloud.google.com/go/civil"
	"os"
	"strings"
	"math"
)

// BlackScholesGamma calculates an approximate gamma if the provider doesn't supply it
func EstimateGamma(spot, strike, iv, daysToExpiration float64) float64 {
	if iv <= 0 || daysToExpiration <= 0 || spot <= 0 {
		return 0
	}

	t := daysToExpiration / 365.0
	r := 0.05 // assume 5% risk free rate
	
	d1 := (math.Log(spot/strike) + (r+0.5*iv*iv)*t) / (iv * math.Sqrt(t))
	pdfD1 := math.Exp(-0.5*d1*d1) / math.Sqrt(2*math.Pi)
	
	gamma := pdfD1 / (spot * iv * math.Sqrt(t))
	return gamma
}

func GetAlpacaConfig() (string, string) {
	key := os.Getenv("ALPACA_API_KEY")
	secret := os.Getenv("ALPACA_API_SECRET")

	// If the value looks like a path or if we have a _FILE env var, read it
	// First check direct env vars
	if key != "" && (strings.HasPrefix(key, "/run/secrets/") || strings.HasPrefix(key, "./") || strings.HasPrefix(key, "/")) {
		data, err := os.ReadFile(key)
		if err == nil {
			key = strings.TrimSpace(string(data))
		}
	}

	if secret != "" && (strings.HasPrefix(secret, "/run/secrets/") || strings.HasPrefix(secret, "./") || strings.HasPrefix(secret, "/")) {
		data, err := os.ReadFile(secret)
		if err == nil {
			secret = strings.TrimSpace(string(data))
		}
	}

	// Also check _FILE suffixes (project's existing pattern)
	if key == "" {
		if keyFile := os.Getenv("ALPACA_API_KEY_FILE"); keyFile != "" {
			data, err := os.ReadFile(keyFile)
			if err == nil {
				key = strings.TrimSpace(string(data))
			}
		}
	}

	if secret == "" {
		if secretFile := os.Getenv("ALPACA_API_SECRET_FILE"); secretFile != "" {
			data, err := os.ReadFile(secretFile)
			if err == nil {
				secret = strings.TrimSpace(string(data))
			}
		}
	}

	return key, secret
}

// Option represents an individual option in the chain
type Option struct {
	Strike       float64 `json:"strike"`
	OptionType   string  `json:"option_type"`
	OpenInterest int     `json:"open_interest"`
	Greeks       struct {
		Gamma float64 `json:"gamma"`
	} `json:"greeks"`
	ExpirationDate string `json:"expiration_date"`
	ExpirationType string `json:"expiration_type"`
}

// Response represents the JSON structure of the option chain
type Response struct {
	Options struct {
		Option []Option `json:"option"`
	} `json:"options"`
}

func GetSpotPrice(apiKey, apiSecret, symbol string) (float64, error) {
	mdClient := marketdata.NewClient(marketdata.ClientOpts{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   "https://data.alpaca.markets",
	})

	snapshot, err := mdClient.GetSnapshot(symbol, marketdata.GetSnapshotRequest{})
	if err != nil {
		return 0, fmt.Errorf("error getting snapshot: %v", err)
	}

	if snapshot == nil || snapshot.LatestQuote == nil {
		return 0, fmt.Errorf("no quote available for %s", symbol)
	}

	return snapshot.LatestQuote.BidPrice, nil
}

// FetchOptionsChain fetches the options chain for the given symbol and expiration date using Alpaca
func FetchOptionsChain(symbol, expiration string, apiKey, apiSecret string) ([]Option, *string, error) {
	mdClient := marketdata.NewClient(marketdata.ClientOpts{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   "https://data.alpaca.markets",
	})

	tradeClient := alpaca.NewClient(alpaca.ClientOpts{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   "https://paper-api.alpaca.markets",
	})

	expDate, err := civil.ParseDate(expiration)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing expiration date: %v", err)
	}

	// 1. Get all contracts for the symbol + expiration to get OpenInterest and Strikes
	contracts, err := tradeClient.GetOptionContracts(alpaca.GetOptionContractsRequest{
		UnderlyingSymbols: symbol,
		ExpirationDate:    expDate,
		Status:            alpaca.OptionStatusActive,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error getting contracts: %v", err)
	}

	if len(contracts) == 0 {
		return nil, nil, fmt.Errorf("no contracts found for %s on %s", symbol, expiration)
	}

	// 2. Get Snapshots for the option chain to get Greeks
	chain, err := mdClient.GetOptionChain(symbol, marketdata.GetOptionChainRequest{
		ExpirationDate: expDate,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error getting option chain snapshots: %v", err)
	}

	// Get spot price for estimation if needed
	spotPrice, err := GetSpotPrice(apiKey, apiSecret, symbol)
	if err != nil {
		fmt.Printf("Warning: failed to get spot price for gamma estimation: %v\n", err)
	}
	
	now := time.Now()
	expTime := time.Date(expDate.Year, expDate.Month, expDate.Day, 16, 0, 0, 0, time.UTC)
	daysToExpiry := expTime.Sub(now).Hours() / 24.0
	if daysToExpiry < 0.01 {
		daysToExpiry = 0.01 // handle 0DTE as small positive for math
	}

	fmt.Printf("Fetched %d snapshots from Alpaca market data for %s\n", len(chain), symbol)

	// 3. Combine contract data with snapshots
	var options []Option
	greeksFound := 0
	greeksEstimated := 0
	for _, c := range contracts {
		oi := 0
		if c.OpenInterest != nil {
			oi = int(c.OpenInterest.InexactFloat64())
		}
		opt := Option{
			Strike:         c.StrikePrice.InexactFloat64(),
			OptionType:     string(c.Type),
			OpenInterest:   oi,
			ExpirationDate: c.ExpirationDate.String(),
			ExpirationType: string(c.Style),
		}

		if snap, ok := chain[c.Symbol]; ok {
			if snap.Greeks != nil && snap.Greeks.Gamma != 0 {
				opt.Greeks.Gamma = snap.Greeks.Gamma
				greeksFound++
			} else if snap.ImpliedVolatility > 0 && spotPrice > 0 {
				// Estimate gamma if Alpaca didn't provide it but has IV
				opt.Greeks.Gamma = EstimateGamma(spotPrice, opt.Strike, snap.ImpliedVolatility, daysToExpiry)
				if opt.Greeks.Gamma > 0 {
					greeksEstimated++
				}
			}
		}
		options = append(options, opt)
	}
	fmt.Printf("Matched %d options with provided Greeks, estimated %d more, out of %d contracts\n", greeksFound, greeksEstimated, len(contracts))

	// Wrap in Response struct for JSON compatibility
	resp := Response{}
	resp.Options.Option = options
	jsonData, err := json.Marshal(resp)
	if err != nil {
		return options, nil, fmt.Errorf("error marshalling options to JSON: %v", err)
	}
	bodyStr := string(jsonData)

	return options, &bodyStr, nil
}

func GetExpirationDates(apiKey, apiSecret, symbol string) ([]string, error) {
	tradeClient := alpaca.NewClient(alpaca.ClientOpts{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   "https://paper-api.alpaca.markets",
	})

	now := civil.DateOf(time.Now())
	contracts, err := tradeClient.GetOptionContracts(alpaca.GetOptionContractsRequest{
		UnderlyingSymbols: symbol,
		ExpirationDateGTE: now,
		Status:            alpaca.OptionStatusActive,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting options contracts: %v", err)
	}

	expMap := make(map[string]bool)
	var dates []string
	for _, c := range contracts {
		dateStr := c.ExpirationDate.String()
		if !expMap[dateStr] {
			expMap[dateStr] = true
			dates = append(dates, dateStr)
		}
	}
	sort.Strings(dates)

	return dates, nil
}

func CalculateGEXPerStrike(options []Option, spotPrice float64) map[float64]float64 {
	gexByStrike := make(map[float64]float64)

	for _, option := range options {
		if option.OpenInterest > 0 && option.Greeks.Gamma != 0 {
			// Calculate GEX using the correct formula
			gex := float64(option.OpenInterest) * option.Greeks.Gamma * (spotPrice * spotPrice)

			// Add or subtract based on option type
			if option.OptionType == "call" {
				gexByStrike[option.Strike] += gex
			} else if option.OptionType == "put" {
				gexByStrike[option.Strike] -= gex
			}
		}
	}

	return gexByStrike
}

func CalculateGammaFlipLevel(gexByStrike map[float64]float64) float64 {
	if len(gexByStrike) == 0 {
		return 0
	}

	totalAbsGEX := 0.0
	for _, gex := range gexByStrike {
		totalAbsGEX += abs(gex)
	}
	
	threshold := totalAbsGEX * 0.001
	
	filteredStrikes := make([]float64, 0)
	for strike, gex := range gexByStrike {
		if abs(gex) > threshold {
			filteredStrikes = append(filteredStrikes, strike)
		}
	}
	
	if len(filteredStrikes) == 0 {
		return 0
	}
	
	sort.Float64s(filteredStrikes)
	
	cumGEX := make([]float64, len(filteredStrikes))
	sum := 0.0
	for i, strike := range filteredStrikes {
		sum += gexByStrike[strike]
		cumGEX[i] = sum
	}
	
	flipStrike := filteredStrikes[0]
	minDist := abs(cumGEX[0])
	
	for i := 0; i < len(filteredStrikes)-1; i++ {
		if (cumGEX[i] < 0 && cumGEX[i+1] > 0) || (cumGEX[i] > 0 && cumGEX[i+1] < 0) {
			if abs(cumGEX[i]) < abs(cumGEX[i+1]) {
				flipStrike = filteredStrikes[i]
			} else {
				flipStrike = filteredStrikes[i+1]
			}
			break
		}
		
		if abs(cumGEX[i]) < minDist {
			minDist = abs(cumGEX[i])
			flipStrike = filteredStrikes[i]
		}
	}
	
	return flipStrike
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func CreateGEXPlot(gexByStrike map[float64]float64, symbol string, path string, spotPrice float64) error {
	p := plot.New()
	p.Title.Text = fmt.Sprintf("GEX Distribution for %s, Spot Price %f", symbol, spotPrice)
	p.X.Label.Text = "Strike Price"
	p.Y.Label.Text = "GEX"

	var strikes []float64
	for strike, gex := range gexByStrike {
		if gex < -100 || gex > 100 {
			strikes = append(strikes, strike)
		}
	}
	sort.Float64s(strikes)

	var posPoints plotter.Values
	var negPoints plotter.Values
	var labels []string

	for _, strike := range strikes {
		gex := gexByStrike[strike]
		if gex >= 0 && gex > 5000 {
			labels = append(labels, fmt.Sprintf("%.2f", strike))
			posPoints = append(posPoints, gex)
			negPoints = append(negPoints, 0)
		} else if gex < -5000 {
			labels = append(labels, fmt.Sprintf("%.2f", strike))
			posPoints = append(posPoints, 0)
			negPoints = append(negPoints, gex)
		}
	}

	posBar, err := plotter.NewBarChart(posPoints, vg.Points(5))
	if err != nil {
		return err
	}
	posBar.Color = color.RGBA{G: 255, A: 255}
	posBar.Offset = vg.Points(0)

	negBar, err := plotter.NewBarChart(negPoints, vg.Points(5))
	if err != nil {
		return err
	}
	negBar.Color = color.RGBA{R: 255, A: 255}
	negBar.Offset = vg.Points(0)

	labelPoints := make(plotter.XYs, len(labels))
	for i := range labels {
		labelPoints[i].X = float64(i)
		labelPoints[i].Y = posPoints[i] + negPoints[i]
	}
	barLabels, err := plotter.NewLabels(plotter.XYLabels{
		XYs:    labelPoints,
		Labels: labels,
	})
	if err != nil {
		return err
	}

	p.Add(posBar, negBar, barLabels)

	p.Y.Tick.Marker = plot.TickerFunc(func(min, max float64) []plot.Tick {
		ticks := plot.DefaultTicks{}.Ticks(min, max)
		for i := range ticks {
			ticks[i].Label = fmt.Sprintf("$%.9f", ticks[i].Value)
		}
		return ticks
	})
	p.NominalX(makeXAxisLabels(strikes)...)

	return p.Save(16*vg.Inch, 8*vg.Inch, path)
}

func makeXAxisLabels(strikePrices []float64) []string {
	step := 5
	labels := make([]string, len(strikePrices))
	for i := range strikePrices {
		if i%step == 0 {
			labels[i] = ""
		} else {
			labels[i] = ""
		}
	}
	return labels
}
