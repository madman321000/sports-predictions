package postgres_test

import (
	"os"
	"testing"
	"time"

	"github.com/madman321000/sports-predictions/internal/game"
	"github.com/madman321000/sports-predictions/internal/postgres"
	"github.com/madman321000/sports-predictions/internal/seed"
	"github.com/madman321000/sports-predictions/internal/team"
)

func TestImportSnapshotsIntegration(t *testing.T) {
	pool, ctx := testPool(t)
	for _, path := range []string{"000002_create_teams.up.sql", "000003_create_games.up.sql", "000004_create_import_snapshots.up.sql"} {
		sql, err := os.ReadFile("../../migrations/" + path)
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
	games := postgres.NewPostgresGameRepository(pool)
	nba, err := teams.LeagueID(ctx, "NBA")
	if err != nil {
		t.Fatal(err)
	}
	nfl, err := teams.LeagueID(ctx, "NFL")
	if err != nil {
		t.Fatal(err)
	}
	roster := []team.Team{{ExternalID: "1", Name: "Home", Abbreviation: "H"}, {ExternalID: "2", Name: "Away", Abbreviation: "A"}}
	checkTeams := func(id int64, provider string, want bool) {
		t.Helper()
		got, err := teams.TeamsImported(ctx, id, provider)
		if err != nil || got != want {
			t.Fatalf("teams cached=%v want=%v error=%v", got, want, err)
		}
	}
	if err := teams.UpsertTeams(ctx, nba, "espn", roster); err != nil {
		t.Fatal(err)
	}
	checkTeams(nba, "espn", false) // Existing rows alone cannot prove a complete import.
	if err := teams.SaveTeamImport(ctx, nba, "espn", roster); err != nil {
		t.Fatal(err)
	}
	checkTeams(nba, "espn", true)
	checkTeams(nfl, "espn", false)
	checkTeams(nba, "other", false)
	if _, err := pool.Exec(ctx, "DELETE FROM teams WHERE league_id=$1 AND external_id='2'", nba); err != nil {
		t.Fatal(err)
	}
	checkTeams(nba, "espn", false)
	if err := teams.SaveTeamImport(ctx, nba, "espn", roster); err != nil {
		t.Fatal(err)
	}
	badRoster := []team.Team{roster[0], {}}
	if err := teams.SaveTeamImport(ctx, nfl, "espn", badRoster); err == nil {
		t.Fatal("invalid roster should fail")
	}
	checkTeams(nfl, "espn", false)
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM teams WHERE league_id=$1", nfl).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed roster partially committed")
	}

	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	zero, score := 0, 23
	g := game.Game{ExternalID: "g1", HomeTeamExternalID: "1", AwayTeamExternalID: "2", StartsAt: date.Add(26 * time.Hour), Status: "final", HomeScore: &zero, AwayScore: &score, Season: 2026, SeasonType: 2, ObservedAt: date.Add(48 * time.Hour)}
	checkDate := func(id int64, provider string, d time.Time, want bool) {
		t.Helper()
		got, err := games.GameDateComplete(ctx, id, provider, d)
		if err != nil || got != want {
			t.Fatalf("date cached=%v want=%v error=%v", got, want, err)
		}
	}
	if err := games.UpsertGames(ctx, nba, "espn", []game.Game{g}); err != nil {
		t.Fatal(err)
	}
	checkDate(nba, "espn", date, false)
	if err := games.SaveGameImport(ctx, nba, "espn", date, []game.Game{g}); err != nil {
		t.Fatal(err)
	}
	checkDate(nba, "espn", date, true)                   // A valid score of zero is complete.
	checkDate(nba, "espn", date.AddDate(0, 0, 1), false) // Use query date, not UTC starts_at.
	checkDate(nfl, "espn", date, false)
	checkDate(nba, "other", date, false)
	scheduled := g
	scheduled.ExternalID = "g2"
	scheduled.Status = "scheduled"
	scheduled.HomeScore = nil
	scheduled.AwayScore = nil
	scheduled.ObservedAt = g.ObservedAt.Add(time.Hour)
	if err := games.SaveGameImport(ctx, nba, "espn", date, []game.Game{g, scheduled}); err != nil {
		t.Fatal(err)
	}
	checkDate(nba, "espn", date, false)
	// An older complete response must not erase the newer manifest's second game.
	if err := games.SaveGameImport(ctx, nba, "espn", date, []game.Game{g}); err != nil {
		t.Fatal(err)
	}
	checkDate(nba, "espn", date, false)
	scheduled.Status = "in_progress"
	scheduled.HomeScore = &zero
	scheduled.AwayScore = &score
	scheduled.ObservedAt = scheduled.ObservedAt.Add(time.Hour)
	if err := games.UpsertGames(ctx, nba, "espn", []game.Game{scheduled}); err != nil {
		t.Fatal(err)
	}
	checkDate(nba, "espn", date, false) // Live scores do not mean finished games.
	scheduled.Status = "final"
	scheduled.ObservedAt = scheduled.ObservedAt.Add(time.Hour)
	if err := games.UpsertGames(ctx, nba, "espn", []game.Game{scheduled}); err != nil {
		t.Fatal(err)
	}
	checkDate(nba, "espn", date, true)
	if _, err := pool.Exec(ctx, "DELETE FROM games WHERE external_id='g2'"); err != nil {
		t.Fatal(err)
	}
	checkDate(nba, "espn", date, false)
	emptyDate := date.AddDate(0, 0, 2)
	if err := games.SaveGameImport(ctx, nba, "espn", emptyDate, nil); err != nil {
		t.Fatal(err)
	}
	checkDate(nba, "espn", emptyDate, false)
	failedDate := date.AddDate(0, 0, 3)
	bad := g
	bad.ExternalID = "z-bad"
	bad.HomeTeamExternalID = "missing"
	if err := games.SaveGameImport(ctx, nba, "espn", failedDate, []game.Game{g, bad}); err == nil {
		t.Fatal("expected missing team failure")
	}
	checkDate(nba, "espn", failedDate, false)
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM game_date_imports WHERE import_date=$1", failedDate.Format(time.DateOnly)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed game batch committed a completion record")
	}
	down, err := os.ReadFile("../../migrations/000004_create_import_snapshots.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
}
