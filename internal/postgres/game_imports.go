package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/madman321000/sports-predictions/internal/game"
)

// GameDateComplete requires a full imported response and final scores for all
// its games. Partial manual inserts and an empty scoreboard are not cache hits.
func (r *PostgresGameRepository) GameDateComplete(ctx context.Context, leagueID int64, provider string, date time.Time) (bool, error) {
	var complete bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1 FROM game_date_imports snapshot
  WHERE league_id=$1 AND provider=$2 AND import_date=$3 AND cardinality(external_ids)>0
  AND NOT EXISTS (
   SELECT 1 FROM unnest(snapshot.external_ids) AS expected(external_id)
   LEFT JOIN games g ON g.external_id=expected.external_id AND g.league_id=$1 AND g.provider=$2
   WHERE g.id IS NULL OR g.status<>'final' OR g.home_score IS NULL OR g.away_score IS NULL
  )
 )`, leagueID, provider, date.Format(time.DateOnly)).Scan(&complete)
	return complete, err
}

func (r *PostgresGameRepository) SaveGameImport(ctx context.Context, leagueID int64, provider string, date time.Time, games []game.Game) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := upsertGames(ctx, tx, leagueID, provider, games); err != nil {
			return err
		}
		ids := make([]string, 0, len(games))
		var observed time.Time
		for _, g := range games {
			ids = append(ids, g.ExternalID)
			if g.ObservedAt.After(observed) {
				observed = g.ObservedAt
			}
		}
		// Empty responses never skip later requests, but still record a successful fetch.
		if observed.IsZero() {
			observed = time.Now().UTC()
		}
		_, err := tx.Exec(ctx, `INSERT INTO game_date_imports (league_id,provider,import_date,external_ids,observed_at)
   VALUES ($1,$2,$3,$4,$5)
   ON CONFLICT (league_id,provider,import_date) DO UPDATE
   SET external_ids=EXCLUDED.external_ids,observed_at=EXCLUDED.observed_at
   WHERE game_date_imports.observed_at <= EXCLUDED.observed_at`, leagueID, provider, date.Format(time.DateOnly), ids, observed)
		return err
	})
}
