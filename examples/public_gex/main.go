package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/arnabmitra/eth-proxy/internal/public"
	"github.com/joho/godotenv"
)

func main() {
	symbolPtr := flag.String("symbol", "SPY", "Underlying symbol")
	expirationPtr := flag.String("exp", "", "Expiration date (YYYY-MM-DD)")
	flag.Parse()

	symbol := strings.ToUpper(*symbolPtr)
	expiration := *expirationPtr

	// Load .env
	_ = godotenv.Load()

	secret := os.Getenv("PUBLIC_PERSONAL_SECRET")
	if secret == "" {
		secret = os.Getenv("PUBLIC_SECRET_KEY")
	}
	accountID := os.Getenv("PUBLIC_ACCOUNT_ID")

	if secret == "" {
		fmt.Println("Error: PUBLIC_PERSONAL_SECRET or PUBLIC_SECRET_KEY is not set in .env or environment")
		fmt.Println("Please add it to your .env file or environment:")
		fmt.Println("PUBLIC_SECRET_KEY=your_secret_here")
		os.Exit(1)
	}

	client := public.NewClient(secret, accountID)

	fmt.Printf("Authenticating with Public.com API...\n")
	err := client.Authenticate()
	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}

	fmt.Printf("Fetching Account ID...\n")
	err = client.FetchAccountID()
	if err != nil {
		log.Fatalf("Failed to fetch Account ID: %v", err)
	}
	fmt.Printf("Using Account ID: %s\n", client.AccountID)

	fmt.Printf("Fetching spot price for %s...\n", symbol)
	spotPrice, err := client.GetSpotPrice(symbol)
	if err != nil {
		log.Fatalf("Failed to fetch spot price: %v", err)
	}
	fmt.Printf("Spot Price: %.2f\n", spotPrice)

	fmt.Printf("Fetching option chain for %s (Exp: %s)...\n", symbol, expiration)
	chain, _, err := client.GetOptionChain(symbol, expiration)
	if err != nil {
		log.Fatalf("Failed to fetch option chain: %v", err)
	}

	// Calculate GEX
	totalCallGex := 0.0
	totalPutGex := 0.0
	gexByStrike := make(map[float64]float64)

	process := func(contracts []public.OptionContract, side string) {
		for _, contract := range contracts {
			gamma := 0.0
			if contract.Greeks != nil {
				gamma, _ = contract.Greeks.Gamma.Float64()
			}

			strike, _ := contract.StrikePrice.Float64()
			oi, _ := contract.OpenInterest.Int64()
			
			// GEX = OI * Gamma * 100 * SpotPrice
			gex := float64(oi) * gamma * 100 * spotPrice

			if side == "CALL" {
				totalCallGex += gex
				gexByStrike[strike] += gex
			} else {
				totalPutGex += gex
				gexByStrike[strike] -= gex
			}
		}
	}

	process(chain.Calls, "CALL")
	process(chain.Puts, "PUT")

	netGex := totalCallGex - totalPutGex
	totalGex := totalCallGex + totalPutGex

	fmt.Printf("\n--- GEX Report for %s ---\n", symbol)
	fmt.Printf("Spot Price: $%.2f\n", spotPrice)
	fmt.Printf("Net GEX:    $%.2fM\n", netGex/1000000.0)
	fmt.Printf("Total GEX:  $%.2fM\n", totalGex/1000000.0)
	fmt.Printf("Gamma Regime: %s\n", func() string { if netGex > 0 { return "Positive" } else { return "Negative" }}())

	// Distribution
	var strikes []float64
	for s := range gexByStrike {
		strikes = append(strikes, s)
	}
	sort.Float64s(strikes)

	fmt.Printf("\nTop 5 GEX Levels:\n")
	type gexLevel struct {
		strike float64
		gex    float64
	}
	var levels []gexLevel
	for s, g := range gexByStrike {
		levels = append(levels, gexLevel{s, g})
	}
	sort.Slice(levels, func(i, j int) bool {
		return abs(levels[i].gex) > abs(levels[j].gex)
	})

	for i := 0; i < 5 && i < len(levels); i++ {
		fmt.Printf("Strike $%.2f: $%.2fM\n", levels[i].strike, levels[i].gex/1000000.0)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
