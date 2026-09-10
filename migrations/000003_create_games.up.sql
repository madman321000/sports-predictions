ALTER TABLE teams ADD CONSTRAINT teams_identity_scope_key UNIQUE (id, league_id, provider);

CREATE TABLE games (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    league_id BIGINT NOT NULL REFERENCES leagues(id),
    provider TEXT NOT NULL,
    external_id TEXT NOT NULL,
    home_team_id BIGINT NOT NULL,
    away_team_id BIGINT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('scheduled', 'in_progress', 'final', 'postponed', 'canceled', 'suspended', 'delayed')),
    home_score INTEGER CHECK (home_score >= 0),
    away_score INTEGER CHECK (away_score >= 0),
    season INTEGER NOT NULL CHECK (season > 0),
    season_type INTEGER NOT NULL CHECK (season_type > 0),
    week INTEGER CHECK (week > 0),
    observed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, league_id, external_id),
    FOREIGN KEY (home_team_id, league_id, provider) REFERENCES teams(id, league_id, provider),
    FOREIGN KEY (away_team_id, league_id, provider) REFERENCES teams(id, league_id, provider),
    CHECK (home_team_id <> away_team_id),
    CHECK (status <> 'final' OR (home_score IS NOT NULL AND away_score IS NOT NULL))
);
CREATE INDEX games_league_starts_at_idx ON games (league_id, starts_at);
