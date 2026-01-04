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

-- name: GetLatestGEXForAllSymbols :many
WITH latest_records AS (
    SELECT DISTINCT ON (gh.symbol)
        gh.symbol,
        gh.gex_value,
        gh.spot_price,
        gh.expiry_date,
        gh.recorded_at
    FROM gex_history gh
    WHERE gh.recorded_at >= $1
    ORDER BY gh.symbol, gh.recorded_at DESC
)
SELECT * FROM latest_records
ORDER BY symbol;

-- name: GetGEXChangeForSymbols :many
WITH latest AS (
    SELECT DISTINCT ON (symbol)
        symbol,
        gex_value as current_gex,
        spot_price as current_price,
        expiry_date,
        recorded_at as current_time
    FROM gex_history
    WHERE gex_history.recorded_at >= $1
    ORDER BY symbol, expiry_date ASC, gex_history.recorded_at DESC
),
previous AS (
    SELECT DISTINCT ON (symbol)
        symbol,
        gex_value as previous_gex,
        recorded_at as previous_time
    FROM gex_history
    WHERE gex_history.recorded_at >= $2 AND gex_history.recorded_at < $1
    ORDER BY symbol, expiry_date ASC, gex_history.recorded_at DESC
)
SELECT
    l.symbol,
    l.current_gex,
    l.current_price,
    l.expiry_date,
    l.current_time,
    COALESCE(p.previous_gex, 0) as previous_gex,
    p.previous_time,
    (l.current_gex - COALESCE(p.previous_gex, 0))::numeric as gex_change,
    (CASE
        WHEN COALESCE(p.previous_gex, 0) != 0
        THEN ((l.current_gex - COALESCE(p.previous_gex, 0)) / ABS(p.previous_gex)) * 100
        ELSE 0
    END)::numeric as gex_change_pct
FROM latest l
LEFT JOIN previous p ON l.symbol = p.symbol
ORDER BY ABS(l.current_gex - COALESCE(p.previous_gex, 0)) DESC;

-- Economic Releases (FRED API)
-- name: UpsertEconomicRelease :one
INSERT INTO economic_releases (release_id, release_name, release_date, impact)
VALUES ($1, $2, $3, $4)
ON CONFLICT (release_id, release_date)
DO UPDATE SET
    release_name = EXCLUDED.release_name,
    impact = EXCLUDED.impact,
    updated_at = now()
RETURNING *;

-- name: GetThisWeekReleases :many
SELECT * FROM economic_releases
WHERE release_date >= CURRENT_DATE - 7 AND release_date <= CURRENT_DATE + 7
ORDER BY release_date DESC, impact DESC;

-- name: GetUpcomingReleases :many
SELECT * FROM economic_releases
WHERE release_date >= $1 AND release_date <= $2
ORDER BY release_date ASC, impact DESC;