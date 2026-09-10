package postgres_test

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/madman321000/sports-predictions/internal/game"
	"github.com/madman321000/sports-predictions/internal/postgres"
	"github.com/madman321000/sports-predictions/internal/seed"
	"github.com/madman321000/sports-predictions/internal/team"
)

func TestGameRepositoryIntegration(t *testing.T) {
	pool, ctx := testPool(t)
	for _, path := range []string{"../../migrations/000002_create_teams.up.sql", "../../migrations/000003_create_games.up.sql"} {
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	if err := seed.SeedLeagues(ctx, postgres.NewPostgresLeagueRepository(pool)); err != nil {
		t.Fatal(err)
	}
	teams := postgres.NewPostgresTeamRepository(pool)
	repo := postgres.NewPostgresGameRepository(pool)
	nba, err := repo.LeagueID(ctx, "NBA")
	if err != nil {
		t.Fatal(err)
	}
	nfl, err := repo.LeagueID(ctx, "NFL")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{nba, nfl} {
		if err := teams.UpsertTeams(ctx, id, "espn", []team.Team{{ExternalID: "1", Name: "Home", Abbreviation: "H"}, {ExternalID: "2", Name: "Away", Abbreviation: "A"}}); err != nil {
			t.Fatal(err)
		}
	}
	observed := time.Now().UTC().Truncate(time.Microsecond)
	g := game.Game{ExternalID: "game1", HomeTeamExternalID: "1", AwayTeamExternalID: "2", StartsAt: observed.Add(time.Hour), Status: "scheduled", Season: 2026, SeasonType: 2, ObservedAt: observed}
	for i := 0; i < 2; i++ {
		if err := repo.UpsertGames(ctx, nba, "espn", []game.Game{g}); err != nil {
			t.Fatal(err)
		}
	}
	var beforeID int64
	var beforeCreated time.Time
	var score *int
	var count int
	if err := pool.QueryRow(ctx, "SELECT id,created_at,home_score FROM games").Scan(&beforeID, &beforeCreated, &score); err != nil {
		t.Fatal(err)
	}
	if score != nil {
		t.Fatal("scheduled score must be null")
	}
	home, away := 101, 98
	g.Status = "final"
	g.HomeScore = &home
	g.AwayScore = &away
	g.ObservedAt = observed.Add(time.Second)
	g.StartsAt = g.StartsAt.Add(time.Hour)
	if err := repo.UpsertGames(ctx, nba, "espn", []game.Game{g}); err != nil {
		t.Fatal(err)
	}
	var afterID int64
	var created, start time.Time
	var status string
	if err := pool.QueryRow(ctx, "SELECT id,created_at,starts_at,status,home_score FROM games").Scan(&afterID, &created, &start, &status, &score); err != nil {
		t.Fatal(err)
	}
	if afterID != beforeID || !created.Equal(beforeCreated) || !start.Equal(g.StartsAt) || status != "final" || score == nil || *score != 101 {
		t.Fatal("game update changed identity or lost fields")
	}
	// A later transaction containing an older fetched response cannot revert a result.
	older := g
	older.ObservedAt = observed
	older.Status = "scheduled"
	older.HomeScore = nil
	older.AwayScore = nil
	if err := repo.UpsertGames(ctx, nba, "espn", []game.Game{older}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT status FROM games").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "final" {
		t.Fatal("stale snapshot overwrote final result")
	}
	// A failed second record rolls back the valid first record's update.
	changed := g
	changed.Status = "postponed"
	changed.ObservedAt = observed.Add(2 * time.Second)
	missing := changed
	missing.ExternalID = "z-missing"
	missing.HomeTeamExternalID = "unknown"
	if err := repo.UpsertGames(ctx, nba, "espn", []game.Game{changed, missing}); err == nil {
		t.Fatal("missing team should fail")
	}
	if err := pool.QueryRow(ctx, "SELECT status FROM games").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "final" {
		t.Fatal("failed batch partially committed")
	}
	// Overlapping concurrent batches converge to the newest fetched snapshot.
	g2 := g
	g2.ExternalID = "game2"
	newHome := 110
	newest := g
	newest.HomeScore = &newHome
	newest.ObservedAt = observed.Add(10 * time.Second)
	newest2 := newest
	newest2.ExternalID = "game2"
	var wg sync.WaitGroup
	for _, batch := range [][]game.Game{{g2, g}, {newest, newest2}} {
		wg.Go(func() {
			if err := repo.UpsertGames(ctx, nba, "espn", batch); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM games WHERE home_score=110").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("concurrent snapshots failed: newest rows=%d", count)
	}
	week := 1
	g.Week = &week
	g.Season = 2025
	if err := repo.UpsertGames(ctx, nfl, "espn", []game.Game{g}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM games").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatal("league-scoped game IDs collided")
	}
	var savedWeek int
	if err := pool.QueryRow(ctx, "SELECT week FROM games WHERE league_id=$1", nfl).Scan(&savedWeek); err != nil {
		t.Fatal(err)
	}
	if savedWeek != 1 {
		t.Fatal("NFL week not saved")
	}
	// Down migration must undo the schema additions without removing teams.
	down, err := os.ReadFile("../../migrations/000003_create_games.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM teams").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatal("down migration changed teams")
	}
}
