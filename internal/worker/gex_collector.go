package worker

import (
	"context"
	json "encoding/json"
	"fmt"
	"github.com/arnabmitra/eth-proxy/internal/handler"
	"github.com/arnabmitra/eth-proxy/internal/handler/gex"
	"os"
	"time"
)

type GexCollector struct {
	gexHandler *handler.GEXHandler
	symbols    []string
	interval   time.Duration
	stop       chan struct{}
}

func NewGEXCollector(gexHandler *handler.GEXHandler, symbols []string) *GexCollector {
	return &GexCollector{
		gexHandler: gexHandler,
		symbols:    symbols,
		interval:   5 * time.Minute,
		stop:       make(chan struct{}),
	}
}

func (c *GexCollector) Start() {
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		// Run immediately on start
		c.collectGEXData()
		for {
			select {
			case <-ticker.C:
				c.collectGEXData()
			case <-c.stop:
				return

			}
		}
	}()
}

func (c *GexCollector) Stop() {
	close(c.stop)
}

func (c *GexCollector) collectGEXData() {
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fmt.Printf("[%s] Starting GEX data collection for symbols: %v\n", startTime.Format(time.RFC3339), c.symbols)
	for _, symbol := range c.symbols {

		// Get the nearest expiry date
		expiryDates, err := c.gexHandler.GetExpiryDates(ctx, symbol)
		if err != nil || len(expiryDates) == 0 {
			apiKey := os.Getenv("TRADIER_API_KEY")
			if apiKey == "" {
				continue
			}
			expirationDates, err := gex.GetExpirationDates(apiKey, symbol)
			if err != nil {
				return
			}

			expirationDatesJSON, err := json.MarshalIndent(expirationDates, "", "  ")
			if err != nil {
				fmt.Printf("Error marshalling expiration dates to JSON: %v\n", err)
				return
			}
			fmt.Println(string(expirationDatesJSON))

			err = c.gexHandler.StoreExpiryDatesInOptionExpiryDates(ctx, symbol, expirationDatesJSON)
			if err != nil {
				return
			}
			expiryDates, err = c.gexHandler.GetExpiryDates(ctx, symbol)
			if err != nil {
				return
			}
		}

		// Use only the nearest expiry date
		nearestExpiry := expiryDates[0]

		// Get the options chain for that expiry
		apiKey := os.Getenv("TRADIER_API_KEY")
		if apiKey == "" {
			continue
		}

		// Get current price
		price, err := gex.GetSpotPrice(apiKey, symbol)
		if err != nil {
			continue
		}

		// Fetch options chain
		options, jsonOption, err := gex.FetchOptionsChain(symbol, nearestExpiry, apiKey)
		if err != nil || jsonOption == nil {
			continue
		}

		// Calculate GEX for the nearest expiry
		gexByStrike := gex.CalculateGEXPerStrike(options, price)

		// Calculate total GEX (sum of all strikes)
		totalGEX := 0.0
		for _, gexValue := range gexByStrike {
			totalGEX += gexValue
		}
		fmt.Printf("[%s] Total GEX for %s (expiry %s): %.2f\n",
			time.Now().Format(time.RFC3339), symbol, nearestExpiry, totalGEX)

		// Store in the database
		err = c.gexHandler.StoreOptionChain(ctx, options, symbol, *jsonOption, fmt.Sprintf("%.2f", price), fmt.Sprintf("%.2f", totalGEX))
		if err != nil {
			return
		}
	}

	fmt.Printf("[%s] Completed GEX data collection in %v\n",
		time.Now().Format(time.RFC3339),
		time.Since(startTime))
}
