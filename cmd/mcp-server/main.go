package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/arnabmitra/eth-proxy/internal/handler/gex"
	"github.com/arnabmitra/eth-proxy/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
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
	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		dbUrl = "postgres://guestbook:guestbook@localhost:5432/guestbook"
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

	mcpServer := mcp.NewServer(stdio.NewStdioServerTransport())

	// Register Tools
	err = mcpServer.RegisterTool("get_gex_regime", "Get the current Gamma Exposure regime, spot price, and flip level for a symbol.", server.GetRegime)
	if err != nil {
		panic(err)
	}

	err = mcpServer.RegisterTool("get_gex_anomalies", "Identify stocks with the highest statistical GEX deviations (Z-scores) from our database.", server.GetAnomalies)
	if err != nil {
		panic(err)
	}

	// Serve the MCP server over stdio
	if err := mcpServer.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "Error serving MCP: %v\n", err)
		os.Exit(1)
	}
}
