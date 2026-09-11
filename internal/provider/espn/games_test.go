package espn

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func gameFixture(t *testing.T, league string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + strings.ToLower(league) + "_games.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFetchGames(t *testing.T) {
	for _, league := range []string{"NBA", "NFL"} {
		t.Run(league, func(t *testing.T) {
			body := gameFixture(t, league)
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				path, _ := leaguePath(league)
				if r.URL.Path != path+"/scoreboard" || r.URL.Query().Get("dates") != "20260101" || r.URL.Query().Get("limit") != "1000" {
					t.Errorf("unexpected URL %s", r.URL)
				}
				respond(w, body)
			})
			games, err := c.FetchGames(context.Background(), league, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			if len(games) == 0 || games[0].ObservedAt.IsZero() || games[0].HomeTeamExternalID != "1" || games[0].AwayTeamExternalID != "2" {
				t.Fatalf("unexpected games: %+v", games)
			}
			if league == "NBA" {
				want := []string{"scheduled", "final", "postponed", "canceled", "in_progress", "suspended", "delayed"}
				for i, status := range want {
					if games[i].Status != status {
						t.Fatalf("status %d = %s", i, games[i].Status)
					}
				}
				if games[0].HomeScore != nil || games[0].AwayScore != nil || games[2].HomeScore != nil {
					t.Fatal("unplayed games have scores")
				}
				if games[1].HomeScore == nil || *games[1].HomeScore != 0 {
					t.Fatal("lost a valid zero score")
				}
				if games[0].Week != nil {
					t.Fatal("NBA game has NFL week")
				}
			} else {
				if games[0].Season != 2025 || games[0].SeasonType != 2 || games[0].Week == nil || *games[0].Week != 1 || *games[0].HomeScore != 20 {
					t.Fatal("NFL season/week/score mapping failed")
				}
			}
		})
	}
}

func TestDecodeGamesRejectsInvalidResponses(t *testing.T) {
	body := string(gameFixture(t, "NFL"))
	cases := []string{
		`{}`, `{"leagues":[{"abbreviation":"NFL"}]}`, `{"leagues":[{"abbreviation":"NFL"}],"events":null}`,
		strings.Replace(body, `"abbreviation": "NFL"`, `"abbreviation": "NBA"`, 1),
		strings.Replace(body, `"score": "20"`, `"score": "bad"`, 1),
		strings.Replace(body, `"score": "20"`, `"score": null`, 1),
		strings.Replace(body, `"homeAway": "away"`, `"homeAway": "home"`, 1),
		strings.Replace(body, `"2025-09-07T17:00Z"`, `"not-a-date"`, 1),
	}
	for i, body := range cases {
		if _, err := decodeGames([]byte(body), "NFL", time.Now()); err == nil {
			t.Errorf("accepted invalid case %d", i)
		}
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatal(err)
	}
	response["events"] = json.RawMessage(`[]`)
	empty, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	games, err := decodeGames(empty, "NFL", time.Now())
	if err != nil || len(games) != 0 {
		t.Fatalf("empty day: %v, %v", games, err)
	}
}

func TestTeamsAndGamesSharePacing(t *testing.T) {
	teams := fixture(t)
	games := gameFixture(t, "NBA")
	var mu sync.Mutex
	var times []time.Time
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/teams") {
			respond(w, teams)
		} else {
			respond(w, games)
		}
	})
	c.interval = 20 * time.Millisecond
	var wg sync.WaitGroup
	wg.Go(func() {
		if _, err := c.FetchTeams(context.Background(), "NBA"); err != nil {
			t.Error(err)
		}
	})
	wg.Go(func() {
		if _, err := c.FetchGames(context.Background(), "NBA", time.Now()); err != nil {
			t.Error(err)
		}
	})
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(times) != 2 || times[1].Sub(times[0]) < c.interval {
		t.Fatal("endpoints bypassed shared pacing")
	}
}
