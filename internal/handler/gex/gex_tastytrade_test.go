package gex

import (
	"testing"

	"github.com/joho/godotenv"
)

func TestTastytradeIntegration(t *testing.T) {
	_ = godotenv.Load("../../../.env")
	cID, cSec, cRef := GetTastytradeConfig()
	if cID == "" || cSec == "" || cRef == "" {
		t.Skip("Tastytrade credentials not configured; skipping integration test")
	}

	symbol := "SPY"

	// 1. Spot Price
	spotPrice, err := GetSpotPrice("", "", symbol)
	if err != nil {
		t.Fatalf("GetSpotPrice failed: %v", err)
	}
	if spotPrice <= 0 {
		t.Fatalf("Expected spot price > 0, got %f", spotPrice)
	}
	t.Logf("Tastytrade spot price for %s: $%.2f", symbol, spotPrice)

	// 2. Expirations
	expiries, err := GetExpirationDates("", "", symbol)
	if err != nil {
		t.Fatalf("GetExpirationDates failed: %v", err)
	}
	if len(expiries) == 0 {
		t.Fatalf("Expected at least one expiration date, got 0")
	}
	t.Logf("Tastytrade found %d expirations for %s (First: %s)", len(expiries), symbol, expiries[0])

	// 3. Option Chain & GEX calculation
	options, jsonStr, warning, err := FetchOptionsChain(symbol, expiries[0], "", "")
	if err != nil {
		t.Fatalf("FetchOptionsChain failed: %v", err)
	}
	if len(options) == 0 {
		t.Fatalf("Expected option contracts, got 0")
	}
	if jsonStr == nil || *jsonStr == "" {
		t.Fatalf("Expected valid JSON response string")
	}
	t.Logf("Fetched %d option contracts for %s on %s (Warning: %s)", len(options), symbol, expiries[0], warning)

	// 4. Calculate GEX
	gexByStrike := CalculateGEXPerStrike(options, spotPrice)
	if len(gexByStrike) == 0 {
		t.Fatalf("Expected GEX per strike to have entries")
	}
	totalGEX := 0.0
	for _, val := range gexByStrike {
		totalGEX += val
	}
	t.Logf("Computed GEX for %d strikes. Total Net GEX: $%.2f", len(gexByStrike), totalGEX)
}
