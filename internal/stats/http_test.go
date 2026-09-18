package stats

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeRepo struct {
	calls    int
	resource string
	filter   Filter
	err      error
}

func (r *fakeRepo) Browse(ctx context.Context, resource string, f Filter) (Page, error) {
	r.calls++
	r.resource = resource
	r.filter = f
	if _, ok := ctx.Deadline(); !ok {
		panic("missing query deadline")
	}
	return Page{Items: json.RawMessage(`[]`), Limit: f.Limit, Offset: f.Offset}, r.err
}
func TestFiltersAndReadOnly(t *testing.T) {
	repo := &fakeRepo{}
	h := NewHandler(repo)
	for _, path := range []string{
		"/api/stats/games", "/api/stats/games?league=MLB&season=2026", "/api/stats/games?league=NBA&season=2026&limit=101",
		"/api/stats/games?league=NBA&season=2026&offset=-1", "/api/stats/games?league=NBA&season=2026&season_type=0",
		"/api/stats/games?league=NBA&season=2026&team_id=1%20OR%201=1", "/api/stats/games?league=NBA&season=2026&status=bad",
		"/api/stats/players?league=NBA&season=2026&game_id=1", "/api/stats/games?league=NBA&league=NFL&season=2026",
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	if repo.calls != 0 {
		t.Fatal("invalid input reached database")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/stats/player-games?league=NFL&season=2025&season_type=3&team_id=2&player_id=3&game_id=4&category=passing&limit=10&offset=20", nil))
	if w.Code != 200 || repo.filter.SeasonType != 3 || repo.filter.GameID != 4 || repo.filter.PlayerID != 3 || repo.filter.TeamID != 2 || repo.filter.Limit != 10 || repo.filter.Offset != 20 || repo.filter.Category != "passing" {
		t.Fatalf("bad parsed filter: %+v, %d", repo.filter, w.Code)
	}
	for _, method := range []string{"POST", "DELETE", "PUT"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, "/api/stats/games", nil))
		if w.Code != 405 {
			t.Fatal("write method accepted")
		}
	}
	repo.err = errors.New("postgres://private-credentials")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/stats/leagues", nil))
	if w.Code != 503 || strings.Contains(w.Body.String(), "private-credentials") {
		t.Fatal("database error not sanitized")
	}
	w = httptest.NewRecorder()
	NewHandler(nil).ServeHTTP(w, httptest.NewRequest("GET", "/api/stats/leagues", nil))
	if w.Code != 503 {
		t.Fatal("disabled browsing should explain configuration")
	}
}
