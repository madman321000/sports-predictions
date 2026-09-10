package postgres

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/madman321000/sports-predictions/internal/game"
)

type PostgresGameRepository struct{ pool *pgxpool.Pool }

func NewPostgresGameRepository(pool *pgxpool.Pool) *PostgresGameRepository {
	return &PostgresGameRepository{pool: pool}
}

func (r *PostgresGameRepository) LeagueID(ctx context.Context, abbreviation string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, "SELECT id FROM leagues WHERE abbreviation=$1", abbreviation).Scan(&id)
	return id, err
}

// UpsertGames commits one date as a unit. Ordering row locks avoids deadlocks
// when concurrent date batches contain the same rescheduled games.
func (r *PostgresGameRepository) UpsertGames(ctx context.Context, leagueID int64, provider string, games []game.Game) error {
	ordered := slices.Clone(games)
	slices.SortFunc(ordered, func(a, b game.Game) int { return strings.Compare(a.ExternalID, b.ExternalID) })
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		for _, g := range ordered {
			if g.ExternalID == "" || g.StartsAt.IsZero() || g.ObservedAt.IsZero() {
				return fmt.Errorf("game missing identity or timestamps")
			}
			var home, away int64
			for _, t := range []struct {
				externalID string
				id         *int64
			}{{g.HomeTeamExternalID, &home}, {g.AwayTeamExternalID, &away}} {
				if err := tx.QueryRow(ctx, "SELECT id FROM teams WHERE league_id=$1 AND provider=$2 AND external_id=$3", leagueID, provider, t.externalID).Scan(t.id); err != nil {
					return fmt.Errorf("game %s team %s unavailable (import teams first): %w", g.ExternalID, t.externalID, err)
				}
			}
			_, err := tx.Exec(ctx, `INSERT INTO games
    (league_id,provider,external_id,home_team_id,away_team_id,starts_at,status,home_score,away_score,season,season_type,week,observed_at)
    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
    ON CONFLICT (provider,league_id,external_id) DO UPDATE SET
    home_team_id=EXCLUDED.home_team_id,away_team_id=EXCLUDED.away_team_id,
    starts_at=EXCLUDED.starts_at,status=EXCLUDED.status,home_score=EXCLUDED.home_score,away_score=EXCLUDED.away_score,
    season=EXCLUDED.season,season_type=EXCLUDED.season_type,week=EXCLUDED.week,observed_at=EXCLUDED.observed_at,updated_at=NOW()
    WHERE games.observed_at <= EXCLUDED.observed_at`, leagueID, provider, g.ExternalID, home, away, g.StartsAt, g.Status, g.HomeScore, g.AwayScore, g.Season, g.SeasonType, g.Week, g.ObservedAt)
			if err != nil {
				return fmt.Errorf("upsert game %s: %w", g.ExternalID, err)
			}
		}
		return nil
	})
}
