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

func (c *Client) do(method, url string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewBuffer(data)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	
	req.Header.Set("User-Agent", "public-go-client/1.0")

	return c.HTTPClient.Do(req)
}

func (c *Client) Authenticate() error {
	reqBody := AccessTokenRequest{
		Secret:            c.Secret,
		ValidityInMinutes: 60,
	}

	url := c.BaseURL + "/userapiauthservice/personal/access-tokens"
	resp, err := c.do("POST", url, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var res AccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}

	c.Token = res.AccessToken
	return nil
}

type AccountResponse struct {
	Account *struct {
		AccountID string `json:"accountId"`
	} `json:"account"`
	Accounts []struct {
		AccountID string `json:"accountId"`
	} `json:"accounts"`
}

func (c *Client) FetchAccountID() error {
	if c.AccountID != "" {
		return nil
	}

	url := c.BaseURL + "/userapigateway/trading/accounts"
	resp, err := c.do("GET", url, nil)
	if err != nil {
		return err
	}
	
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		url = c.BaseURL + "/userapigateway/trading/account"
		resp, err = c.do("GET", url, nil)
		if err != nil {
			return err
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fetch account failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var res AccountResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}

	if res.Account != nil && res.Account.AccountID != "" {
		c.AccountID = res.Account.AccountID
	} else if len(res.Accounts) > 0 {
		c.AccountID = res.Accounts[0].AccountID
	}

	if c.AccountID == "" {
		return fmt.Errorf("no account ID found in response")
	}

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

	url := fmt.Sprintf("%s/userapigateway/marketdata/%s/quotes", c.BaseURL, c.AccountID)
	resp, err := c.do("POST", url, reqBody)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("get quotes failed (HTTP %d): %s", resp.StatusCode, string(body))
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
	Instrument     Instrument `json:"instrument"`
	ExpirationDate string     `json:"expirationDate,omitempty"`
}

type OptionContract struct {
	Instrument   Instrument `json:"instrument"`
	StrikePrice  string     `json:"strikePrice"`
	OptionType   string     `json:"optionType"`
	OpenInterest int        `json:"openInterest"`
	Greeks       *struct {
		Gamma string `json:"gamma"`
		Delta string `json:"delta"`
	} `json:"greeks"`
}

type OptionChainResponse struct {
	Calls []OptionContract `json:"calls"`
	Puts  []OptionContract `json:"puts"`
}

func (c *Client) GetOptionChain(symbol string, expiration string) (*OptionChainResponse, error) {
	reqBody := OptionChainRequest{
		Instrument:     Instrument{Symbol: symbol, Type: "EQUITY"},
		ExpirationDate: expiration,
	}

	url := fmt.Sprintf("%s/userapigateway/marketdata/%s/option-chain", c.BaseURL, c.AccountID)
	resp, err := c.do("POST", url, reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get option chain failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var res OptionChainResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}

	if len(res.Calls) == 0 && len(res.Puts) == 0 {
		fmt.Printf("DEBUG: Public.com returned 0 calls and 0 puts. Raw body snippet: %s\n", string(body[:200]))
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
	gammaMap := make(map[string]float64)

	for i := 0; i < len(osiSymbols); i += 250 {
		end := i + 250
		if end > len(osiSymbols) {
			end = len(osiSymbols)
		}
		batch := osiSymbols[i:end]

		url := fmt.Sprintf("%s/userapigateway/option-details/%s/greeks?osiSymbols=%s", 
			c.BaseURL, c.AccountID, strings.Join(batch, ","))
		
		resp, err := c.do("GET", url, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("get greeks failed (HTTP %d): %s", resp.StatusCode, string(body))
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
	reqBody := struct {
		Instrument Instrument `json:"instrument"`
	}{
		Instrument: Instrument{Symbol: symbol, Type: "EQUITY"},
	}

	url := fmt.Sprintf("%s/userapigateway/marketdata/%s/option-expirations", c.BaseURL, c.AccountID)
	resp, err := c.do("POST", url, reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get expirations failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var res ExpirationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return res.Expirations, nil
}
