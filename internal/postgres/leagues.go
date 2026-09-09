package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/madman321000/sports-predictions/internal/league"
)

type PostgresLeagueRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresLeagueRepository(pool *pgxpool.Pool) *PostgresLeagueRepository {
	return &PostgresLeagueRepository{pool: pool}
}

func (r *PostgresLeagueRepository) Create(ctx context.Context, league *league.League) error {
	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO leagues (name, abbreviation, sport)
	 VALUES ($1, $2, $3)
	 ON CONFLICT (abbreviation)
	 DO UPDATE SET
	     name = EXCLUDED.name,
	     sport = EXCLUDED.sport,
	     updated_at = NOW()`,
		league.Name,
		league.Abbreviation,
		league.Sport,
	)
	if err != nil {
		return fmt.Errorf("failed to create or update league: %w", err)
	}
	return nil
}
