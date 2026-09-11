CREATE TABLE players (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 league_id BIGINT NOT NULL REFERENCES leagues(id),
 provider TEXT NOT NULL,
 external_id TEXT NOT NULL,
 name TEXT NOT NULL,
 observed_at TIMESTAMPTZ NOT NULL,
 UNIQUE (league_id,provider,external_id),
 UNIQUE (id,league_id,provider)
);
ALTER TABLE games ADD CONSTRAINT games_identity_scope_key UNIQUE (id,league_id,provider);
CREATE TABLE player_game_stats (
 game_id BIGINT NOT NULL,
 player_id BIGINT NOT NULL,
 team_id BIGINT NOT NULL,
 league_id BIGINT NOT NULL,
 provider TEXT NOT NULL,
 category TEXT NOT NULL CHECK(category<>''),
 position TEXT NOT NULL,
 jersey TEXT NOT NULL,
 did_not_play BOOLEAN NOT NULL,
 starter BOOLEAN,
 stats JSONB NOT NULL CHECK(jsonb_typeof(stats)='object'),
 additive_stats JSONB NOT NULL CHECK(jsonb_typeof(additive_stats)='object'),
 PRIMARY KEY(game_id,player_id,category),
 FOREIGN KEY(game_id,league_id,provider) REFERENCES games(id,league_id,provider) ON DELETE CASCADE,
 FOREIGN KEY(player_id,league_id,provider) REFERENCES players(id,league_id,provider),
 FOREIGN KEY(team_id,league_id,provider) REFERENCES teams(id,league_id,provider)
);
CREATE INDEX player_game_stats_player_idx ON player_game_stats(player_id,game_id);
CREATE TABLE player_game_imports (
 game_id BIGINT PRIMARY KEY REFERENCES games(id) ON DELETE CASCADE,
 observed_at TIMESTAMPTZ NOT NULL,
 line_count INTEGER NOT NULL CHECK(line_count>0),
 season INTEGER NOT NULL,
 season_type INTEGER NOT NULL,
 home_team_id BIGINT NOT NULL REFERENCES teams(id),
 away_team_id BIGINT NOT NULL REFERENCES teams(id)
);
-- Only expose statistics backed by an intact checkpoint matching the game.
CREATE VIEW player_complete_games AS
 SELECT g.* FROM games g JOIN player_game_imports i ON i.game_id=g.id
 WHERE g.status='final' AND g.season=i.season AND g.season_type=i.season_type
 AND g.home_team_id=i.home_team_id AND g.away_team_id=i.away_team_id
 AND i.line_count=(SELECT COUNT(*) FROM player_game_stats s WHERE s.game_id=g.id);
-- Historical membership observed in box scores; not a complete roster.
CREATE VIEW player_season_teams AS
 SELECT DISTINCT s.player_id,s.team_id,s.league_id,s.provider,g.season,g.season_type
 FROM player_game_stats s JOIN player_complete_games g ON g.id=s.game_id;
-- Totals over imported games only. Team scope preserves trades. Category keeps
-- NFL passing interceptions distinct from defensive interceptions.
CREATE VIEW player_season_totals AS
 SELECT s.player_id,s.team_id,s.league_id,s.provider,g.season,g.season_type,
 s.category,metric.key AS metric,SUM(metric.value::numeric) AS total,
 COUNT(DISTINCT g.id) AS games_with_metric
 FROM player_game_stats s JOIN player_complete_games g ON g.id=s.game_id
 CROSS JOIN LATERAL jsonb_each_text(s.additive_stats) metric
 WHERE g.status='final' AND NOT s.did_not_play
 GROUP BY s.player_id,s.team_id,s.league_id,s.provider,g.season,g.season_type,s.category,metric.key;
CREATE VIEW player_season_coverage AS
 SELECT g.league_id,g.provider,g.season,g.season_type,COUNT(*) AS stored_final_games,
 COUNT(i.game_id) FILTER (WHERE i.season=g.season AND i.season_type=g.season_type
 AND i.home_team_id=g.home_team_id AND i.away_team_id=g.away_team_id
 AND i.line_count=(SELECT COUNT(*) FROM player_game_stats s WHERE s.game_id=g.id)) AS imported_player_games
 FROM games g LEFT JOIN player_game_imports i ON i.game_id=g.id
 WHERE g.status='final' GROUP BY g.league_id,g.provider,g.season,g.season_type;
