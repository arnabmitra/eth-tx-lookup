package optionexpiry

import (
	"context"
	"github.com/arnabmitra/eth-proxy/internal/repository"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Repository interface defines the contract for option expiry operations
type OptionExpiryRepository interface {
	UpsertOptionExpiry(ctx context.Context, symbol string, expiryDate time.Time, chainData []byte) (*repository.OptionExpiry, error)
	GetBySymbol(ctx context.Context, symbol string) ([]repository.OptionExpiry, error)
}

// Implementation using sqlc-generated code
type optionExpiryRepo struct {
	q *repository.Queries
}

func NewOptionExpiryRepository(q *repository.Queries) OptionExpiryRepository {
	return &optionExpiryRepo{q: q}
}

func (r *optionExpiryRepo) UpsertOptionExpiry(ctx context.Context, symbol string, expiryDate time.Time, chainData []byte) (*repository.OptionExpiry, error) {
	// Convert time.Time to pgtype.Date
	pgDate := pgtype.Date{
		Time:  expiryDate,
		Valid: true,
	}

	params := repository.UpsertOptionExpiryParams{
		Symbol:      symbol,
		ExpiryDate:  pgDate,
		ExpiryType:  "standard",
		OptionChain: chainData,
	}

	result, err := r.q.UpsertOptionExpiry(ctx, params)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *optionExpiryRepo) GetBySymbol(ctx context.Context, symbol string) ([]repository.OptionExpiry, error) {
	return r.q.GetOptionExpiryBySymbol(ctx, symbol)
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
