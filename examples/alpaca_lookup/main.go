package main

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"cloud.google.com/go/civil"
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
func EstimateGamma(spot, strike, iv, t float64) float64 {
	if iv <= 0 || t <= 0 || spot <= 0 {
		return 0
	}

	r := 0.05 // assume 5% risk free rate
	
	d1 := (math.Log(spot/strike) + (r+0.5*iv*iv)*t) / (iv * math.Sqrt(t))
	pdfD1 := n_pdf(d1)
	
	gamma := pdfD1 / (spot * iv * math.Sqrt(t))
	return gamma
}

func main() {
	apiKey := os.Getenv("ALPACA_API_KEY")
	apiSecret := os.Getenv("ALPACA_API_SECRET")
	
	symbol := "SPY"
	expStr := "2026-04-02"
	if len(os.Args) > 1 {
		symbol = os.Args[1]
	}
	if len(os.Args) > 2 {
		expStr = os.Args[2]
	}

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

	expDate, _ := civil.ParseDate(expStr)

	fmt.Printf("--- Fetching data for %s (Exp: %s) ---\n", symbol, expStr)

	// 1. Get Spot Price
	snapshot, _ := mdClient.GetSnapshot(symbol, marketdata.GetSnapshotRequest{})
	spotPrice := snapshot.LatestQuote.BidPrice
	fmt.Printf("Spot Price: %.2f\n", spotPrice)

	// 2. Get Contracts
	contracts, _ := tradeClient.GetOptionContracts(alpaca.GetOptionContractsRequest{
		UnderlyingSymbols: symbol,
		ExpirationDate:    expDate,
		Status:            alpaca.OptionStatusActive,
	})

	// 3. Get Chain Snapshots
	chain, _ := mdClient.GetOptionChain(symbol, marketdata.GetOptionChainRequest{
		ExpirationDate: expDate,
	})

	loc, _ := time.LoadLocation("America/New_York")
	now := time.Now().In(loc)
	expTime := time.Date(expDate.Year, expDate.Month, expDate.Day, 16, 0, 0, 0, loc)
	t := expTime.Sub(now).Hours() / 24.0 / 365.0
	if t <= 0 { t = 0.0001 / 365.0 }

	greeksFound := 0
	greeksEstimated := 0
	totalGex := 0.0

	for _, c := range contracts {
		if snap, ok := chain[c.Symbol]; ok {
			gamma := 0.0
			if snap.Greeks != nil && snap.Greeks.Gamma != 0 {
				gamma = snap.Greeks.Gamma
				greeksFound++
			} else {
				// Estimate
				iv := snap.ImpliedVolatility
				if iv <= 0 && snap.LatestQuote != nil && snap.LatestQuote.BidPrice > 0 {
					marketPrice := (snap.LatestQuote.BidPrice + snap.LatestQuote.AskPrice) / 2.0
					isCall := c.Type == "call"
					iv = CalculateIV(marketPrice, spotPrice, c.StrikePrice.InexactFloat64(), t*365.0, 0.05, isCall)
				}
				if iv > 0 {
					gamma = EstimateGamma(spotPrice, c.StrikePrice.InexactFloat64(), iv, t*365.0)
					greeksEstimated++
				}
			}

			if gamma > 0 {
				oi := 0.0
				if c.OpenInterest != nil {
					oi, _ = c.OpenInterest.Float64()
				}
				gex := oi * gamma * (spotPrice * spotPrice)
				if c.Type == "call" {
					totalGex += gex
				} else {
					totalGex -= gex
				}
			}
		}
	}

	fmt.Printf("\nSummary for %s %s:\n", symbol, expStr)
	fmt.Printf("Contracts: %d\n", len(contracts))
	fmt.Printf("Greeks Provided: %d\n", greeksFound)
	fmt.Printf("Greeks Estimated: %d\n", greeksEstimated)
	fmt.Printf("Total GEX: %.2f\n", totalGex)
}
