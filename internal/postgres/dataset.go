package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/madman321000/sports-predictions/internal/dataset"
)

type PostgresDatasetRepository struct{ pool *pgxpool.Pool }

func NewPostgresDatasetRepository(pool *pgxpool.Pool) *PostgresDatasetRepository {
	return &PostgresDatasetRepository{pool: pool}
}

// Read uses one read-only repeatable snapshot so concurrently running imports
// cannot cause the quality report and CSVs to describe different database states.
func (r *PostgresDatasetRepository) Read(ctx context.Context, s dataset.Scope) (dataset.Snapshot, error) {
	var data dataset.Snapshot
	if err := s.Validate(); err != nil {
		return data, err
	}
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT g.external_id,g.starts_at,h.external_id,a.external_id,g.status,g.home_score,g.away_score,c.id IS NOT NULL
 FROM games g JOIN leagues l ON l.id=g.league_id JOIN teams h ON h.id=g.home_team_id JOIN teams a ON a.id=g.away_team_id
 LEFT JOIN player_complete_games c ON c.id=g.id
 WHERE l.abbreviation=$1 AND g.provider='espn' AND g.season=$2 AND g.season_type=$3 ORDER BY g.starts_at,g.external_id`, s.League, s.Season, s.SeasonType)
		if err != nil {
			return err
		}
		for rows.Next() {
			var g dataset.Game
			if err := rows.Scan(&g.ID, &g.StartsAt, &g.Home, &g.Away, &g.Status, &g.HomeScore, &g.AwayScore, &g.PlayersComplete); err != nil {
				rows.Close()
				return err
			}
			data.Games = append(data.Games, g)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `SELECT g.external_id,p.external_id,p.name,t.external_id,s.category,s.position,s.jersey,s.did_not_play,s.starter,s.stats::text
 FROM player_game_stats s JOIN player_complete_games g ON g.id=s.game_id JOIN leagues l ON l.id=g.league_id
 JOIN players p ON p.id=s.player_id JOIN teams t ON t.id=s.team_id
 WHERE l.abbreviation=$1 AND g.provider='espn' AND g.season=$2 AND g.season_type=$3
 ORDER BY g.starts_at,g.external_id,p.external_id,s.category`, s.League, s.Season, s.SeasonType)
		if err != nil {
			return err
		}
		for rows.Next() {
			var p dataset.Player
			if err := rows.Scan(&p.GameID, &p.PlayerID, &p.Name, &p.Team, &p.Category, &p.Position, &p.Jersey, &p.DidNotPlay, &p.Starter, &p.Stats); err != nil {
				rows.Close()
				return err
			}
			data.Players = append(data.Players, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `SELECT i.import_date::text FROM game_date_imports i JOIN leagues l ON l.id=i.league_id WHERE l.abbreviation=$1 AND i.provider='espn' ORDER BY i.import_date`, s.League)
		if err != nil {
			return err
		}
		for rows.Next() {
			var date string
			if err := rows.Scan(&date); err != nil {
				rows.Close()
				return err
			}
			data.ImportDates = append(data.ImportDates, date)
		}
		rows.Close()
		return rows.Err()
	})
	return data, err
}
