package gex_history

import (
	"context"
	"github.com/arnabmitra/eth-proxy/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"log"
	"time"
)

// Repository interface defines the contract for GEX history operations
type GexHistoryRepository interface {
	InsertGexHistory(ctx context.Context, symbol string, expiryDate time.Time, expiryType string, optionChain []byte, gexValue float64) (*repository.GexHistory, error)
	GetGexHistoryBySymbolAndExpiry(ctx context.Context, symbol string, expiryDate time.Time) ([]repository.GexHistory, error)
}

// Implementation using sqlc-generated code
type gexHistoryRepo struct {
	q *repository.Queries
}

func NewGexHistoryRepository(q *repository.Queries) *gexHistoryRepo {
	return &gexHistoryRepo{q: q}
}

func (r *gexHistoryRepo) InsertGexHistory(ctx context.Context, symbol string, expiryDate time.Time, expiryType string, optionChain []byte, gexValue float64) (*repository.GexHistory, error) {
	// Convert time.Time to pgtype.Date
	pgDate := pgtype.Date{
		Time:  expiryDate,
		Valid: true,
	}

	var x pgtype.Numeric
	if err := x.Scan(gexValue); err != nil {
		log.Panic(err)
	}
	params := repository.InsertGexHistoryParams{
		ID:          uuid.New(),
		Symbol:      symbol,
		ExpiryDate:  pgDate,
		ExpiryType:  expiryType,
		OptionChain: optionChain,
		GexValue:    x,
	}

	result, err := r.q.InsertGexHistory(ctx, params)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *gexHistoryRepo) GetGexHistoryBySymbolAndExpiry(ctx context.Context, symbol string, expiryDate time.Time) ([]repository.GexHistory, error) {
	pgDate := pgtype.Date{
		Time:  expiryDate,
		Valid: true,
	}

	return r.q.GetGexHistoryBySymbolAndExpiry(ctx, repository.GetGexHistoryBySymbolAndExpiryParams{Symbol: symbol, ExpiryDate: pgDate})
}
