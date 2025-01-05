package optionexpirydates

import (
	"context"
	"github.com/arnabmitra/eth-proxy/internal/repository"
)

// Repository interface defines the contract for option expiry dates operations
type OptionExpiryDatesRepository interface {
	UpsertOptionExpiryDates(ctx context.Context, symbol string, expiryDates []string) (*repository.OptionExpiryDate, error)
	GetBySymbol(ctx context.Context, symbol string) (*repository.OptionExpiryDate, error)
}

// Implementation using sqlc-generated code
type OptionExpiryDatesRepo struct {
	q *repository.Queries
}

func NewOptionExpiryDatesRepository(q *repository.Queries) *OptionExpiryDatesRepo {
	return &OptionExpiryDatesRepo{q: q}
}

func (r *OptionExpiryDatesRepo) UpsertOptionExpiryDates(ctx context.Context, symbol string, expiryDates []byte) (*repository.OptionExpiryDate, error) {
	params := repository.UpsertOptionExpiryDatesParams{
		Symbol:      symbol,
		ExpiryDates: expiryDates,
	}

	result, err := r.q.UpsertOptionExpiryDates(ctx, params)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *OptionExpiryDatesRepo) GetBySymbol(ctx context.Context, symbol string) (repository.OptionExpiryDate, error) {
	return r.q.GetOptionExpiryDatesBySymbol(ctx, symbol)
}
