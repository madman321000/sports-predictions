package espn

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/madman321000/sports-predictions/internal/player"
)

func TestPlayerGameDecode(t *testing.T) {
	for _, league := range []string{"NBA", "NFL"} {
		t.Run(league, func(t *testing.T) {
			file := "testdata/nba_players.json"
			season := 2026
			if league == "NFL" {
				file = "testdata/nfl_players.json"
				season = 2025
			}
			body, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			g := player.GameRef{ExternalID: "g1", HomeTeamExternalID: "1", AwayTeamExternalID: "2", Season: season, SeasonType: 2}
			got, err := decodePlayerGame(body, league, g)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Lines) != 4 {
				t.Fatalf("lines %d", len(got.Lines))
			}
			if league == "NBA" && got.Lines[0].Stats["points"] != "0" {
				t.Fatal("lost zero")
			}
			if league == "NFL" && got.Lines[1].Category != "rushing" {
				t.Fatal("lost repeated athlete category")
			}
			cases := map[string]func(*playerSummary){
				"wrong game":       func(d *playerSummary) { d.Header.ID = "other" },
				"wrong season":     func(d *playerSummary) { d.Header.Season.Year-- },
				"wrong type":       func(d *playerSummary) { d.Header.Season.Type = 3 },
				"wrong league":     func(d *playerSummary) { d.Header.League.Abbreviation = "other" },
				"not final":        func(d *playerSummary) { d.Header.Competitions[0].Status.Type.Completed = false },
				"missing team":     func(d *playerSummary) { d.Boxscore.Players = d.Boxscore.Players[:1] },
				"duplicate team":   func(d *playerSummary) { d.Boxscore.Players[1].Team.ID = "1" },
				"empty team":       func(d *playerSummary) { d.Boxscore.Players[0].Statistics = nil },
				"missing stats":    func(d *playerSummary) { d.Boxscore.Players[0].Statistics[0].Athletes[0].Stats = nil },
				"missing identity": func(d *playerSummary) { d.Boxscore.Players[0].Statistics[0].Athletes[0].Athlete.ID = "" },
				"duplicate keys": func(d *playerSummary) {
					d.Boxscore.Players[0].Statistics[0].Keys[1] = d.Boxscore.Players[0].Statistics[0].Keys[0]
				},
				"duplicate player": func(d *playerSummary) {
					c := &d.Boxscore.Players[0].Statistics[0]
					c.Athletes = append(c.Athletes, c.Athletes[0])
				},
			}
			for name, mutate := range cases {
				t.Run(name, func(t *testing.T) {
					var d playerSummary
					if err := json.Unmarshal(body, &d); err != nil {
						t.Fatal(err)
					}
					mutate(&d)
					bad, err := json.Marshal(d)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := decodePlayerGame(bad, league, g); err == nil {
						t.Fatal("accepted invalid summary")
					}
				})
			}
		})
	}
}
func TestFetchPlayerGame(t *testing.T) {
	body, err := os.ReadFile("testdata/nfl_players.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/site/v2/sports/football/nfl/summary" || r.URL.Query().Get("event") != "g1" {
			t.Errorf("URL %s", r.URL)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()
	c, err := NewClient(Options{BaseURL: server.URL, RequestInterval: MinInterval, HTTPTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	box, err := c.FetchPlayerGame(context.Background(), "NFL", player.GameRef{ExternalID: "g1", HomeTeamExternalID: "1", AwayTeamExternalID: "2", Season: 2025, SeasonType: 2})
	if err != nil || box.ObservedAt.IsZero() {
		t.Fatalf("box %v error %v", box, err)
	}
}
