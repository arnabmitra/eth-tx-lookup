package optionchain

import (
	"context"
	"github.com/arnabmitra/eth-proxy/internal/repository"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Repository interface defines the contract for option expiry operations
type OptionChainRepository interface {
	UpsertOptionChain(ctx context.Context, symbol string, expiryDate time.Time, chainData []byte) (*repository.OptionExpiry, error)
	GetBySymbolAndExpirationDate(ctx context.Context, symbol string) (repository.OptionChain, error)
}

// Implementation using sqlc-generated code
type optionChainRepo struct {
	q *repository.Queries
}

func NewOptionChainRepository(q *repository.Queries) *optionChainRepo {
	return &optionChainRepo{q: q}
}

func (r *optionChainRepo) UpsertOptionChain(ctx context.Context, symbol string, expiryDate time.Time, chainData []byte, spotPrice string) (*repository.OptionChain, error) {
	// Convert time.Time to pgtype.Date
	pgDate := pgtype.Date{
		Time:  expiryDate,
		Valid: true,
	}

	params := repository.UpsertOptionChainParams{
		Symbol:      symbol,
		SpotPrice:   spotPrice,
		ExpiryDate:  pgDate,
		ExpiryType:  "standard",
		OptionChain: chainData,
	}

	result, err := r.q.UpsertOptionChain(ctx, params)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *optionChainRepo) GetBySymbolAndExpirationDate(ctx context.Context, symbol string, expirationDate pgtype.Date) (repository.OptionChain, error) {
	return r.q.GetOptionChainBySymbolAndExpiry(ctx, repository.GetOptionChainBySymbolAndExpiryParams{symbol, expirationDate})
}

// Helper function to convert pgtype.Date to time.Time
func DateToTime(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return d.Time
}

// Helper function to convert time.Time to pgtype.Date
func TimeToPgDate(t time.Time) pgtype.Date {
	return pgtype.Date{
		Time:  t,
		Valid: !t.IsZero(),
	}
}
