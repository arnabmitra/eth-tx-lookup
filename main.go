package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/arnabmitra/eth-proxy/internal/app"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	a := app.New(logger)
	// Serve static files from the "static" directory
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	// Register the handler function before starting the server
	http.HandleFunc("/eth-tx", ethTxHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/eth-tx", http.StatusSeeOther)
	})

	// Start the server on port 8080
	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			logger.Error("failed to start server", slog.Any("error", err))
		}
	}()

	if err := a.Start(ctx); err != nil {
		logger.Error("failed to start server", slog.Any("error", err))
	}
}

func fetchTxDetails(txHash string) (*AlchemyResponse, error) {

	apiKey := os.Getenv("ETH_API_KEY")
	url := fmt.Sprintf("https://eth-mainnet.alchemyapi.io/v2/%s", apiKey)

	payload := fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":["%s"],"id":1}`, txHash)
	resp, err := http.Post(url, "application/json", strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var alchemyResponse AlchemyResponse
	if err := json.NewDecoder(resp.Body).Decode(&alchemyResponse); err != nil {
		return nil, err
	}

	fmt.Printf("The response from alchemy is %v \n ", alchemyResponse)
	return &alchemyResponse, nil
}

func ethTxHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		txHash := r.FormValue("txhash")
		details, err := fetchTxDetails(txHash)
		if err != nil {
			http.Error(w, "Failed to fetch transaction details", http.StatusInternalServerError)
			return
		}

		tmpl, err := template.ParseFiles("templates/eth.html")
		if err != nil {
			http.Error(w, "Failed to load template", http.StatusInternalServerError)
			return
		}

		tmpl.Execute(w, struct {
			TxHash  string
			Details *AlchemyResponse
		}{TxHash: txHash, Details: details})
	} else {
		tmpl, err := template.ParseFiles("templates/eth.html")
		if err != nil {
			http.Error(w, "Failed to load template", http.StatusInternalServerError)
			return
		}

		tmpl.Execute(w, nil)
	}
}

type AlchemyResponse struct {
	Jsonrpc string `json:"jsonrpc"`
	Id      int    `json:"id"`
	Result  struct {
		BlockHash        string `json:"blockHash"`
		BlockNumber      HexInt `json:"blockNumber"`
		From             string `json:"from"`
		Gas              string `json:"gas"`
		GasPrice         string `json:"gasPrice"`
		Hash             string `json:"hash"`
		Input            string `json:"input"`
		Nonce            string `json:"nonce"`
		To               string `json:"to"`
		TransactionIndex string `json:"transactionIndex"`
		Value            HexInt `json:"value"`
		V                string `json:"v"`
		R                string `json:"r"`
		S                string `json:"s"`
	} `json:"result"`
}
type HexInt int64

func (h *HexInt) UnmarshalJSON(data []byte) error {
	// Remove the quotes around the JSON string
	hexStr := strings.Trim(string(data), "\"")
	if len(hexStr) > 2 && hexStr[:2] == "0x" {
		hexStr = hexStr[3:]
	}
	// Convert the hexadecimal string to an integer
	intValue, err := strconv.ParseInt(hexStr, 16, 64)
	if err != nil {
		return err
	}
	*h = HexInt(intValue)
	return nil
}
