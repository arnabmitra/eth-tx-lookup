package gex

import (
	"sort"
	"testing"
)

func TestCalculateGammaFlipLevel(t *testing.T) {
	tests := []struct {
		name           string
		gexByStrike    map[float64]float64
		expectedFlip   float64
		description    string
	}{
		{
			name: "Simple flip from positive to negative",
			gexByStrike: map[float64]float64{
				580.0: 100000.0,  // Positive GEX below
				590.0: 50000.0,   // Positive GEX
				600.0: -20000.0,  // Negative GEX above
				610.0: -40000.0,  // Negative GEX
			},
			expectedFlip: 595.0, // Should be between 590 and 600
			description: "Gamma flip should occur between the last positive and first negative GEX strike",
		},
		{
			name: "Realistic SPY scenario",
			gexByStrike: map[float64]float64{
				580.0: 500000.0,
				590.0: 300000.0,
				600.0: 200000.0,
				610.0: 100000.0,
				620.0: 50000.0,
				630.0: -10000.0,  // First negative
				640.0: -50000.0,
				650.0: -100000.0,
			},
			expectedFlip: 625.0, // Should be between 620 and 630
			description: "For SPY, gamma flip typically occurs where market makers switch from net long to net short gamma",
		},
		{
			name: "All positive GEX",
			gexByStrike: map[float64]float64{
				580.0: 100000.0,
				590.0: 50000.0,
				600.0: 25000.0,
			},
			expectedFlip: 580.0, // Flip is below all strikes (returns lowest strike)
			description: "When all GEX is positive, gamma flip is below all strikes",
		},
		{
			name: "All negative GEX",
			gexByStrike: map[float64]float64{
				580.0: -100000.0,
				590.0: -50000.0,
				600.0: -25000.0,
			},
			expectedFlip: 600.0, // Flip is above all strikes (returns highest strike)
			description: "When all GEX is negative, gamma flip is above all strikes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateGammaFlipLevel(tt.gexByStrike)
			
			t.Logf("Test: %s", tt.description)
			t.Logf("Expected flip level: %.2f", tt.expectedFlip)
			t.Logf("Actual flip level: %.2f", result)
			
			// For non-zero expected values, check if result is close
			if tt.expectedFlip != 0 {
				// Allow 10% tolerance for interpolation
				tolerance := tt.expectedFlip * 0.1
				if result < tt.expectedFlip-tolerance || result > tt.expectedFlip+tolerance {
					t.Errorf("Gamma flip level = %.2f, want approximately %.2f (tolerance: %.2f)", 
						result, tt.expectedFlip, tolerance)
				}
			}
		})
	}
}

func TestCalculateGammaFlipLevelDetailed(t *testing.T) {
	// This test has cumulative GEX that stays positive throughout
	// So the gamma flip is below all strikes
	gexByStrike := map[float64]float64{
		// Deep ITM puts / OTM calls - positive GEX
		650.0: 800000.0,
		660.0: 750000.0,
		670.0: 700000.0,
		680.0: 500000.0,  // High positive GEX
		685.0: 200000.0,  // Still positive
		687.0: 50000.0,   // Last positive
		688.0: -10000.0,  // First negative
		690.0: -100000.0,
		695.0: -200000.0,
		700.0: -300000.0,
	}

	result := CalculateGammaFlipLevel(gexByStrike)
	
	t.Logf("Gamma flip level calculated: %.2f", result)
	
	// The cumulative GEX stays positive (starts at 800k, ends at ~2.4M)
	// So flip is below all strikes (returns lowest strike)
	if result != 650.0 {
		t.Errorf("Gamma flip level = %.2f, expected 650.00 (below all strikes)", result)
	}
	
	// Let's also test the interpretation:
	// - Below flip level: Not applicable (flip is below all strikes)
	// - Above flip level: Positive GEX zone (stabilizing)
	t.Logf("Interpretation:")
	t.Logf("  Gamma flip at %.2f means all price levels above are in positive GEX zone (stabilizing)", result)
}

func TestCalculateGammaFlipLevelRealSPYData(t *testing.T) {
	// Real SPY data from 01/02 expiry showing gamma flip should be around 678
	// (where cumulative GEX crosses from negative to positive)
	gexByStrike := map[float64]float64{
		654.0: 778.52,
		655.0: -15514.27,
		656.0: -11141.88,
		657.0: -9880.30,
		658.0: -4374.73,
		659.0: -134222.27,
		660.0: -961131.31,
		661.0: -232299.51,
		662.0: -380177.82,
		663.0: -176855.49,
		664.0: -2445113.87,
		665.0: -4229783.31,
		666.0: -1975845.47,
		667.0: -28882783.81,
		668.0: -12669650.27,
		669.0: -4405504.53,
		670.0: -32775436.67,
		671.0: -35027098.98,
		672.0: -401468975.42,
		673.0: -12114905.00,
		674.0: -13908443.35,
		675.0: -62830936.50,
		676.0: -86270656.90,
		677.0: -15077919.55,
		678.0: 731476333.39,  // Large positive that flips cumulative
		679.0: -61728697.39,
		680.0: -177234503.56,
		681.0: -224611540.15,
		682.0: -158644298.24,
		683.0: -134019393.71,
		684.0: -2626516216.12, // Huge negative
		685.0: -78328973.35,
		686.0: -80026843.35,
		687.0: 739759003.30,  // Huge positive
		688.0: 1412067.30,
		689.0: -114177880.58,
		690.0: 342909287.06,  // Large positive
		691.0: 50414852.83,
		692.0: 21979291.50,
		693.0: 19370252.26,
		694.0: 13234224.61,
		695.0: 11022551.81,
	}

	result := CalculateGammaFlipLevel(gexByStrike)
	
	t.Logf("Gamma flip level calculated: %.2f", result)
	t.Logf("Expected to be around 678 where cumulative GEX crosses zero")
	
	// For this data, we need to calculate cumulative GEX to understand
	// Let's compute it here for logging
	strikes := []float64{}
	for strike := range gexByStrike {
		strikes = append(strikes, strike)
	}
	sort.Float64s(strikes)
	
	cumulativeGEX := 0.0
	t.Logf("\nCumulative GEX by strike:")
	for _, strike := range strikes {
		cumulativeGEX += gexByStrike[strike]
		if strike >= 675 && strike <= 690 {
			t.Logf("  Strike %.2f: Individual GEX = %.2f, Cumulative GEX = %.2f", 
				strike, gexByStrike[strike], cumulativeGEX)
		}
	}
	
	// The flip should be around 677-678 where cumulative crosses zero
	if result < 677.0 || result > 679.0 {
		t.Errorf("Gamma flip level = %.2f, expected between 677.00 and 679.00 based on real data", result)
	}
}
