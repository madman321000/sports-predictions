package postgres_test

import (
	"github.com/madman321000/sports-predictions/internal/dataset"
	"github.com/madman321000/sports-predictions/internal/game"
	"github.com/madman321000/sports-predictions/internal/player"
	"github.com/madman321000/sports-predictions/internal/postgres"
	"github.com/madman321000/sports-predictions/internal/seed"
	"github.com/madman321000/sports-predictions/internal/team"
	"os"
	"testing"
	"time"
)

func TestDatasetReadIntegration(t *testing.T) {
	pool, ctx := testPool(t)
	for _, name := range []string{"000002_create_teams", "000003_create_games", "000004_create_import_snapshots", "000005_create_players"} {
		b, e := os.ReadFile("../../migrations/" + name + ".up.sql")
		if e != nil {
			t.Fatal(e)
		}
		if _, e := pool.Exec(ctx, string(b)); e != nil {
			t.Fatal(e)
		}
	}
	if e := seed.SeedLeagues(ctx, postgres.NewPostgresLeagueRepository(pool)); e != nil {
		t.Fatal(e)
	}
	gr := postgres.NewPostgresGameRepository(pool)
	tr := postgres.NewPostgresTeamRepository(pool)
	pr := postgres.NewPostgresPlayerRepository(pool)
	dr := postgres.NewPostgresDatasetRepository(pool)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	zero := 0
	for _, league := range []string{"NBA", "NFL"} {
		id, e := gr.LeagueID(ctx, league)
		if e != nil {
			t.Fatal(e)
		}
		if e := tr.UpsertTeams(ctx, id, "espn", []team.Team{{ExternalID: "1", Name: "Home", Abbreviation: "H"}, {ExternalID: "2", Name: "Away", Abbreviation: "A"}}); e != nil {
			t.Fatal(e)
		}
		for _, kind := range []int{2, 3} {
			key := "regular"
			if kind == 3 {
				key = "playoff"
			}
			g := game.Game{ExternalID: key, HomeTeamExternalID: "1", AwayTeamExternalID: "2", StartsAt: now, ObservedAt: now, Status: "final", Season: 2026, SeasonType: kind, HomeScore: &zero, AwayScore: &zero}
			if e := gr.SaveGameImport(ctx, id, "espn", now, []game.Game{g}); e != nil {
				t.Fatal(e)
			}
		}
	}
	scope := dataset.Scope{League: "NBA", Season: 2026, SeasonType: 2, From: "2026-01-01", To: "2026-01-01"}
	data, e := dr.Read(ctx, scope)
	if e != nil {
		t.Fatal(e)
	}
	if len(data.Games) != 1 || data.Games[0].ID != "regular" || len(data.Players) != 0 || len(data.ImportDates) != 1 || dataset.Audit(scope, data).Ready {
		t.Fatalf("before %+v", data)
	}
	refs, e := pr.PlayerGames(ctx, "NBA", 2026, 2)
	if e != nil {
		t.Fatal(e)
	}
	box := player.BoxScore{ObservedAt: now, Lines: []player.Line{{ExternalID: "p1", Name: "One", TeamExternalID: "1", Category: "general", Stats: map[string]string{"points": "0"}}, {ExternalID: "p2", Name: "Two", TeamExternalID: "2", Category: "general", Stats: map[string]string{"points": "0"}}}}
	if e := pr.SavePlayerGame(ctx, refs[0], box); e != nil {
		t.Fatal(e)
	}
	data, e = dr.Read(ctx, scope)
	if e != nil {
		t.Fatal(e)
	}
	if len(data.Players) != 2 || !dataset.Audit(scope, data).Ready {
		t.Fatalf("after %+v", data)
	}
	// Incomplete player snapshots must not leak partial lines into CSV exports.
	if _, e := pool.Exec(ctx, "DELETE FROM player_game_stats WHERE player_id=(SELECT id FROM players WHERE external_id='p1')"); e != nil {
		t.Fatal(e)
	}
	data, e = dr.Read(ctx, scope)
	if e != nil {
		t.Fatal(e)
	}
	if len(data.Players) != 0 || data.Games[0].PlayersComplete {
		t.Fatalf("partial snapshot %+v", data)
	}
	scope.Season = 2025
	data, e = dr.Read(ctx, scope)
	if e != nil || len(data.Games) != 0 {
		t.Fatalf("season scope %+v %v", data, e)
	}
}
