package handler

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arnabmitra/eth-proxy/internal/handler/gex"
)

func TestHeatmapCalculationsAndRiskTiers(t *testing.T) {
	// Test risk tier classification logic
	tests := []struct {
		delta    float64
		expected string
	}{
		{0.10, "safe"},
		{-0.14, "safe"},
		{0.15, "low"},
		{-0.28, "low"},
		{0.30, "moderate"},
		{-0.42, "moderate"},
		{0.45, "risky"},
		{-0.58, "risky"},
		{0.65, "aggressive"},
		{-0.85, "aggressive"},
	}

	for _, tt := range tests {
		absDelta := math.Abs(tt.delta)
		var tier string
		switch {
		case absDelta < 0.15:
			tier = "safe"
		case absDelta < 0.30:
			tier = "low"
		case absDelta < 0.45:
			tier = "moderate"
		case absDelta < 0.60:
			tier = "risky"
		default:
			tier = "aggressive"
		}

		if tier != tt.expected {
			t.Errorf("delta %f: expected %s, got %s", tt.delta, tt.expected, tier)
		}
	}

	// Test per_day calculation
	mark := 29.42
	premium := mark * 100.0 // $2,942.0
	dte := 6
	perDay := premium / float64(dte)
	expectedPerDay := 490.3333333333333
	if math.Abs(perDay-expectedPerDay) > 0.001 {
		t.Errorf("expected perDay ~490.33, got %f", perDay)
	}

	// Test theta_day calculation
	theta := -0.7095
	thetaDay := math.Abs(theta) * 100.0
	expectedThetaDay := 70.95
	if math.Abs(thetaDay-expectedThetaDay) > 0.001 {
		t.Errorf("expected thetaDay 70.95, got %f", thetaDay)
	}
}

func TestHeatmapDataWithMockCache(t *testing.T) {
	h := NewHeatmapHandler(nil, nil)

	// Pre-populate cache with mock data
	exp1 := time.Now().Add(6 * 24 * time.Hour).Format("2006-01-02")
	exp2 := time.Now().Add(11 * 24 * time.Hour).Format("2006-01-02")

	opt1 := gex.Option{
		Strike:         880.0,
		OptionType:     "PUT",
		Bid:            29.00,
		Ask:            29.84,
		Mark:           29.42,
		ExpirationDate: exp1,
	}
	opt1.Greeks.Delta = -0.48
	opt1.Greeks.Theta = -0.7095
	opt1.Greeks.Gamma = 0.0025
	opt1.Greeks.Vega = 1.1770
	opt1.Greeks.IV = 0.535

	entry := &heatmapCacheEntry{
		symbol:      "MU",
		spotPrice:   879.80,
		expirations: []string{exp1, exp2},
		chains: map[string][]gex.Option{
			exp1: {opt1},
		},
		fetchedAt: time.Now(),
	}
	h.cache.Store("MU", entry)

	req := httptest.NewRequest(http.MethodGet, "/api/option-heatmap?symbol=MU&type=puts&expirations=2", nil)
	w := httptest.NewRecorder()

	h.GetHeatmapData(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var data HeatmapResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if data.Symbol != "MU" {
		t.Errorf("expected symbol MU, got %s", data.Symbol)
	}
	if data.SpotPrice != 879.80 {
		t.Errorf("expected spot price 879.80, got %f", data.SpotPrice)
	}
	if len(data.Expirations) == 0 {
		t.Fatalf("expected expirations, got 0")
	}

	cellKey := "880.00_" + exp1
	cell, ok := data.Matrix[cellKey]
	if !ok {
		t.Fatalf("expected cell for key %s", cellKey)
	}

	if cell.RiskTier != "risky" {
		t.Errorf("expected risk tier 'risky' for delta 0.48, got %s", cell.RiskTier)
	}
	if cell.Premium != 2942.0 {
		t.Errorf("expected premium 2942, got %f", cell.Premium)
	}
	if cell.PerDay <= 0 {
		t.Errorf("expected positive per_day, got %f", cell.PerDay)
	}
	if cell.ThetaDay != 70.95 {
		t.Errorf("expected theta_day 70.95, got %f", cell.ThetaDay)
	}
}
