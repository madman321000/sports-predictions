package ingest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/madman321000/sports-predictions/internal/player"
)

type playerFake struct {
	mu           sync.Mutex
	games        []player.GameRef
	complete     map[int64]bool
	calls, saves int
	fail         bool
}

func (f *playerFake) PlayerGames(context.Context, string, int, int) ([]player.GameRef, error) {
	return f.games, nil
}
func (f *playerFake) PlayerGameComplete(_ context.Context, g player.GameRef) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.complete[g.ID], nil
}
func (f *playerFake) FetchPlayerGame(ctx context.Context, _ string, _ player.GameRef) (player.BoxScore, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if err := ctx.Err(); err != nil {
		return player.BoxScore{}, err
	}
	if f.fail {
		return player.BoxScore{}, errors.New("fetch failed")
	}
	return player.BoxScore{ObservedAt: time.Now(), Lines: []player.Line{{ExternalID: "p"}}}, nil
}
func (f *playerFake) SavePlayerGame(_ context.Context, g player.GameRef, _ player.BoxScore) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saves++
	f.complete[g.ID] = true
	return nil
}
func TestPlayerImportResumeAndForce(t *testing.T) {
	f := &playerFake{games: []player.GameRef{{ID: 1}, {ID: 2}}, complete: map[int64]bool{1: true}}
	o := PlayerOptions{League: "NFL", Season: 2025, SeasonType: 2, Workers: 2}
	r, err := IngestPlayers(context.Background(), f, f, o)
	if err != nil || r.GamesSkipped != 1 || r.GamesProcessed != 1 || f.calls != 1 {
		t.Fatalf("%+v %v calls %d", r, err, f.calls)
	}
	r, err = IngestPlayers(context.Background(), f, f, o)
	if err != nil || r.GamesSkipped != 2 || f.calls != 1 {
		t.Fatalf("repeat %+v %v", r, err)
	}
	o.Force = true
	r, err = IngestPlayers(context.Background(), f, f, o)
	if err != nil || r.GamesProcessed != 2 || f.calls != 3 {
		t.Fatalf("force %+v %v", r, err)
	}
}
func TestPlayerImportFailureAndValidation(t *testing.T) {
	o := PlayerOptions{League: "NBA", Season: 2026, SeasonType: 2, Workers: 1}
	f := &playerFake{games: []player.GameRef{{ID: 1}, {ID: 2}}, complete: map[int64]bool{}, fail: true}
	if _, err := IngestPlayers(context.Background(), f, f, o); err == nil || f.saves != 0 || f.calls != 1 {
		t.Fatalf("failure %v %+v", err, f)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := IngestPlayers(ctx, f, f, o); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel %v", err)
	}
	f.games = nil
	if _, err := IngestPlayers(context.Background(), f, f, o); err == nil {
		t.Fatal("empty season accepted")
	}
	for _, bad := range []PlayerOptions{{League: "MLB", Season: 2026, SeasonType: 2, Workers: 1}, {League: "NBA", SeasonType: 2, Workers: 1}, {League: "NBA", Season: 2026, SeasonType: 1, Workers: 1}, {League: "NBA", Season: 2026, SeasonType: 2, Workers: 5}} {
		if err := bad.Validate(); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}

type blockingPlayerSource struct {
	started chan struct{}
	release chan struct{}
}

func (f *blockingPlayerSource) FetchPlayerGame(ctx context.Context, _ string, _ player.GameRef) (player.BoxScore, error) {
	select {
	case f.started <- struct{}{}:
	case <-ctx.Done():
		return player.BoxScore{}, ctx.Err()
	}
	select {
	case <-f.release:
		return player.BoxScore{Lines: []player.Line{{ExternalID: "p"}}}, nil
	case <-ctx.Done():
		return player.BoxScore{}, ctx.Err()
	}
}
func TestPlayerWorkerBoundAndCancellation(t *testing.T) {
	source := &blockingPlayerSource{started: make(chan struct{}, 4), release: make(chan struct{})}
	f := &playerFake{games: []player.GameRef{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}, complete: map[int64]bool{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := IngestPlayers(ctx, source, f, PlayerOptions{League: "NBA", Season: 2026, SeasonType: 2, Workers: 2})
		done <- err
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-source.started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	select {
	case <-source.started:
		t.Fatal("exceeded two workers")
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("workers did not stop")
	}
	if f.saves != 0 {
		t.Fatal("canceled work committed")
	}
}
