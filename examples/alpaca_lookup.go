package main

import (
	"fmt"
	"log"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"cloud.google.com/go/civil"
)

func main() {
	apiKey := "PK5QDVG6M44WI2LBZNXKPB4Y4E"
	apiSecret := "2srHM4WVZSZVEhcBweAVhsvzoRtKnUPMtQYhUEMFJkzq"
	symbol := "SPY"

	// 1. Initialize Clients
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

	fmt.Printf("--- Fetching data for %s ---\n", symbol)

	// 1. Get Spot Price (Underlying)
	snapshot, err := mdClient.GetSnapshot(symbol, marketdata.GetSnapshotRequest{})
	if err != nil {
		log.Printf("Error getting snapshot for %s: %v", symbol, err)
	} else if snapshot != nil && snapshot.LatestQuote != nil {
		fmt.Printf("Spot Price: %.2f (Bid: %.2f, Ask: %.2f)\n", snapshot.LatestQuote.BidPrice, snapshot.LatestQuote.BidPrice, snapshot.LatestQuote.AskPrice)
	}

	// 2. Get Expiration Dates
	fmt.Println("\n--- Expiration Dates ---")
	
	now := civil.DateOf(time.Now())
	contracts, err := tradeClient.GetOptionContracts(alpaca.GetOptionContractsRequest{
		UnderlyingSymbols: symbol,
		ExpirationDateGTE: now,
		Status:            alpaca.OptionStatusActive,
	})
	if err != nil {
		log.Fatalf("Error getting options contracts: %v", err)
	}

	expirations := make(map[civil.Date]bool)
	var firstExpiration civil.Date
	for _, c := range contracts {
		if !expirations[c.ExpirationDate] {
			expirations[c.ExpirationDate] = true
			if firstExpiration.IsZero() || c.ExpirationDate.Before(firstExpiration) {
				firstExpiration = c.ExpirationDate
			}
		}
	}

	fmt.Printf("Found %d expiration dates. First: %s\n", len(expirations), firstExpiration)
	
	// 3. Get Option Chain for the first expiration
	if !firstExpiration.IsZero() {
		fmt.Printf("\n--- Option Chain for %s (Exp: %s) ---\n", symbol, firstExpiration)
		
		chain, err := mdClient.GetOptionChain(symbol, marketdata.GetOptionChainRequest{
			ExpirationDate: firstExpiration,
		})
		if err != nil {
			log.Printf("Error getting option chain: %v", err)
		} else {
			fmt.Printf("Found %d contracts in chain\n", len(chain))
			
			count := 0
			for sym, snap := range chain {
				if count < 5 {
					fmt.Printf("Contract: %s | Bid: %.2f | Ask: %.2f\n", 
						sym, snap.LatestQuote.BidPrice, snap.LatestQuote.AskPrice)
					if snap.Greeks != nil {
						fmt.Printf("  Greeks: Delta: %.4f, Gamma: %.4f, Theta: %.4f, Vega: %.4f\n", 
							snap.Greeks.Delta, snap.Greeks.Gamma, snap.Greeks.Theta, snap.Greeks.Vega)
					}
					count++
				}
			}
		}
	}
}
