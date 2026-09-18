package postgres_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/madman321000/sports-predictions/internal/game"
	"github.com/madman321000/sports-predictions/internal/player"
	"github.com/madman321000/sports-predictions/internal/postgres"
	"github.com/madman321000/sports-predictions/internal/seed"
	"github.com/madman321000/sports-predictions/internal/stats"
	"github.com/madman321000/sports-predictions/internal/team"
)

func TestStatsBrowseIntegration(t *testing.T) {
	pool, ctx := testPool(t)
	for _, name := range []string{"000002_create_teams", "000003_create_games", "000004_create_import_snapshots", "000005_create_players"} {
		sql, err := os.ReadFile("../../migrations/" + name + ".up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	if err := seed.SeedLeagues(ctx, postgres.NewPostgresLeagueRepository(pool)); err != nil {
		t.Fatal(err)
	}
	gr := postgres.NewPostgresGameRepository(pool)
	tr := postgres.NewPostgresTeamRepository(pool)
	pr := postgres.NewPostgresPlayerRepository(pool)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	score := 10
	for _, league := range []string{"NBA", "NFL"} {
		id, err := gr.LeagueID(ctx, league)
		if err != nil {
			t.Fatal(err)
		}
		if err = tr.UpsertTeams(ctx, id, "espn", []team.Team{{ExternalID: "1", Name: league + " Home", Abbreviation: "H"}, {ExternalID: "2", Name: league + " Away", Abbreviation: "A"}}); err != nil {
			t.Fatal(err)
		}
		for i, spec := range []struct {
			id           string
			season, kind int
		}{{"g1", 2026, 2}, {"g2", 2026, 2}, {"old", 2025, 2}, {"playoff", 2026, 3}} {
			if err = gr.UpsertGames(ctx, id, "espn", []game.Game{{ExternalID: spec.id, HomeTeamExternalID: "1", AwayTeamExternalID: "2", Season: spec.season, SeasonType: spec.kind, Status: "final", StartsAt: now.Add(time.Duration(i) * 24 * time.Hour), ObservedAt: now, HomeScore: &score, AwayScore: &score}}); err != nil {
				t.Fatal(err)
			}
		}
		games, err := pr.PlayerGames(ctx, league, 2026, 2)
		if err != nil {
			t.Fatal(err)
		}
		for i, g := range games {
			// A traded player with the same provider ID as the other league.
			teamID := "1"
			if i == 1 {
				teamID = "2"
			}
			otherTeam := "2"
			if teamID == "2" {
				otherTeam = "1"
			}
			box := player.BoxScore{ObservedAt: now, Lines: []player.Line{{ExternalID: "p1", Name: league + " Player", TeamExternalID: teamID, Category: "general", Stats: map[string]string{"points": "10"}}}}
			box.Lines = append(box.Lines, player.Line{ExternalID: "p2", Name: league + " Reserve", TeamExternalID: otherTeam, Category: "general", DidNotPlay: true, Stats: map[string]string{"points": "0"}})
			if err = pr.SavePlayerGame(ctx, g, box); err != nil {
				t.Fatal(err)
			}
		}
	}
	repo := postgres.NewStatsRepository(pool)
	f := stats.Filter{League: "NBA", Season: 2026, SeasonType: 2, Limit: 25}
	browse := func(resource string, filter stats.Filter, want int) []map[string]any {
		t.Helper()
		page, err := repo.Browse(ctx, resource, filter)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != want {
			t.Fatalf("%s total %d want %d", resource, page.Total, want)
		}
		var rows []map[string]any
		if err = json.Unmarshal(page.Items, &rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	browse("leagues", f, 2)
	browse("seasons", f, 3)
	browse("teams", f, 2)
	games := browse("games", f, 2)
	if games[0]["home_team"] != "NBA Home" || games[0]["player_stats_complete"] != true {
		t.Fatal(games)
	}
	players := browse("players", f, 2)
	if players[0]["name"] != "NBA Player" || players[0]["games"] != float64(2) {
		t.Fatal(players)
	}
	rows := browse("player-games", f, 4)
	if rows[0]["player"] != "NBA Player" {
		t.Fatal(rows)
	}
	totals := browse("totals", f, 2)
	for _, r := range totals {
		if r["total"] != float64(10) || r["games_with_metric"] != float64(1) {
			t.Fatal("trade totals merged or duplicated", totals)
		}
	}
	paged := f
	paged.Limit = 1
	paged.Offset = 1
	second := browse("games", paged, 2)
	if len(second) != 1 || second[0]["id"] == games[0]["id"] {
		t.Fatal("pagination unstable")
	}
	paged.Offset = 500
	if len(browse("games", paged, 2)) != 0 {
		t.Fatal("out of range page nonempty")
	}
	filtered := f
	filtered.Search = "' OR 1=1 --"
	browse("players", filtered, 0)
	filtered.Search = "nba player"
	browse("players", filtered, 1)
	filtered = f
	filtered.Category = "passing"
	browse("totals", filtered, 0)
	browse("player-games", filtered, 0)
	filtered = f
	filtered.SeasonType = 3
	browse("games", filtered, 1)
	browse("players", filtered, 0)
	// IDs are database IDs, and every detail filter stays within the selected scope.
	filtered = f
	filtered.PlayerID, _ = strconv.ParseInt(players[0]["id"].(string), 10, 64)
	browse("player-games", filtered, 2)
	filtered.TeamID, _ = strconv.ParseInt(totals[0]["team_id"].(string), 10, 64)
	browse("player-games", filtered, 1)
	browse("totals", filtered, 1)
	filtered = f
	filtered.GameID, _ = strconv.ParseInt(games[0]["id"].(string), 10, 64)
	browse("player-games", filtered, 2)
	filtered = f
	filtered.Status = "scheduled"
	browse("games", filtered, 0)
	w := httptest.NewRecorder()
	stats.NewHandler(repo).ServeHTTP(w, httptest.NewRequest("GET", "/api/stats/players?league=NBA&season=2026", nil))
	if w.Code != 200 {
		t.Fatalf("HTTP/database round trip failed: %d %s", w.Code, w.Body.String())
	}
	// Invalidate one checkpoint: partial box scores and their totals must disappear.
	if _, err := pool.Exec(ctx, "UPDATE player_game_imports SET line_count=line_count+1 WHERE game_id=$1", games[0]["id"]); err != nil {
		t.Fatal(err)
	}
	browse("player-games", f, 2)
	browse("totals", f, 1)
	filtered = f
	filtered.League = "NFL"
	browse("player-games", filtered, 4)
}
