DROP VIEW player_season_coverage;
DROP VIEW player_season_totals;
DROP VIEW player_season_teams;
DROP VIEW player_complete_games;
DROP TABLE player_game_imports;
DROP TABLE player_game_stats;
ALTER TABLE games DROP CONSTRAINT games_identity_scope_key;
DROP TABLE players;
