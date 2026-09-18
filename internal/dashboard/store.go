package dashboard

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Read loads published reports from disk or Postgres at startup.
func Read(ctx context.Context, directory, databaseURL string) (map[string]report, error) {
	if databaseURL == "" {
		return Load(directory)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	defer pool.Close()
	rows, err := pool.Query(ctx, "SELECT id,body FROM dashboard_reports ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reports := map[string]report{}
	for rows.Next() {
		var id string
		var body []byte
		if err := rows.Scan(&id, &body); err != nil {
			return nil, err
		}
		r, err := decodeReport(id, body)
		if err != nil {
			return nil, err
		}
		reports[id] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return reports, nil
}

// Publish validates a directory and upserts only the public report fields atomically.
func Publish(ctx context.Context, directory, databaseURL string) error {
	reports, err := Load(directory)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for id, r := range reports {
		body, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO dashboard_reports(id,body) VALUES($1,$2) ON CONFLICT(id) DO UPDATE SET body=EXCLUDED.body,updated_at=now()", id, body); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
