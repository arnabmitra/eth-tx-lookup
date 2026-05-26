package public

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	Secret     string
	Token      string
	AccountID  string
	HTTPClient *http.Client
}

func NewClient(secret, accountID string) *Client {
	return &Client{
		BaseURL:    "https://api.public.com",
		Secret:     secret,
		AccountID:  accountID,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type AccessTokenRequest struct {
	Secret            string `json:"secret"`
	ValidityInMinutes int    `json:"validityInMinutes"`
}

type AccessTokenResponse struct {
	AccessToken string `json:"accessToken"`
}

func (c *Client) Authenticate() error {
	reqBody := AccessTokenRequest{
		Secret:            c.Secret,
		ValidityInMinutes: 60,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	url := c.BaseURL + "/userapiauthservice/personal/access-tokens"
	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth failed: %s %s", resp.Status, string(body))
	}

	var res AccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}

	c.Token = res.AccessToken
	return nil
}

type AccountResponse struct {
	Account struct {
		AccountID string `json:"accountId"`
	} `json:"account"`
}

func (c *Client) FetchAccountID() error {
	if c.AccountID != "" {
		return nil
	}

	req, err := http.NewRequest("GET", c.BaseURL+"/userapigateway/trading/account", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fetch account failed: %s %s", resp.Status, string(body))
	}

	var res AccountResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}

	c.AccountID = res.Account.AccountID
	return nil
}

type QuoteRequest struct {
	Instruments []Instrument `json:"instruments"`
}

type Instrument struct {
	Symbol string `json:"symbol"`
	Type   string `json:"type"`
}

type QuoteResponse struct {
	Quotes []struct {
		Last string `json:"last"`
	} `json:"quotes"`
}

func (c *Client) GetSpotPrice(symbol string) (float64, error) {
	reqBody := QuoteRequest{
		Instruments: []Instrument{{Symbol: symbol, Type: "EQUITY"}},
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return 0, err
	}

	url := fmt.Sprintf("%s/userapigateway/marketdata/%s/quotes", c.BaseURL, c.AccountID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("get quotes failed: %s %s", resp.Status, string(body))
	}

	var res QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, err
	}

	if len(res.Quotes) == 0 {
		return 0, fmt.Errorf("no quotes returned")
	}

	var price float64
	fmt.Sscanf(res.Quotes[0].Last, "%f", &price)
	return price, nil
}

type OptionChainRequest struct {
	Instrument     string `json:"instrument"`
	ExpirationDate string `json:"expirationDate,omitempty"`
}

type OptionContract struct {
	OptionSymbol string `json:"optionSymbol"`
	StrikePrice  string `json:"strikePrice"`
	OptionType   string `json:"optionType"`
	OpenInterest int    `json:"openInterest"`
}

type OptionChainResponse struct {
	Calls []OptionContract `json:"calls"`
	Puts  []OptionContract `json:"puts"`
}

func (c *Client) GetOptionChain(symbol string, expiration string) (*OptionChainResponse, error) {
	reqBody := OptionChainRequest{
		Instrument:     symbol,
		ExpirationDate: expiration,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/userapigateway/marketdata/%s/option-chain", c.BaseURL, c.AccountID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get option chain failed: %s %s", resp.Status, string(body))
	}

	var res OptionChainResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return &res, nil
}

type GreeksResponse struct {
	Greeks []struct {
		Symbol string `json:"symbol"`
		Greeks struct {
			Gamma string `json:"gamma"`
			Delta string `json:"delta"`
		} `json:"greeks"`
	} `json:"greeks"`
}

func (c *Client) GetGreeks(osiSymbols []string) (map[string]float64, error) {
	// Max 250 symbols per request
	gammaMap := make(map[string]float64)

	for i := 0; i < len(osiSymbols); i += 250 {
		end := i + 250
		if end > len(osiSymbols) {
			end = len(osiSymbols)
		}
		batch := osiSymbols[i:end]

		url := fmt.Sprintf("%s/userapigateway/option-details/%s/greeks?osiSymbols=%s", 
			c.BaseURL, c.AccountID, strings.Join(batch, ","))
		
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("get greeks failed: %s %s", resp.Status, string(body))
		}

		var res GreeksResponse
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			return nil, err
		}

		for _, g := range res.Greeks {
			var gamma float64
			fmt.Sscanf(g.Greeks.Gamma, "%f", &gamma)
			gammaMap[g.Symbol] = gamma
		}
	}

	return gammaMap, nil
}

type ExpirationsResponse struct {
	Expirations []string `json:"expirations"`
}

func (c *Client) GetExpirations(symbol string) ([]string, error) {
	reqBody := map[string]string{"instrument": symbol}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/userapigateway/marketdata/%s/option-expirations", c.BaseURL, c.AccountID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get expirations failed: %s %s", resp.Status, string(body))
	}

	var res ExpirationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return res.Expirations, nil
}
