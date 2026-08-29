package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/arnabmitra/eth-proxy/internal/handler/gex"
	"github.com/arnabmitra/eth-proxy/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type GEXMcpServer struct {
	db   *pgxpool.Pool
	repo *repository.Queries
}

func (s *GEXMcpServer) GetRegimeHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol, err := request.RequireString("symbol")
	if err != nil {
		return mcp.NewToolResultError("symbol argument is required"), nil
	}

	apiKey, apiSecret := gex.GetAlpacaConfig()
	if apiKey == "" {
		return mcp.NewToolResultError("ALPACA_API_KEY not set"), nil
	}

	symbol = strings.ToUpper(symbol)
	price, err := gex.GetSpotPrice(apiKey, apiSecret, symbol)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("error getting spot price: %v", err)), nil
	}

	expirations, err := gex.GetExpirationDates(apiKey, apiSecret, symbol)
	if err != nil || len(expirations) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("error getting expirations: %v", err)), nil
	}

	options, _, warning, err := gex.FetchOptionsChain(symbol, expirations[0], apiKey, apiSecret)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("error fetching options: %v", err)), nil
	}

	gexByStrike := gex.CalculateGEXPerStrike(options, price)
	flip := gex.CalculateGammaFlipLevel(gexByStrike)

	totalCallGEX := 0.0
	totalPutGEX := 0.0
	for _, opt := range options {
		if opt.Greeks.Gamma != 0 {
			val := float64(opt.OpenInterest) * opt.Greeks.Gamma * 100 * price
			if strings.ToLower(opt.OptionType) == "call" {
				totalCallGEX += val
			} else {
				totalPutGEX += val
			}
		}
	}

	netGEX := totalCallGEX - totalPutGEX
	condition := "Negative"
	if netGEX > 0 {
		condition = "Positive"
	}

	report := fmt.Sprintf("Regime for %s:\n- Spot Price: %.2f\n- Net GEX: $%.2fM\n- Gamma Condition: %s\n- Gamma Flip: %.2f\n",
		symbol, price, netGEX/1000000.0, condition, flip)

	if warning != "" {
		report += fmt.Sprintf("\nWarning: %s", warning)
	}

	return mcp.NewToolResultText(report), nil
}

func (s *GEXMcpServer) GetAnomaliesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := int(request.GetFloat("limit", 5.0))

	anomalies, err := s.repo.GetGEXAnomalies(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("error fetching anomalies: %v", err)), nil
	}

	var sb strings.Builder
	sb.WriteString("Current GEX Anomalies (Z-Score):\n\n")

	count := 0
	for _, a := range anomalies {
		if count >= limit {
			break
		}

		var zScoreVal float64
		if a.ZScore.Valid {
			f, _ := a.ZScore.Float64Value()
			zScoreVal = f.Float64
		}

		var gexValueM float64
		if a.GexValue.Valid {
			f, _ := a.GexValue.Float64Value()
			gexValueM = f.Float64 / 1000000.0
		}

		spotPriceStr := "N/A"
		if a.SpotPrice.Valid {
			spotPriceStr = a.SpotPrice.String
		}

		sb.WriteString(fmt.Sprintf("- %s: GEX $%.2fM (Z-Score: %.2f) @ Price %s\n",
			a.Symbol, gexValueM, zScoreVal, spotPriceStr))
		count++
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func main() {
	godotenv.Load()

	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		fmt.Fprintf(os.Stderr, "Error: DATABASE_URL environment variable is missing\n")
		os.Exit(1)
	}

	pool, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	gexSrv := &GEXMcpServer{
		db:   pool,
		repo: repository.New(pool),
	}

	mcpServer := server.NewMCPServer("gex-server", "1.0.0")

	// Register tools
	regimeTool := mcp.NewTool("get_gex_regime",
		mcp.WithDescription("Get the current Gamma Exposure regime, spot price, and flip level for a symbol."),
		mcp.WithString("symbol", mcp.Required(), mcp.Description("The stock ticker symbol (e.g., SPY, ARM, NET)")),
	)
	mcpServer.AddTool(regimeTool, gexSrv.GetRegimeHandler)

	anomaliesTool := mcp.NewTool("get_gex_anomalies",
		mcp.WithDescription("Identify stocks with the highest statistical GEX deviations (Z-scores) from our database."),
		mcp.WithNumber("limit", mcp.Description("Number of anomalies to return, default=5")),
	)
	mcpServer.AddTool(anomaliesTool, gexSrv.GetAnomaliesHandler)

	// Create SSE server wrapper
	sseServer := server.NewSSEServer(mcpServer)

	mux := http.NewServeMux()
	// Depending on library version, might use Handle or HandleFunc, but SSEHandler() returns http.Handler
	mux.Handle("/sse", sseServer.SSEHandler())
	mux.Handle("/message", sseServer.MessageHandler())

	// Auth Middleware
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")

			var exists bool
			err := pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM mcp_api_keys WHERE token = $1)", token).Scan(&exists)
			if err != nil || !exists {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}

	fmt.Println("Starting authenticated MCP SSE server on :8081...")
	if err := http.ListenAndServe(":8081", authMiddleware(mux)); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting HTTP server: %v\n", err)
		os.Exit(1)
	}
}
