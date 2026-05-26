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
	"github.com/arnabmitra/eth-proxy/internal/public"
	"os"
	"strings"
	"math"
)

// N calculates the cumulative distribution function for a standard normal distribution
func N(x float64) float64 {
	return 0.5 * (1 + math.Erf(x/math.Sqrt(2)))
}

// n_pdf calculates the probability density function for a standard normal distribution
func n_pdf(x float64) float64 {
	return math.Exp(-0.5*x*x) / math.Sqrt(2*math.Pi)
}

// BlackScholesPrice calculates the theoretical price of an option
func BlackScholesPrice(spot, strike, iv, t, r float64, isCall bool) float64 {
	if t <= 0 {
		if isCall {
			return math.Max(0, spot-strike)
		}
		return math.Max(0, strike-spot)
	}
	d1 := (math.Log(spot/strike) + (r+0.5*iv*iv)*t) / (iv * math.Sqrt(t))
	d2 := d1 - iv*math.Sqrt(t)

	if isCall {
		return spot*N(d1) - strike*math.Exp(-r*t)*N(d2)
	}
	return strike*math.Exp(-r*t)*N(-d2) - spot*N(-d1)
}

// CalculateIV finds the implied volatility using Newton-Raphson
func CalculateIV(marketPrice, spot, strike, t, r float64, isCall bool) float64 {
	if marketPrice <= 0 || t <= 0 {
		return 0
	}

	// Initial guess
	iv := 0.5
	for i := 0; i < 20; i++ {
		price := BlackScholesPrice(spot, strike, iv, t, r, isCall)
		diff := price - marketPrice
		if math.Abs(diff) < 1e-6 {
			return iv
		}

		// Calculate Vega
		d1 := (math.Log(spot/strike) + (r+0.5*iv*iv)*t) / (iv * math.Sqrt(t))
		vega := spot * math.Sqrt(t) * n_pdf(d1)
		
		if vega < 1e-6 {
			break
		}
		
		iv = iv - diff/vega
		if iv <= 0 {
			iv = 0.0001 // prevent negative/zero IV
		}
	}
	return iv
}

// BlackScholesGamma calculates an approximate gamma if the provider doesn't supply it
func EstimateGamma(spot, strike, iv, daysToExpiration float64) float64 {
	if iv <= 0 || daysToExpiration <= 0 || spot <= 0 {
		return 0
	}

	t := daysToExpiration / 365.0
	r := 0.05 // assume 5% risk free rate
	
	d1 := (math.Log(spot/strike) + (r+0.5*iv*iv)*t) / (iv * math.Sqrt(t))
	pdfD1 := n_pdf(d1)
	
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

func GetPublicConfig() (string, string) {
	secret := os.Getenv("PUBLIC_PERSONAL_SECRET")
	if secret == "" {
		secret = os.Getenv("PUBLIC_SECRET_KEY")
	}
	accountID := os.Getenv("PUBLIC_ACCOUNT_ID")
	return secret, accountID
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
	Warning string `json:"warning,omitempty"`
}

func GetSpotPrice(apiKey, apiSecret, symbol string) (float64, error) {
	// Try Public.com first if secret is available
	publicSecret, publicAccountID := GetPublicConfig()
	if publicSecret != "" {
		fmt.Printf("Attempting to fetch spot price from Public.com for %s...\n", symbol)
		client := public.NewClient(publicSecret, publicAccountID)
		if err := client.Authenticate(); err == nil {
			if err := client.FetchAccountID(); err == nil {
				price, err := client.GetSpotPrice(symbol)
				if err == nil {
					fmt.Printf("Successfully fetched spot price from Public.com: %.2f\n", price)
					return price, nil
				}
				fmt.Printf("Public.com spot price fetch failed: %v\n", err)
			} else {
				fmt.Printf("Public.com account ID fetch failed: %v\n", err)
			}
		} else {
			fmt.Printf("Public.com authentication failed: %v\n", err)
		}
		fmt.Println("Falling back to Alpaca for spot price.")
	}

	mdClient := marketdata.NewClient(marketdata.ClientOpts{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   "https://data.alpaca.markets",
	})

	snapshot, err := mdClient.GetSnapshot(symbol, marketdata.GetSnapshotRequest{})
	if err != nil {
		return 0, fmt.Errorf("error getting snapshot: %v", err)
	}

	if snapshot == nil {
		return 0, fmt.Errorf("no snapshot available for %s", symbol)
	}

	// Try Latest Trade first as it is more reliable for "last price" than Bid/Ask at market close
	if snapshot.LatestTrade != nil && snapshot.LatestTrade.Price > 0 {
		return snapshot.LatestTrade.Price, nil
	}

	if snapshot.LatestQuote != nil {
		if snapshot.LatestQuote.BidPrice > 0 && snapshot.LatestQuote.AskPrice > 0 {
			return (snapshot.LatestQuote.BidPrice + snapshot.LatestQuote.AskPrice) / 2.0, nil
		}
		if snapshot.LatestQuote.BidPrice > 0 {
			return snapshot.LatestQuote.BidPrice, nil
		}
	}

	return 0, fmt.Errorf("no valid price data available for %s", symbol)
}

// FetchOptionsChain fetches the options chain for the given symbol and expiration date using Alpaca
func FetchOptionsChain(symbol, expiration string, apiKey, apiSecret string) ([]Option, *string, string, error) {
	publicSecret, publicAccountID := GetPublicConfig()
	if publicSecret != "" {
		fmt.Printf("Attempting to fetch options chain from Public.com for %s (%s)...\n", symbol, expiration)
		client := public.NewClient(publicSecret, publicAccountID)
		if err := client.Authenticate(); err == nil {
			if err := client.FetchAccountID(); err == nil {
				chain, err := client.GetOptionChain(symbol, expiration)
				if err == nil {
					fmt.Printf("Successfully fetched option chain from Public.com for %s\n", symbol)
					var options []Option
					for _, c := range chain.Options {
						opt := Option{
							Strike:         c.StrikePrice,
							OptionType:     c.OptionType,
							OpenInterest:   c.OpenInterest,
							ExpirationDate: expiration,
							ExpirationType: "AMERICAN",
						}
						if c.Greeks != nil {
							opt.Greeks.Gamma = c.Greeks.Gamma
						}
						options = append(options, opt)
					}

					resp := Response{}
					resp.Options.Option = options
					jsonData, _ := json.Marshal(resp)
					bodyStr := string(jsonData)
					return options, &bodyStr, "", nil
				} else {
					fmt.Printf("Public.com GetOptionChain failed: %v\n", err)
				}
			} else {
				fmt.Printf("Public.com account ID fetch failed: %v\n", err)
			}
		} else {
			fmt.Printf("Public.com authentication failed: %v\n", err)
		}
		fmt.Println("Falling back to Alpaca for options chain.")
	}

	// Try Live API first for contracts
	liveBaseUrl := "https://api.alpaca.markets"
	
	mdClient := marketdata.NewClient(marketdata.ClientOpts{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   "https://data.alpaca.markets",
	})

	tradeClient := alpaca.NewClient(alpaca.ClientOpts{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   liveBaseUrl,
	})

	expDate, err := civil.ParseDate(expiration)
	if err != nil {
		return nil, nil, "", fmt.Errorf("error parsing expiration date: %v", err)
	}

	// 1. Get all contracts for the symbol + expiration to get OpenInterest and Strikes
	contracts, err := tradeClient.GetOptionContracts(alpaca.GetOptionContractsRequest{
		UnderlyingSymbols: symbol,
		ExpirationDate:    expDate,
		Status:            alpaca.OptionStatusActive,
	})

	isPaperFallback := false
	if err != nil || len(contracts) == 0 {
		fmt.Printf("Live API returned 0 contracts for %s on %s, falling back to Paper API\n", symbol, expiration)
		paperBaseUrl := "https://paper-api.alpaca.markets"
		tradeClient = alpaca.NewClient(alpaca.ClientOpts{
			APIKey:    apiKey,
			APISecret: apiSecret,
			BaseURL:   paperBaseUrl,
		})
		contracts, err = tradeClient.GetOptionContracts(alpaca.GetOptionContractsRequest{
			UnderlyingSymbols: symbol,
			ExpirationDate:    expDate,
			Status:            alpaca.OptionStatusActive,
		})
		if err == nil && len(contracts) > 0 {
			isPaperFallback = true
		}
	}

	if err != nil {
		return nil, nil, "", fmt.Errorf("error getting contracts: %v", err)
	}

	if len(contracts) == 0 {
		return nil, nil, "", fmt.Errorf("no contracts found for %s on %s (tried both Live and Paper)", symbol, expiration)
	}

	// 2. Get Snapshots for the option chain to get Greeks
	chain, err := mdClient.GetOptionChain(symbol, marketdata.GetOptionChainRequest{
		ExpirationDate: expDate,
	})
	if err != nil {
		return nil, nil, "", fmt.Errorf("error getting option chain snapshots: %v", err)
	}

	// Check underlying spread for warning
	warning := ""
	if isPaperFallback {
		warning = "Data Quality Warning: Using Paper API for contract data (Live API returned no results). Greeks are re-estimated using live spot price."
	}

	snapshot, err := mdClient.GetSnapshot(symbol, marketdata.GetSnapshotRequest{})
	if err == nil && snapshot != nil && snapshot.LatestQuote != nil {
		bid := snapshot.LatestQuote.BidPrice
		ask := snapshot.LatestQuote.AskPrice
		if ask > 0 {
			mid := (bid + ask) / 2.0
			spread := (ask - bid) / mid
			if spread > 0.01 {
				highSpreadWarning := fmt.Sprintf("High Bid-Ask spread (%.2f%%) for underlying %s. Data may be indicative or low-confidence.", spread*100, symbol)
				if warning != "" {
					warning += " " + highSpreadWarning
				} else {
					warning = highSpreadWarning
				}
			}
		}
	}

	// Get spot price for estimation if needed
	spotPrice, err := GetSpotPrice(apiKey, apiSecret, symbol)
	if err != nil {
		fmt.Printf("Warning: failed to get spot price for gamma estimation: %v\n", err)
	}
	
	loc, _ := time.LoadLocation("America/New_York")
	now := time.Now().In(loc)
	// Market close is 4 PM ET
	expTime := time.Date(expDate.Year, expDate.Month, expDate.Day, 16, 0, 0, 0, loc)
	daysToExpiry := expTime.Sub(now).Hours() / 24.0
	
	if daysToExpiry < 0 && daysToExpiry > -16.0/24.0 {
		// If it's the same day but after 4 PM ET, still treat as 0DTE for a few hours
		daysToExpiry = 0.001
	} else if daysToExpiry <= 0 {
		daysToExpiry = 0.001
	}

	fmt.Printf("Fetched %d snapshots from Alpaca market data for %s (Days to Expiry: %.4f, Paper Fallback: %v)\n", len(chain), symbol, daysToExpiry, isPaperFallback)

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
			// Re-estimate if Greeks are missing OR if it's from a delayed feed (Paper Fallback)
			if snap.Greeks != nil && snap.Greeks.Gamma != 0 && !isPaperFallback {
				opt.Greeks.Gamma = snap.Greeks.Gamma
				greeksFound++
			} else if spotPrice > 0 {
				// Fallback: Estimate gamma from market price if Greeks are missing or suspect
				iv := snap.ImpliedVolatility
				
				// If IV is also missing or suspect, calculate it from price
				if (iv <= 0 || isPaperFallback) && snap.LatestQuote != nil && snap.LatestQuote.BidPrice > 0 {
					marketPrice := (snap.LatestQuote.BidPrice + snap.LatestQuote.AskPrice) / 2.0
					isCall := strings.ToLower(opt.OptionType) == "call"
					t := daysToExpiry / 365.0
					iv = CalculateIV(marketPrice, spotPrice, opt.Strike, t, 0.05, isCall)
				}

				if iv > 0 {
					opt.Greeks.Gamma = EstimateGamma(spotPrice, opt.Strike, iv, daysToExpiry)
					if opt.Greeks.Gamma > 0 {
						greeksEstimated++
					}
				}
			}
		}
		options = append(options, opt)
	}
	fmt.Printf("Matched %d options with provided Greeks, estimated %d more, out of %d contracts\n", greeksFound, greeksEstimated, len(contracts))

	// Wrap in Response struct for JSON compatibility
	resp := Response{}
	resp.Options.Option = options
	resp.Warning = warning
	jsonData, err := json.Marshal(resp)
	if err != nil {
		return options, nil, warning, fmt.Errorf("error marshalling options to JSON: %v", err)
	}
	bodyStr := string(jsonData)

	return options, &bodyStr, warning, nil
}

func GetExpirationDates(apiKey, apiSecret, symbol string) ([]string, error) {
	publicSecret, publicAccountID := GetPublicConfig()
	if publicSecret != "" {
		fmt.Printf("Attempting to fetch expiration dates from Public.com for %s...\n", symbol)
		client := public.NewClient(publicSecret, publicAccountID)
		if err := client.Authenticate(); err == nil {
			if err := client.FetchAccountID(); err == nil {
				dates, err := client.GetExpirations(symbol)
				if err == nil {
					fmt.Printf("Successfully fetched %d expiration dates from Public.com\n", len(dates))
					sort.Strings(dates)
					return dates, nil
				}
				fmt.Printf("Public.com expiration fetch failed: %v\n", err)
			}
		}
		fmt.Println("Falling back to Alpaca for expiration dates.")
	}

	baseUrl := os.Getenv("ALPACA_API_BASE_URL")
	if baseUrl == "" {
		baseUrl = "https://api.alpaca.markets"
	}

	tradeClient := alpaca.NewClient(alpaca.ClientOpts{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   baseUrl,
	})

	loc, _ := time.LoadLocation("America/New_York")
	nowNY := time.Now().In(loc)
	today := civil.DateOf(nowNY)
	
	fmt.Printf("Fetching expirations for %s starting from %s (NY Time: %s)\n", symbol, today, nowNY.Format(time.RFC3339))

	contracts, err := tradeClient.GetOptionContracts(alpaca.GetOptionContractsRequest{
		UnderlyingSymbols: symbol,
		ExpirationDateGTE: today,
		Status:            alpaca.OptionStatusActive,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting options contracts: %v", err)
	}

	fmt.Printf("Alpaca returned %d active contracts for %s\n", len(contracts), symbol)

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
			// GEX = OpenInterest * Gamma * 100 * SpotPrice (Dollar Gamma Exposure)
			// This represents the dollar value change in the delta-hedged position for a 1% move in underlying.
			gex := float64(option.OpenInterest) * option.Greeks.Gamma * 100 * spotPrice

			// Add or subtract based on option type
			optionType := strings.ToLower(option.OptionType)
			if optionType == "call" {
				gexByStrike[option.Strike] += gex
			} else if optionType == "put" {
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
