CREATE TABLE option_expiry_dates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    symbol varchar(10) NOT NULL,
    expiry_dates jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(symbol)
);

CREATE INDEX idx_option_expiry_dates_symbol ON option_expiry(symbol);
