package gex

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

// Option represents an individual option in the chain
type Option struct {
	Strike       float64 `json:"strike"`
	OptionType   string  `json:"option_type"`
	OpenInterest int     `json:"open_interest"`
	Greeks       struct {
		Gamma float64 `json:"gamma"`
	} `json:"greeks"`
}

// Response represents the JSON structure of the option chain
type Response struct {
	Options struct {
		Option []Option `json:"option"`
	} `json:"options"`
}

// Response structure for the Tradier API
type QuoteResponse struct {
	Quotes struct {
		Quote struct {
			Symbol string  `json:"symbol"`
			Last   float64 `json:"last"` // Spot price
		} `json:"quote"`
	} `json:"quotes"`
}

func GetSpotPrice(apiToken, symbol string) (float64, error) {
	url := fmt.Sprintf("https://api.tradier.com/v1/markets/quotes?symbols=%s", symbol)

	// Create a new request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Accept", "application/json")

	// Make the HTTP request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Parse the response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var quoteResp QuoteResponse
	err = json.Unmarshal(body, &quoteResp)
	if err != nil {
		return 0, err
	}

	// Return the spot price
	return quoteResp.Quotes.Quote.Last, nil
}

// FetchOptionsChain fetches the options chain for the given symbol and expiration date
func FetchOptionsChain(symbol, expiration string, apiKey string) ([]Option, error) {
	url := fmt.Sprintf("https://api.tradier.com/v1/markets/options/chains?symbol=%s&expiration=%s&greeks=true", symbol, expiration)

	// Create HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	// Make HTTP request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check HTTP response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch data: %s", resp.Status)
	}

	// Parse response body
	var response Response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	return response.Options.Option, nil
}

func CalculateGEXPerStrike(options []Option, spotPrice float64) map[float64]float64 {
	gexByStrike := make(map[float64]float64)

	for _, option := range options {
		if option.OpenInterest > 0 && option.Greeks.Gamma != 0 {
			// Calculate GEX using the correct formula
			gex := float64(option.OpenInterest) * option.Greeks.Gamma * (spotPrice * spotPrice)

			// Add or subtract based on option type
			if option.OptionType == "call" {
				gexByStrike[option.Strike] += gex
			} else if option.OptionType == "put" {
				gexByStrike[option.Strike] -= gex
			}
		}
	}

	return gexByStrike
}

func main() {

	// API Key from environment variable
	apiKey := os.Getenv("TRADIER_API_KEY")
	if apiKey == "" {
		log.Fatal("TRADIER_API_KEY environment variable is not set")
	}

	// Parameters for API call
	symbol := "TSLA"           // Replace with your desired symbol
	expiration := "2024-12-27" // Replace with the desired expiration date

	// Fetch options chain
	price, err := GetSpotPrice(apiKey, symbol)
	if err != nil {
		log.Fatalf("Error fetching price: %v", err)
	}
	options, err := FetchOptionsChain(symbol, expiration, apiKey)

	if err != nil {
		log.Fatalf("Error fetching options chain: %v", err)
	}

	gexByStrike := CalculateGEXPerStrike(options, price)

	// Calculate GEX per strike
	// Sort the strike prices
	strikePrices := make([]float64, 0, len(gexByStrike))
	for strike := range gexByStrike {
		strikePrices = append(strikePrices, strike)
	}
	sort.Float64s(strikePrices)
	// Print GEX per strike in sorted order
	fmt.Println("Gamma Exposure (GEX) per Strike Price (Sorted):")
	for _, strike := range strikePrices {
		fmt.Printf("Strike: %.2f, GEX: %.2f\n", strike, gexByStrike[strike])
	}

	// Create a plot
	p := plot.New()
	p.Title.Text = "Gamma Exposure (GEX) per Strike Price"
	p.X.Label.Text = "Strike Price"
	p.Y.Label.Text = "GEX"

	// Plot GEX as a bar chart
	outputPath := "gex_chart_" + symbol + time.Now().String() + ".png"

	// Create the GEX plot
	err = CreateGEXPlot(gexByStrike, symbol, outputPath)
	if err != nil {
		log.Fatal("Error creating GEX plot:", err)
	}

	fmt.Println("GEX plot has been saved as 'gex_plot.png'")

}

func CreateGEXPlot(gexByStrike map[float64]float64, symbol string, path string) error {
	p := plot.New()
	p.Title.Text = fmt.Sprintf("GEX Distribution for %s", symbol)
	p.X.Label.Text = "Strike Price"
	p.Y.Label.Text = "GEX"

	// Convert map to sorted slices
	var strikes []float64
	for strike := range gexByStrike {
		if strike < -100 || strike > 100 {
			strikes = append(strikes, strike)
		}
	}
	sort.Float64s(strikes)

	// Create separate bars for positive and negative values
	var posPoints plotter.Values
	var negPoints plotter.Values
	var labels []string

	// Split data into positive and negative values
	for _, strike := range strikes {
		gex := gexByStrike[strike]
		labels = append(labels, fmt.Sprintf("%.2f", strike))
		if gex >= 0 && gex > 100 {
			posPoints = append(posPoints, gex)
			negPoints = append(negPoints, 0)
		} else if gex < -100 {
			posPoints = append(posPoints, 0)
			negPoints = append(negPoints, gex)
		}
	}

	// Create positive bars (green)
	posBar, err := plotter.NewBarChart(posPoints, vg.Points(20))
	if err != nil {
		return err
	}
	posBar.Color = color.RGBA{G: 255, A: 255}
	posBar.Offset = 0

	// Create negative bars (red)
	negBar, err := plotter.NewBarChart(negPoints, vg.Points(20))
	if err != nil {
		return err
	}
	negBar.Color = color.RGBA{R: 255, A: 255}
	negBar.Offset = 0

	p.Add(posBar, negBar)

	// Set X axis labels
	p.NominalX(labels...)
	p.X.Tick.Label.Rotation = 1.0 // Rotate labels by 1 radian (~57 degrees)
	p.X.Tick.Label.XAlign = 1.0   // Align labels to the right

	return p.Save(16*vg.Inch, 8*vg.Inch, path)
}

// makeXAxisLabels converts strike prices to formatted labels
func makeXAxisLabels(strikePrices []float64) []string {
	step := 5 // Show every 5th strike price (adjust as needed)
	labels := make([]string, len(strikePrices))
	for i, strike := range strikePrices {
		if i%step == 0 {
			labels[i] = fmt.Sprintf("%.2f", strike) // Add label
		} else {
			labels[i] = "" // Leave blank for others
		}
	}
	return labels
}
