package postgres_test

import (
	"os"
	"testing"
	"time"

	"github.com/madman321000/sports-predictions/internal/game"
	"github.com/madman321000/sports-predictions/internal/player"
	"github.com/madman321000/sports-predictions/internal/postgres"
	"github.com/madman321000/sports-predictions/internal/seed"
	"github.com/madman321000/sports-predictions/internal/team"
)

func TestPlayerRepositoryIntegration(t *testing.T) {
	pool, ctx := testPool(t)
	for _, name := range []string{"000002_create_teams", "000003_create_games", "000004_create_import_snapshots", "000005_create_players"} {
		body, err := os.ReadFile("../../migrations/" + name + ".up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := seed.SeedLeagues(ctx, postgres.NewPostgresLeagueRepository(pool)); err != nil {
		t.Fatal(err)
	}
	gr := postgres.NewPostgresGameRepository(pool)
	tr := postgres.NewPostgresTeamRepository(pool)
	pr := postgres.NewPostgresPlayerRepository(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	zero := 0
	for _, league := range []string{"NBA", "NFL"} {
		id, err := gr.LeagueID(ctx, league)
		if err != nil {
			t.Fatal(err)
		}
		if err := tr.UpsertTeams(ctx, id, "espn", []team.Team{{ExternalID: "1", Name: "Home", Abbreviation: "H"}, {ExternalID: "2", Name: "Away", Abbreviation: "A"}, {ExternalID: "3", Name: "Third", Abbreviation: "T"}}); err != nil {
			t.Fatal(err)
		}
		for _, spec := range []struct {
			id           string
			season, kind int
			status       string
		}{{"g1", 2026, 2, "final"}, {"g2", 2026, 2, "final"}, {"playoff", 2026, 3, "final"}, {"old", 2025, 2, "final"}, {"future", 2026, 2, "scheduled"}} {
			if err := gr.UpsertGames(ctx, id, "espn", []game.Game{{ExternalID: spec.id, HomeTeamExternalID: "1", AwayTeamExternalID: "2", Season: spec.season, SeasonType: spec.kind, Status: spec.status, StartsAt: now, ObservedAt: now, HomeScore: &zero, AwayScore: &zero}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	games, err := pr.PlayerGames(ctx, "NBA", 2026, 2)
	if err != nil || len(games) != 2 {
		t.Fatalf("games %v %v", games, err)
	}
	g := games[0]
	box := player.BoxScore{ObservedAt: now, Lines: []player.Line{
		{ExternalID: "p1", Name: "One", TeamExternalID: "1", Category: "general", Stats: map[string]string{"points": "10", "fieldGoalsMade-fieldGoalsAttempted": "4-8"}},
		{ExternalID: "p2", Name: "Two", TeamExternalID: "2", Category: "general", Stats: map[string]string{"points": "0"}},
	}}
	checkComplete := func(want bool) {
		t.Helper()
		got, err := pr.PlayerGameComplete(ctx, g)
		if err != nil || got != want {
			t.Fatalf("complete %v want %v: %v", got, want, err)
		}
	}
	checkComplete(false)
	for i := 0; i < 2; i++ {
		if err := pr.SavePlayerGame(ctx, g, box); err != nil {
			t.Fatal(err)
		}
	}
	checkComplete(true)
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM player_game_stats").Scan(&count); err != nil || count != 2 {
		t.Fatalf("duplicates: %d %v", count, err)
	}
	var total float64
	checkTotal := func(want float64) {
		t.Helper()
		if err := pool.QueryRow(ctx, "SELECT SUM(total) FROM player_season_totals WHERE metric='points'").Scan(&total); err != nil || total != want {
			t.Fatalf("total %v want %v: %v", total, want, err)
		}
	}
	checkTotal(10)
	// Failed force refresh must retain old stats and completion marker.
	bad := player.BoxScore{ObservedAt: now.Add(time.Second), Lines: append([]player.Line{}, box.Lines...)}
	bad.Lines[1].TeamExternalID = "3"
	if err := pr.SavePlayerGame(ctx, g, bad); err == nil {
		t.Fatal("accepted unrelated team")
	}
	checkComplete(true)
	checkTotal(10)
	// Older responses cannot replace a newer snapshot.
	old := box
	old.ObservedAt = now.Add(-time.Second)
	old.Lines = append([]player.Line{}, box.Lines...)
	old.Lines[0].Stats = map[string]string{"points": "99"}
	if err := pr.SavePlayerGame(ctx, g, old); err != nil {
		t.Fatal(err)
	}
	checkTotal(10)
	// Trade: the same player belongs to different teams in different games.
	traded := player.BoxScore{ObservedAt: now.Add(time.Second), Lines: append([]player.Line{}, box.Lines...)}
	traded.Lines[0].TeamExternalID = "2"
	traded.Lines[1].TeamExternalID = "1"
	if err := pr.SavePlayerGame(ctx, games[1], traded); err != nil {
		t.Fatal(err)
	}
	checkTotal(20)
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM player_season_teams WHERE player_id=(SELECT id FROM players WHERE external_id='p1')").Scan(&count); err != nil || count != 2 {
		t.Fatalf("trade membership %d %v", count, err)
	}
	// A deleted stat invalidates the checkpoint and rerun repairs it.
	if _, err := pool.Exec(ctx, "DELETE FROM player_game_stats WHERE game_id=$1 AND player_id=(SELECT id FROM players WHERE external_id='p1')", g.ID); err != nil {
		t.Fatal(err)
	}
	checkComplete(false)
	if err := pr.SavePlayerGame(ctx, g, box); err != nil {
		t.Fatal(err)
	}
	checkComplete(true)
	// Separate league uses its own identity even with identical ESPN IDs.
	nfl, err := pr.PlayerGames(ctx, "NFL", 2026, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := pr.SavePlayerGame(ctx, nfl[0], box); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM players WHERE external_id='p1'").Scan(&count); err != nil || count != 2 {
		t.Fatalf("league scope %d %v", count, err)
	}
	// Down migration leaves existing game data intact.
	down, err := os.ReadFile("../../migrations/000005_create_players.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM games").Scan(&count); err != nil || count != 10 {
		t.Fatalf("games after rollback %d %v", count, err)
	}
}
