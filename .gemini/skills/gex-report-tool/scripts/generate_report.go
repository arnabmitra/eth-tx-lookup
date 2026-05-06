package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"cloud.google.com/go/civil"
)

func n_pdf(x float64) float64 {
	return math.Exp(-0.5*x*x) / math.Sqrt(2*math.Pi)
}

func EstimateGamma(spot, strike, iv, t float64) float64 {
	if iv <= 0 || t <= 0 || spot <= 0 {
		return 0
	}
	r := 0.05
	d1 := (math.Log(spot/strike) + (r+0.5*iv*iv)*t) / (iv * math.Sqrt(t))
	return n_pdf(d1) / (spot * iv * math.Sqrt(t))
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: generate_report <SYMBOL> [EXPIRY_DATE] [SPOT_PRICE]")
		return
	}

	symbol := os.Args[1]
	
	apiKey := os.Getenv("ALPACA_API_KEY")
	apiSecret := os.Getenv("ALPACA_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		fmt.Println("Error: ALPACA_API_KEY and ALPACA_API_SECRET must be set")
		return
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

	// Get Spot Price
	spotPrice := 0.0
	if len(os.Args) > 3 {
		fmt.Sscanf(os.Args[3], "%f", &spotPrice)
	} else {
		snapshot, err := mdClient.GetSnapshot(symbol, marketdata.GetSnapshotRequest{})
		if err == nil && snapshot != nil && snapshot.LatestQuote != nil {
			spotPrice = snapshot.LatestQuote.BidPrice
		}
	}

	if spotPrice == 0 {
		fmt.Println("Error: Could not determine spot price")
		return
	}

	// Get Expiry
	var expStr string
	if len(os.Args) > 2 {
		expStr = os.Args[2]
	} else {
		contracts, err := tradeClient.GetOptionContracts(alpaca.GetOptionContractsRequest{
			UnderlyingSymbols: symbol,
			Status:            alpaca.OptionStatusActive,
		})
		if err == nil && len(contracts) > 0 {
			// Find nearest
			sort.Slice(contracts, func(i, j int) bool {
				return contracts[i].ExpirationDate.Before(contracts[j].ExpirationDate)
			})
			expStr = contracts[0].ExpirationDate.String()
		}
	}

	expDate, _ := civil.ParseDate(expStr)

	// Get Contracts
	contracts, _ := tradeClient.GetOptionContracts(alpaca.GetOptionContractsRequest{
		UnderlyingSymbols: symbol,
		ExpirationDate:    expDate,
		Status:            alpaca.OptionStatusActive,
	})

	// Get Chain
	chain, _ := mdClient.GetOptionChain(symbol, marketdata.GetOptionChainRequest{
		ExpirationDate: expDate,
	})

	loc, _ := time.LoadLocation("America/New_York")
	now := time.Now().In(loc)
	expTime := time.Date(expDate.Year, expDate.Month, expDate.Day, 16, 0, 0, 0, loc)
	t := expTime.Sub(now).Hours() / 24.0 / 365.0
	if t <= 0 { t = 0.001 / 365.0 }

	totalCallGex := 0.0
	totalPutGex := 0.0
	gexByStrike := make(map[float64]float64)
	ivSum := 0.0
	ivCount := 0

	for _, c := range contracts {
		strike := c.StrikePrice.InexactFloat64()
		if snap, ok := chain[c.Symbol]; ok {
			gamma := 0.0
			iv := snap.ImpliedVolatility
			if iv > 0 {
				ivSum += iv
				ivCount++
			}
			
			gamma = EstimateGamma(spotPrice, strike, iv, t*365.0)

			if gamma > 0 {
				oi := 0.0
				if c.OpenInterest != nil {
					oi, _ = c.OpenInterest.Float64()
				}
				gex := oi * gamma * 100 * spotPrice
				if c.Type == "call" {
					totalCallGex += gex
					gexByStrike[strike] += gex
				} else {
					totalPutGex += gex
					gexByStrike[strike] -= gex
				}
			}
		}
	}

	netGex := totalCallGex - totalPutGex
	totalGex := totalCallGex + math.Abs(totalPutGex)
	avgIV := 0.0
	if ivCount > 0 {
		avgIV = ivSum / float64(ivCount)
	}

	fmt.Printf("NetGEX for %s (%s)\n\n", symbol, expStr)
	fmt.Printf("Regime Summary:\n\n")
	fmt.Printf("Net GEX: %s$%.2fM (%s gamma regime)\n", func() string { if netGex < 0 { return "-" }; return "+" }(), math.Abs(netGex/1000000.0), func() string { if netGex > 0 { return "positive" } else { return "negative" }}())
	fmt.Printf("Total GEX: $%.2fM\n", totalGex/1000000.0)
	fmt.Printf("Gamma Condition: %s\n", func() string { if netGex > 0 { return "Positive" } else { return "Negative" }}())
	fmt.Printf("IV (Avg): %.2f%%\n", avgIV*100)
	fmt.Printf("Put/Call GEX Ratio: %.1f\n\n", math.Abs(totalPutGex/totalCallGex))

	fmt.Printf("GEX Distribution:\n\n")
	fmt.Printf("Above Spot (%.2f):\n", spotPrice)
	var strikes []float64
	for s := range gexByStrike { strikes = append(strikes, s) }
	sort.Float64s(strikes)

	aboveCount := 0
	for _, s := range strikes {
		if s > spotPrice && aboveCount < 3 {
			fmt.Printf("  Strike $%.2f: GEX $%.2fM\n", s, gexByStrike[s]/1000000.0)
			aboveCount++
		}
	}

	fmt.Printf("\nBelow Spot:\n")
	belowCount := 0
	belowStrikes := []float64{}
	for _, s := range strikes { if s < spotPrice { belowStrikes = append(belowStrikes, s) } }
	sort.Sort(sort.Reverse(sort.Float64Slice(belowStrikes)))
	for _, s := range belowStrikes {
		if belowCount < 3 {
			fmt.Printf("  Strike $%.2f: GEX $%.2fM\n", s, gexByStrike[s]/1000000.0)
			belowCount++
		}
	}
}
