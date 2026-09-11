package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/madman321000/sports-predictions/internal/player"
)

type PostgresPlayerRepository struct{ pool *pgxpool.Pool }

func NewPostgresPlayerRepository(pool *pgxpool.Pool) *PostgresPlayerRepository {
	return &PostgresPlayerRepository{pool: pool}
}
func (r *PostgresPlayerRepository) PlayerGames(ctx context.Context, league string, season, seasonType int) ([]player.GameRef, error) {
	rows, err := r.pool.Query(ctx, `SELECT g.id,g.external_id,h.external_id,a.external_id,g.season,g.season_type
 FROM games g JOIN leagues l ON l.id=g.league_id JOIN teams h ON h.id=g.home_team_id JOIN teams a ON a.id=g.away_team_id
 WHERE l.abbreviation=$1 AND g.provider='espn' AND g.season=$2 AND g.season_type=$3 AND g.status='final'
 ORDER BY g.starts_at,g.id`, league, season, seasonType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var games []player.GameRef
	for rows.Next() {
		var g player.GameRef
		if err := rows.Scan(&g.ID, &g.ExternalID, &g.HomeTeamExternalID, &g.AwayTeamExternalID, &g.Season, &g.SeasonType); err != nil {
			return nil, err
		}
		games = append(games, g)
	}
	return games, rows.Err()
}
func (r *PostgresPlayerRepository) PlayerGameComplete(ctx context.Context, g player.GameRef) (bool, error) {
	var complete bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM player_game_imports i JOIN games g ON g.id=i.game_id
 WHERE g.id=$1 AND g.status='final' AND g.season=i.season AND g.season_type=i.season_type
 AND g.home_team_id=i.home_team_id AND g.away_team_id=i.away_team_id
 AND i.line_count=(SELECT COUNT(*) FROM player_game_stats s WHERE s.game_id=g.id))`, g.ID).Scan(&complete)
	return complete, err
}
func (r *PostgresPlayerRepository) SavePlayerGame(ctx context.Context, g player.GameRef, box player.BoxScore) error {
	if len(box.Lines) == 0 || box.ObservedAt.IsZero() {
		return fmt.Errorf("empty player box score or observation time")
	}
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var leagueID, home, away int64
		var provider, externalID, status string
		var season, seasonType int
		if err := tx.QueryRow(ctx, `SELECT league_id,provider,external_id,status,season,season_type,home_team_id,away_team_id FROM games WHERE id=$1 FOR UPDATE`, g.ID).Scan(&leagueID, &provider, &externalID, &status, &season, &seasonType, &home, &away); err != nil {
			return err
		}
		if externalID != g.ExternalID || provider != "espn" || status != "final" || season != g.Season || seasonType != g.SeasonType {
			return fmt.Errorf("stored player game identity changed")
		}
		var previous *time.Time
		if err := tx.QueryRow(ctx, `SELECT (SELECT observed_at FROM player_game_imports WHERE game_id=$1)`, g.ID).Scan(&previous); err != nil {
			return err
		}
		if previous != nil && previous.After(box.ObservedAt) {
			return nil
		}
		if _, err := tx.Exec(ctx, `DELETE FROM player_game_stats WHERE game_id=$1`, g.ID); err != nil {
			return err
		}
		lines := slices.Clone(box.Lines)
		slices.SortFunc(lines, func(a, b player.Line) int { return strings.Compare(a.ExternalID, b.ExternalID) })
		seenTeams := map[int64]bool{}
		for _, line := range lines {
			if line.ExternalID == "" || strings.TrimSpace(line.Name) == "" || line.Category == "" || line.Stats == nil {
				return fmt.Errorf("invalid player line")
			}
			var teamID, playerID int64
			if err := tx.QueryRow(ctx, `SELECT id FROM teams WHERE league_id=$1 AND provider=$2 AND external_id=$3`, leagueID, provider, line.TeamExternalID).Scan(&teamID); err != nil {
				return err
			}
			if teamID != home && teamID != away {
				return fmt.Errorf("player team is not a game participant")
			}
			seenTeams[teamID] = true
			if err := tx.QueryRow(ctx, `INSERT INTO players(league_id,provider,external_id,name,observed_at) VALUES($1,$2,$3,$4,$5)
    ON CONFLICT(league_id,provider,external_id) DO UPDATE SET
    name=CASE WHEN players.observed_at<=EXCLUDED.observed_at THEN EXCLUDED.name ELSE players.name END,
    observed_at=GREATEST(players.observed_at,EXCLUDED.observed_at) RETURNING id`, leagueID, provider, line.ExternalID, line.Name, box.ObservedAt).Scan(&playerID); err != nil {
				return err
			}
			raw, err := json.Marshal(line.Stats)
			if err != nil {
				return err
			}
			additive, err := json.Marshal(player.AdditiveStats(line))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO player_game_stats(game_id,player_id,team_id,league_id,provider,category,position,jersey,did_not_play,starter,stats,additive_stats)
    VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, g.ID, playerID, teamID, leagueID, provider, line.Category, line.Position, line.Jersey, line.DidNotPlay, line.Starter, raw, additive); err != nil {
				return err
			}
		}
		if len(seenTeams) != 2 {
			return fmt.Errorf("player box score missing a team")
		}
		_, err := tx.Exec(ctx, `INSERT INTO player_game_imports(game_id,observed_at,line_count,season,season_type,home_team_id,away_team_id)
   VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(game_id) DO UPDATE SET observed_at=EXCLUDED.observed_at,line_count=EXCLUDED.line_count,
   season=EXCLUDED.season,season_type=EXCLUDED.season_type,home_team_id=EXCLUDED.home_team_id,away_team_id=EXCLUDED.away_team_id`, g.ID, box.ObservedAt, len(lines), season, seasonType, home, away)
		return err
	})
}
