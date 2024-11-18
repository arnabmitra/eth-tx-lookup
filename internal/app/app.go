package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/arnabmitra/eth-proxy/internal/database"
	"github.com/arnabmitra/eth-proxy/internal/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type App struct {
	logger *slog.Logger
	router *http.ServeMux
	db     *pgxpool.Pool
	rdb    *redis.Client
}

func New(logger *slog.Logger) *App {
	router := http.NewServeMux()

	redisAddr, exists := os.LookupEnv("REDIS_ADDR")
	if !exists {
		redisAddr = "localhost:6379"
	}

	app := &App{
		logger: logger,
		router: router,
		rdb: redis.NewClient(&redis.Options{
			Addr: redisAddr,
		}),
	}

	return app
}

func (a *App) Start(ctx context.Context) error {
	db, err := database.Connect(ctx, a.logger)
	if err != nil {
		return fmt.Errorf("failed to connect to db: %w", err)
	}

	a.db = db

	tmpl := template.Must(template.New("").ParseGlob("./templates/*"))

	a.loadRoutes()

	server := http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: middleware.Logging(a.logger, middleware.HandleBadCode(tmpl, a.router)),
	}

	done := make(chan struct{})
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("failed to listen and serve", slog.Any("error", err))
		}
		close(done)
	}()

	a.logger.Info("Server listening", slog.String("addr", ":8080"))
	select {
	case <-done:
		break
	case <-ctx.Done():
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		server.Shutdown(ctx)
		cancel()
	}

	return nil
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
