package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/madman321000/sports-predictions/internal/team"
)

type PostgresTeamRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresTeamRepository(pool *pgxpool.Pool) *PostgresTeamRepository {
	return &PostgresTeamRepository{pool: pool}
}

func (r *PostgresTeamRepository) LeagueID(ctx context.Context, abbreviation string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, "SELECT id FROM leagues WHERE abbreviation = $1", abbreviation).Scan(&id)
	return id, err
}

func (r *PostgresTeamRepository) UpsertTeams(ctx context.Context, leagueID int64, provider string, teams []team.Team) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		return upsertTeams(ctx, tx, leagueID, provider, teams)
	})
}

func upsertTeams(ctx context.Context, tx pgx.Tx, leagueID int64, provider string, teams []team.Team) error {
	for _, t := range teams {
		if t.ExternalID == "" || t.Name == "" || t.Abbreviation == "" || provider == "" {
			return fmt.Errorf("team is missing required fields")
		}
		_, err := tx.Exec(ctx, `INSERT INTO teams (league_id, provider, external_id, name, abbreviation)
    VALUES ($1, $2, $3, $4, $5)
    ON CONFLICT (provider, league_id, external_id) DO UPDATE
    SET name = EXCLUDED.name, abbreviation = EXCLUDED.abbreviation, updated_at = NOW()`,
			leagueID, provider, t.ExternalID, t.Name, t.Abbreviation)
		if err != nil {
			return fmt.Errorf("upsert team %q: %w", t.ExternalID, err)
		}
	}
	return nil
}
