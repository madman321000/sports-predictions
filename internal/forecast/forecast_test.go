package forecast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/madman321000/sports-predictions/internal/game"
	"github.com/madman321000/sports-predictions/internal/provider/espn"
	"github.com/madman321000/sports-predictions/internal/team"
)

func testModel(t *testing.T) Model {
	t.Helper()
	var f struct {
		Model Model
		Cases []struct {
			Values   map[string]float64
			Expected float64
		}
	}
	body, err := os.ReadFile("testdata/parity.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(body, &f); err != nil {
		t.Fatal(err)
	}
	if err = f.Model.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Cases {
		if math.Abs(f.Model.Probability(c.Values)-c.Expected) > 1e-12 {
			t.Fatal("Go/sklearn probability mismatch")
		}
	}
	return f.Model
}
func testHistory() []game.Game {
	rows := []game.Game{}
	for i := 0; i < 6; i++ {
		a, b := 100+i, 95
		rows = append(rows, game.Game{ExternalID: fmt.Sprint(i), HomeTeamExternalID: "h", AwayTeamExternalID: "a", Status: "final", Season: 2026, SeasonType: 2, StartsAt: time.Date(2026, 1, 1+i, 12, 0, 0, 0, time.UTC), HomeScore: &a, AwayScore: &b})
	}
	return rows
}
func TestFeaturePolicyAndParity(t *testing.T) {
	testModel(t)
	target := game.Game{ExternalID: "today", HomeTeamExternalID: "h", AwayTeamExternalID: "a", Season: 2026, SeasonType: 2, StartsAt: time.Date(2026, 1, 9, 20, 0, 0, 0, time.UTC)}
	history := testHistory()
	values, h, a := Features(target, history)
	if h != 6 || a != 6 || values["win_rate_diff"] != 1 || values["margin_diff"] != 15 || values["last5_margin_diff"] != 16 || values["rest_days_diff"] != 0 {
		t.Fatal(values, h, a)
	}
	sameDay := history[0]
	sameDay.ExternalID = "same"
	sameDay.StartsAt = target.StartsAt.Add(-10 * time.Hour)
	future := sameDay
	future.ExternalID = "future"
	future.StartsAt = target.StartsAt.Add(24 * time.Hour)
	otherSeason := history[0]
	otherSeason.ExternalID = "old"
	otherSeason.Season = 2025
	after, _, _ := Features(target, append(history, sameDay, future, otherSeason, history[0]))
	if after["margin_diff"] != values["margin_diff"] {
		t.Fatal("same-day/future/duplicate data leaked")
	}
	if v, _, _ := Features(target, history[:4]); v != nil {
		t.Fatal("insufficient warmup accepted")
	}
}

type fakeProvider struct {
	calls   atomic.Int32
	games   []game.Game
	err     error
	onFetch func()
}

func (f *fakeProvider) FetchGameRange(context.Context, string, time.Time, time.Time) ([]game.Game, error) {
	f.calls.Add(1)
	if f.onFetch != nil {
		f.onFetch()
	}
	return f.games, f.err
}
func (f *fakeProvider) FetchTeams(context.Context, string) ([]team.Team, error) {
	return []team.Team{{ExternalID: "h", Name: "Home"}, {ExternalID: "a", Name: "Away"}}, nil
}
func TestTodayTimezoneFreezeAndHTTP(t *testing.T) {
	now := time.Date(2026, 1, 10, 2, 0, 0, 0, time.UTC)
	target := game.Game{ExternalID: "today", HomeTeamExternalID: "h", AwayTeamExternalID: "a", Season: 2026, SeasonType: 2, Status: "scheduled", StartsAt: now.Add(2 * time.Hour)}
	p := &fakeProvider{games: []game.Game{target}}
	s := NewService(context.Background(), p, map[string]Model{"NBA": testModel(t)})
	s.now = func() time.Time { return now }
	state := s.states["NBA"]
	state.season = 2026
	state.history = testHistory()
	state.historyAt = now
	la, _ := time.LoadLocation("America/Los_Angeles")
	first, err := s.Today(context.Background(), "NBA", la)
	if err != nil || first.Date != "2026-01-09" || len(first.Games) != 1 || first.Games[0].Prediction == nil {
		t.Fatal(first, err)
	}
	probability := first.Games[0].Prediction.HomeProbability
	// UTC viewers see Jan 10, but the provider snapshot is shared across zones.
	utc, err := s.Today(context.Background(), "NBA", time.UTC)
	if err != nil || utc.Date != "2026-01-10" || p.calls.Load() != 1 {
		t.Fatal(utc, err, p.calls.Load())
	}
	now = now.Add(3 * time.Hour)
	state.scheduleAt = now
	state.historyAt = now
	state.games[0].Status = "final"
	final, err := s.Today(context.Background(), "NBA", la)
	if err != nil || final.Games[0].Prediction.HomeProbability != probability {
		t.Fatal("pregame forecast not frozen")
	}
	state.saved = map[string]savedPrediction{}
	missing, _ := s.Today(context.Background(), "NBA", la)
	if missing.Games[0].Prediction != nil {
		t.Fatal("postgame forecast invented")
	}
	handler := NewHandler(s, "http://localhost:5173")
	for path, code := range map[string]int{"/api/today?league=NBA&timezone=America%2FLos_Angeles": 200, "/api/today?league=NBA&timezone=Bad/Zone": 400, "/api/today?league=NBA&date=2020-01-01": 400, "/api/today?league=MLB": 400, "/api/stats/players": 404, "/api/runs": 404} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Origin", "http://localhost:5173")
		handler.ServeHTTP(w, req)
		if w.Code != code {
			t.Fatalf("%s: %d", path, w.Code)
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
			t.Fatal("CORS missing")
		}
	}
}
func TestSharedScheduleAndErrorBackoff(t *testing.T) {
	p := &fakeProvider{}
	s := NewService(context.Background(), p, nil)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Today(context.Background(), "NBA", time.UTC); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if p.calls.Load() != 1 {
		t.Fatal("duplicate schedule requests", p.calls.Load())
	}
	p2 := &fakeProvider{err: errors.New("provider blocked")}
	s = NewService(context.Background(), p2, nil)
	for i := 0; i < 3; i++ {
		if _, err := s.Today(context.Background(), "NFL", time.UTC); err == nil {
			t.Fatal("expected failure")
		}
	}
	if p2.calls.Load() != 1 {
		t.Fatal("provider failure repeatedly retried")
	}
}
func TestHistoryRefreshAndModelValidation(t *testing.T) {
	p := &fakeProvider{games: testHistory()}
	s := NewService(context.Background(), p, nil)
	s.now = func() time.Time { return time.Date(2026, 1, 9, 12, 0, 0, 0, time.UTC) }
	state := s.states["NBA"]
	state.season = 2026
	s.refresh("NBA", 2026, nil, time.Time{})
	if len(state.history) != 6 || state.historyAt.IsZero() || state.loading {
		t.Fatal("history not ready")
	}
	p.err = errors.New("unavailable")
	s.refresh("NBA", 2026, state.history, state.historyAt)
	if !state.historyError || len(state.history) != 6 {
		t.Fatal("failed refresh replaced history")
	}
	m := testModel(t)
	m.Scale[0] = 0
	if m.Validate() == nil {
		t.Fatal("invalid scaler accepted")
	}
	if models, err := LoadModels(t.TempDir()); err != nil || len(models) != 0 {
		t.Fatal("missing model should allow schedules")
	}
}

func TestCrossingKickoffDuringFetchAndAbstention(t *testing.T) {
	now := time.Date(2026, 1, 10, 2, 0, 0, 0, time.UTC)
	g := game.Game{ExternalID: "g", HomeTeamExternalID: "h", AwayTeamExternalID: "a", Season: 2026, SeasonType: 2, Status: "scheduled", StartsAt: now.Add(time.Minute)}
	p := &fakeProvider{games: []game.Game{g}, onFetch: func() { now = now.Add(2 * time.Minute) }}
	m := testModel(t)
	s := NewService(context.Background(), p, map[string]Model{"NBA": m})
	s.now = func() time.Time { return now }
	state := s.states["NBA"]
	state.season = 2026
	state.history = testHistory()
	state.historyAt = now
	result, err := s.Today(context.Background(), "NBA", time.UTC)
	if err != nil || result.Games[0].Prediction != nil {
		t.Fatal("computed prediction after kickoff", err)
	}
	state.games[0].StartsAt = now.Add(time.Hour)
	state.historyAt = now.Add(-16 * time.Minute)
	state.retryAt = now.Add(time.Hour)
	result, err = s.Today(context.Background(), "NBA", time.UTC)
	if err != nil || result.HistoryStatus != "stale" || result.Games[0].Prediction != nil {
		t.Fatal("stale history used")
	}
	state.historyAt = now
	m.TrainedThrough = "2026-01-10"
	s.models["NBA"] = m
	result, err = s.Today(context.Background(), "NBA", time.UTC)
	if err != nil || result.Games[0].Prediction != nil {
		t.Fatal("training cutoff leaked")
	}
}

func TestCanceledVisitorDoesNotPoisonSharedCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &fakeProvider{onFetch: cancel}
	s := NewService(context.Background(), p, nil)
	if _, err := s.Today(ctx, "NBA", time.UTC); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Today(context.Background(), "NBA", time.UTC); err != nil {
		t.Fatal(err)
	}
	if p.calls.Load() != 1 {
		t.Fatal("shared schedule was not cached")
	}
	if _, err := s.Today(ctx, "NFL", time.UTC); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled visitor should not start another fetch")
	}
}

func TestProviderErrorsAreSafeAndActionable(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&espn.HTTPError{StatusCode: 400, RetryAfter: "private-value"}, "ESPN HTTP 400"},
		{context.DeadlineExceeded, "provider request timed out"},
		{errors.New("private-value"), "provider response could not be read or validated"},
	} {
		p := &fakeProvider{err: tc.err}
		service := NewService(context.Background(), p, nil)
		response := httptest.NewRecorder()
		NewHandler(service, "").ServeHTTP(response, httptest.NewRequest("GET", "/api/today?league=NBA", nil))
		if response.Code != 503 || !strings.Contains(response.Body.String(), tc.want) || strings.Contains(response.Body.String(), "private-value") {
			t.Fatal(response.Code, response.Body.String())
		}
	}
}
