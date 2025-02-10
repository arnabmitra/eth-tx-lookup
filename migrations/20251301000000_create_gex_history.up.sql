-- migrations/20251207000000_create_gex_history.up.sql
CREATE TABLE gex_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    symbol varchar(10) NOT NULL,
    expiry_date date NOT NULL,
    expiry_type varchar(20) NOT NULL,
    option_chain jsonb,  -- Store the full option chain
    gex_value numeric NOT NULL,  -- Store the GEX value
    recorded_at timestamptz NOT NULL DEFAULT now(),  -- Timestamp of the record
    UNIQUE(symbol, expiry_date, recorded_at)
);

CREATE INDEX idx_gex_history_symbol ON gex_history(symbol);
CREATE INDEX idx_gex_history_expiry_date ON gex_history(expiry_date);
CREATE INDEX idx_gex_history_symbol_expiry_date ON gex_history(symbol, expiry_date);