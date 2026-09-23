package espn

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestGameRangeRequestAndBounds(t *testing.T) {
	body, err := os.ReadFile("testdata/nba_games.json")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		expected := from.AddDate(0, 0, calls).Format("20060102")
		calls++
		if r.URL.Query().Get("dates") != expected || r.URL.Query().Get("limit") != "1000" {
			t.Error(r.URL)
		}
		respond(w, body)
	})
	games, err := client.FetchGameRange(context.Background(), "NBA", from, from.AddDate(0, 0, 30))
	daily, decodeErr := decodeGames(body, "NBA", time.Now())
	if err != nil || decodeErr != nil || len(games) == 0 || len(games) != len(daily) {
		t.Fatal(games, err, decodeErr)
	}
	if calls != 31 {
		t.Fatalf("got %d calls", calls)
	}
	if _, err = client.FetchGameRange(context.Background(), "NBA", from, from.AddDate(0, 0, 31)); err == nil {
		t.Fatal("oversized range accepted")
	}
	if calls != 31 {
		t.Fatal("invalid range reached provider")
	}
}

func TestGameRangeRejectsPossibleTruncation(t *testing.T) {
	events := make([]map[string]any, 1000)
	body, _ := json.Marshal(map[string]any{"leagues": []map[string]string{{"abbreviation": "NFL"}}, "events": events})
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) { respond(w, body) })
	date := time.Now()
	if _, err := client.FetchGameRange(context.Background(), "NFL", date, date); err == nil {
		t.Fatal("potentially truncated range accepted")
	}
}

func TestGameRangeDiscardsPartialResultsAndHonorsBlock(t *testing.T) {
	body, err := os.ReadFile("testdata/nba_games.json")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		respond(w, body)
	})
	date := time.Now()
	games, err := client.FetchGameRange(context.Background(), "NBA", date, date.AddDate(0, 0, 2))
	var status *HTTPError
	if len(games) != 0 || !errors.As(err, &status) || status.StatusCode != http.StatusForbidden {
		t.Fatal("partial range accepted", err)
	}
	_, _ = client.FetchGameRange(context.Background(), "NBA", date, date)
	if calls != 2 {
		t.Fatal("requested after block", calls)
	}
}
