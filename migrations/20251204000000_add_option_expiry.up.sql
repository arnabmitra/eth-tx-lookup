CREATE TABLE option_expiry (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    symbol varchar(10) NOT NULL,
    expiry_date date NOT NULL,
    expiry_type varchar(20) NOT NULL,
    option_chain jsonb,  -- Store the full option chain
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(symbol, expiry_date)
);

CREATE INDEX idx_option_expiry_symbol ON option_expiry(symbol);
CREATE INDEX idx_option_expiry_date ON option_expiry(expiry_date);