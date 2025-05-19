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

-- name: GetOptionExpiryBySymbolAndExpiry :many
SELECT * FROM option_expiry
WHERE symbol = $1 AND expiry_date = $2
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

-- name: UpsertOptionChain :one
INSERT INTO option_chain (
    symbol,
    spot_price,
    expiry_date,
    expiry_type,
    option_chain
) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (symbol, expiry_date)
DO UPDATE SET
    spot_price = EXCLUDED.spot_price,
    option_chain = EXCLUDED.option_chain,
    updated_at = now()
RETURNING *;

-- name: GetOptionChainBySymbolAndExpiry :one
SELECT * FROM option_chain
WHERE symbol = $1 and expiry_date = $2;

-- name: InsertGEXHistory :one
INSERT INTO gex_history (
    id, symbol, expiry_date, expiry_type, option_chain, gex_value, recorded_at, spot_price
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    RETURNING *;


-- name: GetGexHistoryBySymbolAndExpiry :many
SELECT * FROM gex_history
WHERE symbol = $1 AND expiry_date = $2
ORDER BY recorded_at DESC
    LIMIT $3;

-- name: GetLatestGEXHistoryBySymbol :many
SELECT id, symbol, expiry_date, recorded_at, gex_value, option_chain, spot_price
FROM gex_history
WHERE symbol = $1 AND recorded_at >= $2
ORDER BY recorded_at DESC
    LIMIT $3;