package tastytrade

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type OptionContract struct {
	Symbol         string
	Strike         float64
	OptionType     string // "CALL" or "PUT"
	OpenInterest   int
	Gamma          float64
	Delta          float64
	Theta          float64
	Vega           float64
	IV             float64
	ExpirationDate string
	ExpirationType string
}

type cachedChain struct {
	items     []chainItem
	fetchedAt time.Time
}

type chainItem struct {
	Symbol         string `json:"symbol"`
	StrikePrice    string `json:"strike-price"`
	OptionType     string `json:"option-type"`
	ExpirationDate string `json:"expiration-date"`
	ExpirationType string `json:"expiration-type"`
	Active         bool   `json:"active"`
}

type Client struct {
	BaseURL        string
	ClientID       string
	ClientSecret   string
	RefreshToken   string
	AccessToken    string
	TokenExpiresAt time.Time
	HTTPClient     *http.Client
	mu             sync.Mutex
	chainCache     map[string]cachedChain
	cacheMu        sync.RWMutex
}

func NewClient(clientID, clientSecret, refreshToken string) *Client {
	return &Client{
		BaseURL:      "https://api.tastyworks.com",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RefreshToken: refreshToken,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
		chainCache:   make(map[string]cachedChain),
	}
}

// Authenticate exchanges the refresh token for a 15-minute access token.
func (c *Client) Authenticate() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If already authenticated and token has at least 2 minutes left, reuse
	if c.AccessToken != "" && time.Now().Before(c.TokenExpiresAt.Add(-2*time.Minute)) {
		return nil
	}

	payload := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": c.RefreshToken,
		"client_id":     c.ClientID,
		"client_secret": c.ClientSecret,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/oauth/token", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "eth-tx-lookup/1.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("tastytrade auth request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tastytrade auth failed (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBytes, &tokenResp); err != nil {
		return fmt.Errorf("failed to parse auth response: %w", err)
	}

	c.AccessToken = tokenResp.AccessToken
	if tokenResp.ExpiresIn <= 0 {
		tokenResp.ExpiresIn = 900 // Default to 15 mins
	}
	c.TokenExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return nil
}

func (c *Client) getAccessToken() (string, error) {
	if c.AccessToken == "" || time.Now().After(c.TokenExpiresAt.Add(-2*time.Minute)) {
		if err := c.Authenticate(); err != nil {
			return "", err
		}
	}
	return c.AccessToken, nil
}

// GetSpotPrice returns the current spot price for an equity or index symbol.
func (c *Client) GetSpotPrice(symbol string) (float64, error) {
	token, err := c.getAccessToken()
	if err != nil {
		return 0, err
	}

	sym := strings.ToUpper(strings.TrimSpace(symbol))
	paramName := "equity[]"
	if sym == "SPX" || sym == "NDX" || sym == "VIX" || sym == "RUT" {
		paramName = "index[]"
	}
	urlStr := fmt.Sprintf("%s/market-data/by-type?%s=%s", c.BaseURL, paramName, url.QueryEscape(sym))

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "eth-tx-lookup/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("tastytrade market-data failed (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Data struct {
			Items []map[string]interface{} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return 0, err
	}

	if len(result.Data.Items) == 0 {
		return 0, fmt.Errorf("no market data item returned for %s", sym)
	}

	item := result.Data.Items[0]
	for _, key := range []string{"last", "mark", "mid", "close"} {
		if val, ok := item[key].(string); ok && val != "" {
			if f, err := strconv.ParseFloat(val, 64); err == nil && f > 0 {
				return f, nil
			}
		}
	}

	// Try bid/ask average
	bidStr, _ := item["bid"].(string)
	askStr, _ := item["ask"].(string)
	bid, _ := strconv.ParseFloat(bidStr, 64)
	ask, _ := strconv.ParseFloat(askStr, 64)
	if bid > 0 && ask > 0 {
		return (bid + ask) / 2.0, nil
	}

	return 0, fmt.Errorf("unable to determine spot price for %s from market data", sym)
}

// fetchRawOptionChain retrieves and caches the list of option contracts for a symbol.
func (c *Client) fetchRawOptionChain(symbol string) ([]chainItem, error) {
	sym := strings.ToUpper(strings.TrimSpace(symbol))

	c.cacheMu.RLock()
	cached, found := c.chainCache[sym]
	c.cacheMu.RUnlock()

	if found && time.Since(cached.fetchedAt) < 5*time.Minute {
		return cached.items, nil
	}

	token, err := c.getAccessToken()
	if err != nil {
		return nil, err
	}

	urlStr := fmt.Sprintf("%s/option-chains/%s", c.BaseURL, url.PathEscape(sym))
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "eth-tx-lookup/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tastytrade get option-chains failed (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Data struct {
			Items []chainItem `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, err
	}

	c.cacheMu.Lock()
	c.chainCache[sym] = cachedChain{
		items:     result.Data.Items,
		fetchedAt: time.Now(),
	}
	c.cacheMu.Unlock()

	return result.Data.Items, nil
}

// GetExpirations returns all future active expiration dates for the given symbol, sorted ascending.
func (c *Client) GetExpirations(symbol string) ([]string, error) {
	items, err := c.fetchRawOptionChain(symbol)
	if err != nil {
		return nil, err
	}

	today := time.Now().Format("2006-01-02")
	seen := make(map[string]bool)
	var expiries []string

	for _, itm := range items {
		if itm.Active && itm.ExpirationDate >= today {
			if !seen[itm.ExpirationDate] {
				seen[itm.ExpirationDate] = true
				expiries = append(expiries, itm.ExpirationDate)
			}
		}
	}

	sort.Strings(expiries)
	return expiries, nil
}

// FetchOptionChain returns all option contracts for a specific expiration, complete with Greeks and Open Interest.
func (c *Client) FetchOptionChain(symbol, expiration string) ([]OptionContract, error) {
	items, err := c.fetchRawOptionChain(symbol)
	if err != nil {
		return nil, err
	}

	var targetItems []chainItem
	for _, itm := range items {
		if itm.Active && itm.ExpirationDate == expiration {
			targetItems = append(targetItems, itm)
		}
	}

	if len(targetItems) == 0 {
		return nil, fmt.Errorf("no active option contracts found for %s on %s", symbol, expiration)
	}

	token, err := c.getAccessToken()
	if err != nil {
		return nil, err
	}

	// Fetch market data in concurrent batches of 50 contracts
	batchSize := 50
	numBatches := (len(targetItems) + batchSize - 1) / batchSize

	type batchResult struct {
		data map[string]map[string]interface{}
		err  error
	}

	resChan := make(chan batchResult, numBatches)
	sem := make(chan struct{}, 5) // max 5 concurrent requests

	for i := 0; i < len(targetItems); i += batchSize {
		end := i + batchSize
		if end > len(targetItems) {
			end = len(targetItems)
		}
		chunk := targetItems[i:end]

		go func(contracts []chainItem) {
			sem <- struct{}{}
			defer func() { <-sem }()

			var queryParts []string
			for _, item := range contracts {
				queryParts = append(queryParts, "equity-option[]="+url.QueryEscape(item.Symbol))
			}
			mdURL := fmt.Sprintf("%s/market-data/by-type?%s", c.BaseURL, strings.Join(queryParts, "&"))

			mdReq, err := http.NewRequest("GET", mdURL, nil)
			if err != nil {
				resChan <- batchResult{err: err}
				return
			}
			mdReq.Header.Set("Authorization", "Bearer "+token)
			mdReq.Header.Set("User-Agent", "eth-tx-lookup/1.0")
			mdReq.Header.Set("Accept", "application/json")

			mdResp, err := c.HTTPClient.Do(mdReq)
			if err != nil {
				resChan <- batchResult{err: err}
				return
			}
			defer mdResp.Body.Close()

			mdBytes, _ := io.ReadAll(mdResp.Body)
			if mdResp.StatusCode != http.StatusOK {
				resChan <- batchResult{err: fmt.Errorf("market data batch failed (%d): %s", mdResp.StatusCode, string(mdBytes))}
				return
			}

			var mdResult struct {
				Data struct {
					Items []map[string]interface{} `json:"items"`
				} `json:"data"`
			}
			if err := json.Unmarshal(mdBytes, &mdResult); err != nil {
				resChan <- batchResult{err: err}
				return
			}

			mMap := make(map[string]map[string]interface{})
			for _, itm := range mdResult.Data.Items {
				if sym, ok := itm["symbol"].(string); ok {
					mMap[sym] = itm
				}
			}
			resChan <- batchResult{data: mMap}
		}(chunk)
	}

	marketDataMap := make(map[string]map[string]interface{})
	for i := 0; i < numBatches; i++ {
		res := <-resChan
		if res.data != nil {
			for k, v := range res.data {
				marketDataMap[k] = v
			}
		}
	}

	var contracts []OptionContract
	for _, itm := range targetItems {
		strike, _ := strconv.ParseFloat(itm.StrikePrice, 64)
		optType := "CALL"
		if strings.ToUpper(itm.OptionType) == "P" {
			optType = "PUT"
		}

		contract := OptionContract{
			Symbol:         itm.Symbol,
			Strike:         strike,
			OptionType:     optType,
			ExpirationDate: expiration,
			ExpirationType: itm.ExpirationType,
		}

		if md, ok := marketDataMap[itm.Symbol]; ok {
			if oiVal, ok := md["open-interest"].(float64); ok {
				contract.OpenInterest = int(oiVal)
			}
			if gStr, ok := md["gamma"].(string); ok {
				if gFloat, err := strconv.ParseFloat(gStr, 64); err == nil {
					contract.Gamma = gFloat
				}
			}
			if dStr, ok := md["delta"].(string); ok {
				if dFloat, err := strconv.ParseFloat(dStr, 64); err == nil {
					contract.Delta = dFloat
				}
			}
			if tStr, ok := md["theta"].(string); ok {
				if tFloat, err := strconv.ParseFloat(tStr, 64); err == nil {
					contract.Theta = tFloat
				}
			}
			if vStr, ok := md["vega"].(string); ok {
				if vFloat, err := strconv.ParseFloat(vStr, 64); err == nil {
					contract.Vega = vFloat
				}
			}
			if ivStr, ok := md["volatility"].(string); ok {
				if ivFloat, err := strconv.ParseFloat(ivStr, 64); err == nil {
					contract.IV = ivFloat
				}
			}
		}

		contracts = append(contracts, contract)
	}

	return contracts, nil
}
