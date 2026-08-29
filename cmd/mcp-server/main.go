package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"net/http"

	"github.com/arnabmitra/eth-proxy/internal/handler/gex"
	"github.com/arnabmitra/eth-proxy/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/sse"
)

type GEXMcpServer struct {
	db   *pgxpool.Pool
	repo *repository.Queries
}

type RegimeArgs struct {
	Symbol string `json:"symbol" jsonschema:"required,description=The stock ticker symbol (e.g., SPY, ARM, NET)"`
}

func (s *GEXMcpServer) GetRegime(ctx context.Context, args RegimeArgs) (*mcp.ToolResponse, error) {
	apiKey, apiSecret := gex.GetAlpacaConfig()
	if apiKey == "" {
		return nil, fmt.Errorf("ALPACA_API_KEY not set")
	}

	symbol := strings.ToUpper(args.Symbol)
	price, err := gex.GetSpotPrice(apiKey, apiSecret, symbol)
	if err != nil {
		return nil, fmt.Errorf("error getting spot price: %v", err)
	}

	expirations, err := gex.GetExpirationDates(apiKey, apiSecret, symbol)
	if err != nil || len(expirations) == 0 {
		return nil, fmt.Errorf("error getting expirations: %v", err)
	}

	options, _, warning, err := gex.FetchOptionsChain(symbol, expirations[0], apiKey, apiSecret)
	if err != nil {
		return nil, fmt.Errorf("error fetching options: %v", err)
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

	return mcp.NewToolResponse(mcp.NewTextContent(report)), nil
}

type AnomaliesArgs struct {
	Limit int `json:"limit" jsonschema:"description=Number of anomalies to return,default=5"`
}

func (s *GEXMcpServer) GetAnomalies(ctx context.Context, args AnomaliesArgs) (*mcp.ToolResponse, error) {
	if args.Limit == 0 {
		args.Limit = 5
	}

	anomalies, err := s.repo.GetGEXAnomalies(ctx)
	if err != nil {
		return nil, fmt.Errorf("error fetching anomalies: %v", err)
	}

	var sb strings.Builder
	sb.WriteString("Current GEX Anomalies (Z-Score):\n\n")
	
	count := 0
	for _, a := range anomalies {
		if count >= args.Limit {
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
		
		// Handle pgtype.Text for SpotPrice
		spotPriceStr := "N/A"
		if a.SpotPrice.Valid {
			spotPriceStr = a.SpotPrice.String
		}
		
		sb.WriteString(fmt.Sprintf("- %s: GEX $%.2fM (Z-Score: %.2f) @ Price %s\n",
			a.Symbol, gexValueM, zScoreVal, spotPriceStr))
		count++
	}

	return mcp.NewToolResponse(mcp.NewTextContent(sb.String())), nil
}

func main() {
	// Automatically load .env file from the root directory just like the main app does
	godotenv.Load()

	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		fmt.Fprintf(os.Stderr, "Error: DATABASE_URL environment variable is missing (check your .env file)\n")
		os.Exit(1)
	}

	pool, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	server := &GEXMcpServer{
		db:   pool,
		repo: repository.New(pool),
	}

	transport := sse.NewSSEServerTransport("/message")
	mcpServer := mcp.NewServer(transport)

	// Register Tools
	err = mcpServer.RegisterTool("get_gex_regime", "Get the current Gamma Exposure regime, spot price, and flip level for a symbol.", server.GetRegime)
	if err != nil {
		panic(err)
	}

	err = mcpServer.RegisterTool("get_gex_anomalies", "Identify stocks with the highest statistical GEX deviations (Z-scores) from our database.", server.GetAnomalies)
	if err != nil {
		panic(err)
	}

	// Serve the MCP server
	go func() {
		if err := mcpServer.Serve(); err != nil {
			fmt.Fprintf(os.Stderr, "Error serving MCP: %v\n", err)
			os.Exit(1)
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/sse", transport.HandleSSE())
	mux.Handle("/message", transport.HandleMessage())

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
