package dashboard

import (
	"context"
	"crypto/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/madman321000/sports-predictions/internal/config"
)

func TestPublicationRoundTrip(t *testing.T) {
	databaseURL, err := config.LoadTestDatabaseURL(filepath.Join("..", "..", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "dashboard_test_" + strings.ToLower(rand.Text())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	// The integration URL is a URI, as documented in the test configuration.
	u, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	migration, err := os.ReadFile("../../migrations/000006_create_dashboard_reports.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "nba.json")
	if err = os.WriteFile(path, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"NBA test", "NBA updated"} {
		if err = os.WriteFile(path, []byte(strings.ReplaceAll(fixture, "NBA test", title)), 0600); err != nil {
			t.Fatal(err)
		}
		if err = Publish(ctx, dir, u.String()); err != nil {
			t.Fatal(err)
		}
		reports, err := Read(ctx, "unused", u.String())
		if err != nil {
			t.Fatal(err)
		}
		if len(reports) != 1 || reports["nba"].Title != title {
			t.Fatalf("unexpected reports: %v", reports)
		}
	}
	var body string
	if err = pool.QueryRow(ctx, "SELECT body::text FROM dashboard_reports WHERE id='nba'").Scan(&body); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "secret-path") {
		t.Fatal("private metadata persisted")
	}
	if err = os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err = Publish(ctx, dir, u.String()); err == nil {
		t.Fatal("invalid publication accepted")
	}
	reports, err := Read(ctx, "unused", u.String())
	if err != nil || len(reports) != 1 || reports["nba"].Title != "NBA updated" {
		t.Fatal("failed publication altered stored reports")
	}
	if _, err = pool.Exec(ctx, "UPDATE dashboard_reports SET body='{}'::jsonb"); err != nil {
		t.Fatal(err)
	}
	if _, err = Read(ctx, "unused", u.String()); err == nil {
		t.Fatal("invalid stored report accepted")
	}
}
