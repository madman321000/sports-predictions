CREATE TABLE teams (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    league_id BIGINT NOT NULL REFERENCES leagues(id),
    provider TEXT NOT NULL,
    external_id TEXT NOT NULL,
    name TEXT NOT NULL,
    abbreviation TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, league_id, external_id)
);
