package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/madman321000/sports-predictions/internal/team"
)

// TeamsImported checks that every team in a full successful response still exists.
func (r *PostgresTeamRepository) TeamsImported(ctx context.Context, leagueID int64, provider string) (bool, error) {
	var complete bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1 FROM team_imports snapshot WHERE league_id=$1 AND provider=$2
  AND NOT EXISTS (
   SELECT 1 FROM unnest(snapshot.external_ids) AS expected(external_id)
   LEFT JOIN teams t ON t.external_id=expected.external_id AND t.league_id=$1 AND t.provider=$2
   WHERE t.id IS NULL
  )
 )`, leagueID, provider).Scan(&complete)
	return complete, err
}

// SaveTeamImport persists the response and its completion record atomically.
func (r *PostgresTeamRepository) SaveTeamImport(ctx context.Context, leagueID int64, provider string, teams []team.Team) error {
	if len(teams) == 0 {
		return fmt.Errorf("cannot mark an empty team import complete")
	}
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := upsertTeams(ctx, tx, leagueID, provider, teams); err != nil {
			return err
		}
		ids := make([]string, 0, len(teams))
		for _, t := range teams {
			ids = append(ids, t.ExternalID)
		}
		_, err := tx.Exec(ctx, `INSERT INTO team_imports (league_id,provider,external_ids) VALUES ($1,$2,$3)
   ON CONFLICT (league_id,provider) DO UPDATE SET external_ids=EXCLUDED.external_ids,imported_at=NOW()`, leagueID, provider, ids)
		return err
	})
}
