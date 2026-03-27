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
		
		// We need to match contracts with snapshots to see OI and Greeks
		expDate := firstExpiration
		
		contracts, err := tradeClient.GetOptionContracts(alpaca.GetOptionContractsRequest{
			UnderlyingSymbols: symbol,
			ExpirationDate:    expDate,
			Status:            alpaca.OptionStatusActive,
		})
		if err != nil {
			log.Fatalf("Error getting contracts: %v", err)
		}

		chain, err := mdClient.GetOptionChain(symbol, marketdata.GetOptionChainRequest{
			ExpirationDate: expDate,
		})
		if err != nil {
			log.Printf("Error getting option chain snapshots: %v", err)
		} else {
			fmt.Printf("Found %d snapshots and %d contracts\n", len(chain), len(contracts))
			
			greeksFound := 0
			oiFound := 0
			for _, c := range contracts {
				oi := 0.0
				if c.OpenInterest != nil {
					oi, _ = c.OpenInterest.Float64()
					if oi > 0 {
						oiFound++
					}
				}

				if snap, ok := chain[c.Symbol]; ok && snap.Greeks != nil {
					greeksFound++
					if greeksFound < 10 {
						fmt.Printf("Contract: %s | OI: %.0f | Gamma: %.6f\n", 
							c.Symbol, oi, snap.Greeks.Gamma)
					}
				}
			}
			fmt.Printf("\nSummary: OI > 0: %d | Greeks Found: %d | Total Contracts: %d\n", 
				oiFound, greeksFound, len(contracts))
		}
	}
}
