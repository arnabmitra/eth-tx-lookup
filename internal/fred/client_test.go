package fred

import (
	"fmt"
	"testing"
)

func TestFREDClient(t *testing.T) {
	// Get API key from environment
	apiKey := "f5991f935f3de996990f99823bdd172b"
	
	client := NewClient(apiKey)
	
	// Test fetching upcoming releases
	response, err := client.GetUpcomingReleases(30)
	if err != nil {
		t.Fatalf("Failed to fetch releases: %v", err)
	}
	
	fmt.Printf("Found %d total releases in next 30 days\n", response.Count)
	fmt.Printf("Sample releases:\n")
	for i, release := range response.ReleaseDates {
		if i >= 5 {
			break
		}
		fmt.Printf("  - [%d] %s on %s\n", release.ReleaseID, release.ReleaseName, release.Date)
	}
}

func TestFilteredReleases(t *testing.T) {
	apiKey := "f5991f935f3de996990f99823bdd172b"
	
	client := NewClient(apiKey)
	
	// Test filtered releases for next 60 days to get more results
	filtered, err := client.GetFilteredReleases(60)
	if err != nil {
		t.Fatalf("Failed to fetch filtered releases: %v", err)
	}
	
	fmt.Printf("\nFiltered to %d important releases in next 60 days:\n", len(filtered))
	for _, release := range filtered {
		fmt.Printf("  - [%s] %s on %s (ID: %d)\n", 
			release.Impact, 
			release.ReleaseName, 
			release.Date.Format("Jan 02, 2006"),
			release.ReleaseID)
	}
	
	// Verify we got some high-impact releases
	if len(filtered) == 0 {
		t.Error("Expected at least some filtered releases")
	}
}
