package gex

import (
	"encoding/json"
	"fmt"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"image/color"
	"io/ioutil"
	"net/http"
	"sort"
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

func CreateGEXPlot(gexByStrike map[float64]float64, symbol string, path string, spotPrice float64) error {
	p := plot.New()
	p.Title.Text = fmt.Sprintf("GEX Distribution for %s, Spot Price %f", symbol, spotPrice)
	p.X.Label.Text = "Strike Price"
	p.Y.Label.Text = "GEX"

	// Convert map to sorted slices
	var strikes []float64
	for strike, gex := range gexByStrike {
		if gex < -100 || gex > 100 {
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
		if gex >= 0 && gex > 5000 {
			//print labels and values
			fmt.Printf(" +ve Strike: %.2f, GEX: %.2f\n", strike, gex)
			labels = append(labels, fmt.Sprintf("%.2f", strike))
			posPoints = append(posPoints, gex)
			negPoints = append(negPoints, 0)
		} else if gex < -5000 {
			fmt.Printf(" -ve Strike: %.2f, GEX: %.2f\n", strike, gex)
			labels = append(labels, fmt.Sprintf("%.2f", strike))
			posPoints = append(posPoints, 0)
			negPoints = append(negPoints, gex)
		}
	}

	// Create positive bars (green)
	posBar, err := plotter.NewBarChart(posPoints, vg.Points(10))
	if err != nil {
		return err
	}
	posBar.Color = color.RGBA{G: 255, A: 255}
	posBar.Offset = vg.Points(5)

	// Create negative bars (red)
	negBar, err := plotter.NewBarChart(negPoints, vg.Points(10))
	if err != nil {
		return err
	}
	negBar.Color = color.RGBA{R: 255, A: 255}
	negBar.Offset = vg.Points(5)

	// Create labels for the bars
	labelPoints := make(plotter.XYs, len(labels))
	for i, _ := range labels {
		labelPoints[i].X = float64(i)
		labelPoints[i].Y = posPoints[i] + negPoints[i]
	}
	barLabels, err := plotter.NewLabels(plotter.XYLabels{
		XYs:    labelPoints,
		Labels: labels,
	})
	if err != nil {
		return err
	}

	p.Add(posBar, negBar, barLabels)

	// Custom y-axis ticker
	p.Y.Tick.Marker = plot.TickerFunc(func(min, max float64) []plot.Tick {
		ticks := plot.DefaultTicks{}.Ticks(min, max)
		for i := range ticks {
			ticks[i].Label = fmt.Sprintf("$%.9f", ticks[i].Value)
		}
		return ticks
	})
	// Set X axis labels
	p.NominalX(makeXAxisLabels(strikes)...)

	return p.Save(16*vg.Inch, 8*vg.Inch, path)
}

// makeXAxisLabels converts strike prices to formatted labels
func makeXAxisLabels(strikePrices []float64) []string {
	step := 5 // Show every 5th strike price (adjust as needed)
	labels := make([]string, len(strikePrices))
	for i, _ := range strikePrices {
		if i%step == 0 {
			// labels[i] = fmt.Sprintf("%.2f", strike) // Add label
			labels[i] = ""
		} else {
			labels[i] = "" // Leave blank for others
		}
	}
	return labels
}

// Expiration date for the options chain
type ExpirationResponse struct {
	Expirations struct {
		Expiration []struct {
			Date           string `json:"date"`
			ExpirationType string `json:"expiration_type"`
		} `json:"expiration"`
	} `json:"expirations"`
}

func GetExpirationDates(apiToken, symbol string) ([]string, error) {
	url := fmt.Sprintf("https://api.tradier.com/v1/markets/options/expirations?symbol=%s&expirationType=true", symbol)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error executing request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}

	var data ExpirationResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}

	dates := make([]string, 0, len(data.Expirations.Expiration))
	for _, exp := range data.Expirations.Expiration {
		dates = append(dates, exp.Date)
	}

	// print expiration dates
	fmt.Println("Expiration Dates:")
	for _, date := range dates {
		fmt.Println(date)
	}

	return dates, nil
}
