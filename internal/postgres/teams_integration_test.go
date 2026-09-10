package postgres_test

import (
	"os"
	"testing"

	"github.com/madman321000/sports-predictions/internal/postgres"
	"github.com/madman321000/sports-predictions/internal/seed"
	"github.com/madman321000/sports-predictions/internal/team"
)

func TestTeamRepositoryIntegration(t *testing.T) {
	pool, ctx := testPool(t)
	migration, err := os.ReadFile("../../migrations/000002_create_teams.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	if err := seed.SeedLeagues(ctx, postgres.NewPostgresLeagueRepository(pool)); err != nil {
		t.Fatal(err)
	}
	repo := postgres.NewTeamRepository(pool)
	id, err := repo.LeagueID(ctx, "NBA")
	if err != nil {
		t.Fatal(err)
	}
	values := []team.Team{{ExternalID: "1", Name: "Atlanta Hawks", Abbreviation: "ATL"}}
	for i := 0; i < 2; i++ {
		if err := repo.UpsertTeams(ctx, id, "espn", values); err != nil {
			t.Fatal(err)
		}
	}
	var beforeID int64
	if err := pool.QueryRow(ctx, "SELECT id FROM teams").Scan(&beforeID); err != nil {
		t.Fatal(err)
	}
	values[0].Name = "Updated Hawks"
	if err := repo.UpsertTeams(ctx, id, "espn", values); err != nil {
		t.Fatal(err)
	}
	var afterID int64
	var name string
	var count int
	if err := pool.QueryRow(ctx, "SELECT id,name FROM teams").Scan(&afterID, &name); err != nil {
		t.Fatal(err)
	}
	if afterID != beforeID || name != "Updated Hawks" {
		t.Fatal("update failed to preserve identity or persist name")
	}
	// Invalid second record must roll back the first record in the same batch.
	values[0].Name = "Should Roll Back"
	values = append(values, team.Team{})
	if err := repo.UpsertTeams(ctx, id, "espn", values); err == nil {
		t.Fatal("expected validation error")
	}
	if err := pool.QueryRow(ctx, "SELECT name FROM teams").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Updated Hawks" {
		t.Fatal("partial batch committed")
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM teams").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("row count=%d", count)
	}
	// Provider IDs may overlap across leagues and providers.
	nfl, err := repo.LeagueID(ctx, "NFL")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		id       int64
		provider string
	}{{nfl, "espn"}, {id, "other"}} {
		if err := repo.UpsertTeams(ctx, v.id, v.provider, values[:1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM teams").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("scoped identities collided: count=%d", count)
	}
}
