package worker

import (
	"context"
	json "encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/arnabmitra/eth-proxy/internal/handler"
	"github.com/arnabmitra/eth-proxy/internal/handler/gex"
)

type GexCollector struct {
	gexHandler     *handler.GEXHandler
	symbols        []string
	interval       time.Duration
	stop           chan struct{}
	maxConcurrent  int
	rateLimitDelay time.Duration
}

func NewGEXCollector(gexHandler *handler.GEXHandler, symbols []string, interval time.Duration) *GexCollector {
	if interval == 0 {
		interval = 30 * time.Minute // Default to 15 minutes
	}
	return &GexCollector{
		gexHandler:     gexHandler,
		symbols:        symbols,
		interval:       interval,
		stop:           make(chan struct{}),
		maxConcurrent:  5, // Process 5 stocks concurrently to avoid rate limits
		rateLimitDelay: 2 * time.Second,
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

// isMarketOpen checks if the US stock market is currently open (7:30 AM - 4:00 PM ET, Mon-Fri).
func isMarketOpen() bool {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		fmt.Printf("Error loading location: %v\n", err)
		return false // Default to not open if location cannot be loaded
	}

	now := time.Now().In(loc)
	weekday := now.Weekday()
	hour := now.Hour()
	minute := now.Minute()

	// Check if it's a weekday (Monday to Friday)
	if weekday < time.Monday || weekday > time.Friday {
		return false
	}

	// Check if it's within market hours (9:30 AM to 4:00 PM ET)
	// Market opens at 9:30
	if hour < 9 || (hour == 9 && minute < 30) {
		return false
	}

	// Market closes at 4:00 PM ET
	if hour >= 16 {
		return false
	}

	return true
}

type gexResult struct {
	symbol string
	err    error
}

func (c *GexCollector) collectGEXData() {
	if !isMarketOpen() {
		fmt.Printf("Market is closed. Skipping GEX data collection.\\n")
		return
	}
	startTime := time.Now()
	fmt.Printf("[%s] Starting GEX data collection for %d symbols\\n", startTime.Format(time.RFC3339), len(c.symbols))

	// Create channels for work distribution and results
	symbolChan := make(chan string, len(c.symbols))
	resultChan := make(chan gexResult, len(c.symbols))

	// Create context with timeout for entire operation
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Start worker goroutines
	for i := 0; i < c.maxConcurrent; i++ {
		go c.worker(ctx, symbolChan, resultChan)
	}

	// Send all symbols to the work channel
	for _, symbol := range c.symbols {
		symbolChan <- symbol
	}
	close(symbolChan)

	// Collect results
	successCount := 0
	errorCount := 0
	for i := 0; i < len(c.symbols); i++ {
		result := <-resultChan
		if result.err != nil {
			errorCount++
			fmt.Printf("[%s] Error collecting GEX for %s: %v\\n",
				time.Now().Format(time.RFC3339), result.symbol, result.err)
		} else {
			successCount++
		}
	}

	fmt.Printf("[%s] Completed GEX data collection in %v. Success: %d, Errors: %d\\n",
		time.Now().Format(time.RFC3339), time.Since(startTime), successCount, errorCount)
}

func (c *GexCollector) worker(ctx context.Context, symbolChan <-chan string, resultChan chan<- gexResult) {
	apiKey := os.Getenv("TRADIER_API_KEY")
	if apiKey == "" {
		return
	}

	for symbol := range symbolChan {
		// Check if market is still open
		if !isMarketOpen() {
			resultChan <- gexResult{symbol: symbol, err: fmt.Errorf("market closed")}
			continue
		}

		// Rate limiting delay
		time.Sleep(c.rateLimitDelay)

		err := c.collectSymbolGEX(ctx, symbol, apiKey)
		resultChan <- gexResult{symbol: symbol, err: err}
	}
}

func (c *GexCollector) collectSymbolGEX(ctx context.Context, symbol string, apiKey string) error {
	// Get the nearest expiry date
	expiryDates, err := c.gexHandler.GetExpiryDates(ctx, symbol)
	if err != nil || len(expiryDates) == 0 {
		expirationDates, err := gex.GetExpirationDates(apiKey, symbol)
		if err != nil {
			return fmt.Errorf("failed to get expiration dates: %w", err)
		}

		expirationDatesJSON, err := json.MarshalIndent(expirationDates, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal expiration dates: %w", err)
		}

		err = c.gexHandler.StoreExpiryDatesInOptionExpiryDates(ctx, symbol, expirationDatesJSON)
		if err != nil {
			return fmt.Errorf("failed to store expiry dates: %w", err)
		}

		expiryDates, err = c.gexHandler.GetExpiryDates(ctx, symbol)
		if err != nil {
			return fmt.Errorf("failed to get stored expiry dates: %w", err)
		}
	}

	if len(expiryDates) == 0 {
		return fmt.Errorf("no expiry dates available")
	}

	// Use only the nearest expiry date
	nearestExpiry := expiryDates[0]

	// Get current price
	price, err := gex.GetSpotPrice(apiKey, symbol)
	if err != nil {
		// Check if it's a rate limit error (403)
		if err.Error() == "unexpected status code: 403" {
			time.Sleep(5 * time.Second) // Back off on rate limit
			// Retry once
			price, err = gex.GetSpotPrice(apiKey, symbol)
			if err != nil {
				return fmt.Errorf("failed to get spot price after retry: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get spot price: %w", err)
		}
	}

	// Fetch options chain
	options, jsonOption, err := gex.FetchOptionsChain(symbol, nearestExpiry, apiKey)
	if err != nil {
		// Check if it's a rate limit error
		if err.Error() == "unexpected status code: 403" {
			time.Sleep(5 * time.Second)
			// Retry once
			options, jsonOption, err = gex.FetchOptionsChain(symbol, nearestExpiry, apiKey)
			if err != nil {
				return fmt.Errorf("failed to fetch options chain after retry: %w", err)
			}
		} else {
			return fmt.Errorf("failed to fetch options chain: %w", err)
		}
	}

	if jsonOption == nil {
		return fmt.Errorf("nil options chain returned")
	}

	// Calculate GEX for the nearest expiry
	gexByStrike := gex.CalculateGEXPerStrike(options, price)

	// Calculate total GEX (sum of all strikes)
	totalGEX := 0.0
	for _, gexValue := range gexByStrike {
		totalGEX += gexValue
	}

	fmt.Printf("[%s] %s: Total GEX=%.2f, Price=%.2f, Expiry=%s\\n",
		time.Now().Format(time.RFC3339), symbol, totalGEX, price, nearestExpiry)

	// Store in the database
	err = c.gexHandler.StoreOptionChain(ctx, options, symbol, *jsonOption, fmt.Sprintf("%.2f", price), fmt.Sprintf("%.2f", totalGEX))
	if err != nil {
		return fmt.Errorf("failed to store option chain: %w", err)
	}

	return nil
}
