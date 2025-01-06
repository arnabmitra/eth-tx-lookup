-- migrations/20251206000000_add_option_chain.up.sql
CREATE TABLE option_chain (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    symbol varchar(10) NOT NULL,
    spot_price varchar(500) NOT NULL,
    expiry_date date NOT NULL,
    expiry_type varchar(20) NOT NULL,
    option_chain jsonb,  -- Store the full option chain
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(symbol, expiry_date)
);

CREATE INDEX idx_option_chain_symbol ON option_chain(symbol);
CREATE INDEX idx_option_chain_date ON option_chain(expiry_date);