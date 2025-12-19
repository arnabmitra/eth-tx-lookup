-- Economic releases from FRED API
CREATE TABLE economic_releases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    release_id integer NOT NULL,
    release_name varchar(255) NOT NULL,
    release_date date NOT NULL,
    impact varchar(50) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(release_id, release_date)
);

CREATE INDEX idx_economic_releases_date ON economic_releases(release_date DESC);
CREATE INDEX idx_economic_releases_impact ON economic_releases(impact);
