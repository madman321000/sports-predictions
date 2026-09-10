-- A successful full response is required before existing rows can skip a request.
CREATE TABLE team_imports (
    league_id BIGINT NOT NULL REFERENCES leagues(id),
    provider TEXT NOT NULL,
    external_ids TEXT[] NOT NULL CHECK (cardinality(external_ids) > 0),
    imported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (league_id, provider)
);

-- Store ESPN's requested date explicitly; it is not the game's UTC start date.
CREATE TABLE game_date_imports (
    league_id BIGINT NOT NULL REFERENCES leagues(id),
    provider TEXT NOT NULL,
    import_date DATE NOT NULL,
    external_ids TEXT[] NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (league_id, provider, import_date)
);
