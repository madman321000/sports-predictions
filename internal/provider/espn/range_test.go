package espn

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestGameRangeRequestAndBounds(t *testing.T) {
	body, err := os.ReadFile("testdata/nba_games.json")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("dates") != "20260101-20260131" || r.URL.Query().Get("limit") != "1000" {
			t.Error(r.URL)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()
	client, err := NewClient(Options{BaseURL: server.URL, RequestInterval: 5 * time.Second, HTTPTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	games, err := client.FetchGameRange(context.Background(), "NBA", from, from.AddDate(0, 0, 30))
	if err != nil || len(games) == 0 {
		t.Fatal(games, err)
	}
	if _, err = client.FetchGameRange(context.Background(), "NBA", from, from.AddDate(0, 0, 31)); err == nil {
		t.Fatal("oversized range accepted")
	}
	if calls != 1 {
		t.Fatal("invalid range reached provider")
	}
}
func TestGameRangeRejectsPossibleTruncation(t *testing.T) {
	events := make([]map[string]any, 1000)
	body, _ := json.Marshal(map[string]any{"events": events})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	client, err := NewClient(Options{BaseURL: server.URL, RequestInterval: 5 * time.Second, HTTPTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now()
	if _, err = client.FetchGameRange(context.Background(), "NFL", date, date); err == nil {
		t.Fatal("potentially truncated range accepted")
	}
}
