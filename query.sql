-- name: Insert :one
INSERT INTO guest (id, message, created_at, updated_at, ip)
VALUES ($1, $2, $3, $3, $4)
RETURNING *;

-- name: FindAll :many
SELECT *
FROM guest
ORDER BY created_at DESC
LIMIT $1;

-- name: Count :one
SELECT COUNT(*) FROM guest;

-- name: UpsertOptionExpiry :one
INSERT INTO option_expiry (
    symbol,
    expiry_date,
    expiry_type,
    option_chain
) VALUES ($1, $2, $3, $4)
ON CONFLICT (symbol, expiry_date)
DO UPDATE SET
    option_chain = EXCLUDED.option_chain,
    updated_at = now()
RETURNING *;

-- name: GetOptionExpiryBySymbol :many
SELECT * FROM option_expiry
WHERE symbol = $1
ORDER BY expiry_date ASC;

-- name: UpsertOptionExpiryDates :one
INSERT INTO option_expiry_dates (
    symbol,
    expiry_dates
) VALUES ($1, $2)
ON CONFLICT (symbol)
    DO UPDATE SET
                  expiry_dates = EXCLUDED.expiry_dates,
                  updated_at = now()
RETURNING *;

-- name: GetOptionExpiryDatesBySymbol :one
SELECT * FROM option_expiry_dates
WHERE symbol = $1;
