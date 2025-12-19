package fred

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	baseURL = "https://api.stlouisfed.org/fred"
)

// Client for FRED API
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new FRED API client
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ReleaseDatesResponse represents the FRED API response for release dates
type ReleaseDatesResponse struct {
	RealtimeStart string        `json:"realtime_start"`
	RealtimeEnd   string        `json:"realtime_end"`
	OrderBy       string        `json:"order_by"`
	SortOrder     string        `json:"sort_order"`
	Count         int           `json:"count"`
	Offset        int           `json:"offset"`
	Limit         int           `json:"limit"`
	ReleaseDates  []ReleaseDate `json:"release_dates"`
}

// ReleaseDate represents a single economic release date
type ReleaseDate struct {
	ReleaseID   int    `json:"release_id"`
	ReleaseName string `json:"release_name"`
	Date        string `json:"date"`
}

// GetUpcomingReleases fetches releases from past 7 days to next N days
func (c *Client) GetUpcomingReleases(days int) (*ReleaseDatesResponse, error) {
	startDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02") // Look back 7 days
	endDate := time.Now().AddDate(0, 0, days).Format("2006-01-02")

	url := fmt.Sprintf("%s/releases/dates?realtime_start=%s&realtime_end=%s&api_key=%s&file_type=json&limit=1000",
		baseURL, startDate, endDate, c.apiKey)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result ReleaseDatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// FilteredRelease represents a release we care about with impact level
type FilteredRelease struct {
	ReleaseID   int
	ReleaseName string
	Date        time.Time
	Impact      string // High, Medium, Low
}

// GetFilteredReleases returns only the releases we care about with impact levels
func (c *Client) GetFilteredReleases(days int) ([]FilteredRelease, error) {
	response, err := c.GetUpcomingReleases(days)
	if err != nil {
		return nil, err
	}

	filtered := make([]FilteredRelease, 0)

	// Map of release IDs we care about with their impact levels
	importantReleases := map[int]string{
		// High Impact - Market Moving
		10:  "High",   // Consumer Price Index (CPI)
		50:  "High",   // Employment Situation (NFP + Unemployment)
		9:   "High",   // Advance Monthly Sales for Retail and Food Services
		192: "High",   // Job Openings and Labor Turnover Survey (JOLTS)
		436: "High",   // Monthly Retail Trade and Food Services
		
		// Medium Impact - Important Economic Indicators
		46:  "Medium", // Producer Price Index (PPI)
		479: "Medium", // Consumer Expenditure Surveys
		11:  "Medium", // Employment Cost Index
		386: "Medium", // GDPNow (Atlanta Fed GDP forecast)
		296: "Medium", // Housing Vacancies and Homeownership
		92:  "Medium", // Selected Real Retail Sales Series
		
		// Low Impact - Regional/Supplementary Data
		112: "Low",    // State Employment and Unemployment
		113: "Low",    // Metropolitan Area Employment and Unemployment
		308: "Low",    // State and Metro Area Employment, Hours, and Earnings
		477: "Low",    // Monthly State Retail Sales
		
		// Note: FOMC Press Release (ID 101) excluded - it's daily data, not the actual meeting
		// Actual FOMC meetings should be added manually or from Fed calendar
	}

	for _, release := range response.ReleaseDates {
		if impact, ok := importantReleases[release.ReleaseID]; ok {
			releaseDate, err := time.Parse("2006-01-02", release.Date)
			if err != nil {
				continue
			}

			filtered = append(filtered, FilteredRelease{
				ReleaseID:   release.ReleaseID,
				ReleaseName: release.ReleaseName,
				Date:        releaseDate,
				Impact:      impact,
			})
		}
	}

	return filtered, nil
}
