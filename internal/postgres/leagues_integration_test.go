package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/madman321000/sports-predictions/internal/config"
	"github.com/madman321000/sports-predictions/internal/league"
	"github.com/madman321000/sports-predictions/internal/postgres"
	"github.com/madman321000/sports-predictions/internal/seed"
)

// Each test owns a schema. Never migrate or delete tables in the caller's schema.
func testPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	url, err := config.LoadTestDatabaseURL(filepath.Join("..", "..", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	schema := "league_test_" + hex.EncodeToString(suffix[:])
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("clean up test schema: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000001_create_leagues.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply league migration: %v", err)
	}
	return pool, ctx
}

func TestLeagueRepositoryUpsertIntegration(t *testing.T) {
	pool, ctx := testPool(t)
	repo := postgres.NewPostgresLeagueRepository(pool)
	for i := 0; i < 2; i++ {
		if err := seed.SeedLeagues(ctx, repo); err != nil {
			t.Fatalf("seed run %d: %v", i+1, err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM leagues").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("row count = %d, want 2", count)
	}
	var beforeID int64
	var beforeCreated time.Time
	var name, sport string
	if err := pool.QueryRow(ctx, "SELECT id, name, sport, created_at FROM leagues WHERE abbreviation = 'NBA'").Scan(&beforeID, &name, &sport, &beforeCreated); err != nil {
		t.Fatal(err)
	}
	if name != "National Basketball Association" || sport != "Basketball" {
		t.Fatalf("unexpected seed values: %q, %q", name, sport)
	}
	// Use a fixed old timestamp to test timestamp refresh without sleeping.
	if _, err := pool.Exec(ctx, "UPDATE leagues SET updated_at = '2000-01-01 UTC' WHERE abbreviation = 'NBA'"); err != nil {
		t.Fatal(err)
	}
	value := &league.League{Name: "Updated NBA", Abbreviation: "NBA", Sport: "Updated Basketball"}
	if err := repo.Create(ctx, value); err != nil {
		t.Fatal(err)
	}
	var afterID int64
	var created, updated time.Time
	if err := pool.QueryRow(ctx, "SELECT id, name, sport, created_at, updated_at FROM leagues WHERE abbreviation = 'NBA'").Scan(&afterID, &name, &sport, &created, &updated); err != nil {
		t.Fatal(err)
	}
	if afterID != beforeID || !created.Equal(beforeCreated) {
		t.Fatal("upsert changed existing row identity or creation time")
	}
	if name != value.Name || sport != value.Sport {
		t.Fatalf("update not persisted: %q, %q", name, sport)
	}
	if !updated.After(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("updated_at was not refreshed")
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM leagues").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("row count after update = %d, want 2", count)
	}
}

func TestLeagueRepositoryWrapsDatabaseErrorIntegration(t *testing.T) {
	pool, ctx := testPool(t)
	// Force a real database constraint failure without changing production SQL.
	if _, err := pool.Exec(ctx, "ALTER TABLE leagues ADD CONSTRAINT reject_test CHECK (abbreviation <> 'TEST')"); err != nil {
		t.Fatal(err)
	}
	repo := postgres.NewPostgresLeagueRepository(pool)
	err := repo.Create(ctx, &league.League{Name: "Test", Abbreviation: "TEST", Sport: "Test"})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("error = %v, want wrapped PostgreSQL error", err)
	}
	if pgErr.Code != "23514" {
		t.Fatalf("SQLSTATE = %s, want 23514", pgErr.Code)
	}
}
