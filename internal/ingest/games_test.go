package ingest_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/madman321000/sports-predictions/internal/game"
	"github.com/madman321000/sports-predictions/internal/ingest"
)

type gameSource struct {
	fetch func(context.Context, string, time.Time) ([]game.Game, error)
}

func (s gameSource) FetchGames(ctx context.Context, l string, d time.Time) ([]game.Game, error) {
	return s.fetch(ctx, l, d)
}

type gameStore struct {
	save     func(context.Context, []game.Game) error
	complete func(time.Time) (bool, error)
}

func (s gameStore) LeagueID(context.Context, string) (int64, error) { return 1, nil }
func (s gameStore) SaveGameImport(ctx context.Context, _ int64, _ string, _ time.Time, g []game.Game) error {
	return s.save(ctx, g)
}
func gameOptions(workers int) ingest.GameOptions {
	return ingest.GameOptions{League: "NFL", From: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2025, 9, 4, 0, 0, 0, 0, time.UTC), Workers: workers}
}

func TestIngestGamesBoundsConcurrency(t *testing.T) {
	var active, maximum atomic.Int32
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	var mu sync.Mutex
	dates := map[string]int{}
	src := gameSource{fetch: func(ctx context.Context, l string, d time.Time) ([]game.Game, error) {
		if l != "NFL" {
			t.Error("wrong league")
		}
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := maximum.Load()
			if n <= old || maximum.CompareAndSwap(old, n) {
				break
			}
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		mu.Lock()
		dates[d.Format(time.DateOnly)]++
		mu.Unlock()
		return []game.Game{{ExternalID: d.Format(time.DateOnly)}}, nil
	}}
	db := gameStore{save: func(context.Context, []game.Game) error { return nil }}
	done := make(chan struct{})
	var result ingest.GameResult
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { result, err = ingest.IngestGames(ctx, src, db, gameOptions(2)); close(done) }()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("workers did not start")
		}
	}
	close(release)
	<-done
	if err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 2 || result.DatesProcessed != 4 || result.GamesProcessed != 4 {
		t.Fatalf("concurrency=%d result=%+v", maximum.Load(), result)
	}
	for date, count := range dates {
		if count != 1 {
			t.Errorf("date %s fetched %d times", date, count)
		}
	}
}

func TestIngestGamesReportsPartialProgress(t *testing.T) {
	failure := errors.New("provider failed")
	calls := 0
	src := gameSource{fetch: func(context.Context, string, time.Time) ([]game.Game, error) {
		calls++
		if calls == 2 {
			return nil, failure
		}
		return []game.Game{{ExternalID: "1"}}, nil
	}}
	db := gameStore{save: func(context.Context, []game.Game) error { return nil }}
	result, err := ingest.IngestGames(context.Background(), src, db, gameOptions(1))
	if !errors.Is(err, failure) || result.DatesProcessed != 1 || result.GamesProcessed != 1 || calls != 2 {
		t.Fatalf("result=%+v error=%v calls=%d", result, err, calls)
	}
}

func TestIngestGamesCancelsOtherWorkers(t *testing.T) {
	failure := errors.New("database failed")
	var calls atomic.Int32
	waiting := make(chan struct{})
	src := gameSource{fetch: func(ctx context.Context, _ string, _ time.Time) ([]game.Game, error) {
		if calls.Add(1) == 1 {
			select {
			case <-waiting:
				return []game.Game{{ExternalID: "1"}}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		close(waiting)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	db := gameStore{save: func(context.Context, []game.Game) error { return failure }}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := ingest.IngestGames(ctx, src, db, gameOptions(2))
	if !errors.Is(err, failure) || result.DatesProcessed != 0 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func (s gameStore) GameDateComplete(_ context.Context, _ int64, _ string, date time.Time) (bool, error) {
	if s.complete == nil {
		return false, nil
	}
	return s.complete(date)
}

func TestGamesSkipCachedDatesAndForceRefresh(t *testing.T) {
	var calls atomic.Int32
	src := gameSource{fetch: func(context.Context, string, time.Time) ([]game.Game, error) {
		calls.Add(1)
		return []game.Game{{ExternalID: "1"}}, nil
	}}
	db := gameStore{save: func(context.Context, []game.Game) error { return nil }, complete: func(time.Time) (bool, error) { return true, nil }}
	options := gameOptions(4)
	result, err := ingest.IngestGames(context.Background(), src, db, options)
	if err != nil || calls.Load() != 0 || result.DatesSkipped != 4 || result.DatesProcessed != 0 {
		t.Fatalf("cached result=%+v calls=%d error=%v", result, calls.Load(), err)
	}
	options.Force = true
	result, err = ingest.IngestGames(context.Background(), src, db, options)
	if err != nil || calls.Load() != 4 || result.DatesSkipped != 0 || result.DatesProcessed != 4 {
		t.Fatalf("force result=%+v calls=%d error=%v", result, calls.Load(), err)
	}
}

func TestGamesOnlyFetchIncompleteDates(t *testing.T) {
	var calls atomic.Int32
	src := gameSource{fetch: func(_ context.Context, _ string, date time.Time) ([]game.Game, error) {
		calls.Add(1)
		if date.Day()%2 == 0 {
			t.Error("called provider for complete date")
		}
		return nil, nil
	}}
	db := gameStore{save: func(context.Context, []game.Game) error { return nil }, complete: func(date time.Time) (bool, error) { return date.Day()%2 == 0, nil }}
	result, err := ingest.IngestGames(context.Background(), src, db, gameOptions(2))
	if err != nil || calls.Load() != 2 || result.DatesSkipped != 2 || result.DatesProcessed != 2 {
		t.Fatalf("result=%+v error=%v calls=%d", result, err, calls.Load())
	}
}

func TestGamesCacheErrorDoesNotCallProvider(t *testing.T) {
	failure := errors.New("cache read failed")
	src := gameSource{fetch: func(context.Context, string, time.Time) ([]game.Game, error) {
		t.Error("provider called after cache error")
		return nil, nil
	}}
	db := gameStore{complete: func(time.Time) (bool, error) { return false, failure }}
	_, err := ingest.IngestGames(context.Background(), src, db, gameOptions(1))
	if !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
}

func TestFullSeasonDatesAndProgress(t *testing.T) {
	options := gameOptions(4)
	options.From = time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC)
	options.To = time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	seen := map[string]int{}
	source := gameSource{fetch: func(_ context.Context, _ string, d time.Time) ([]game.Game, error) {
		mu.Lock()
		seen[d.Format(time.DateOnly)]++
		mu.Unlock()
		return []game.Game{{ExternalID: d.Format(time.DateOnly)}}, nil
	}}
	store := gameStore{save: func(context.Context, []game.Game) error { return nil }, complete: func(d time.Time) (bool, error) { return d.Day() == 1, nil }}
	var progress []ingest.GameResult
	options.Progress = func(r ingest.GameResult) { progress = append(progress, r) }
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := ingest.IngestGames(ctx, source, store, options)
	if err != nil {
		t.Fatal(err)
	}
	if result.DatesProcessed != 265 || result.DatesSkipped != 9 || len(progress) != 274 {
		t.Fatalf("result %+v progress %d", result, len(progress))
	}
	for d := options.From; !d.After(options.To); d = d.AddDate(0, 0, 1) {
		want := 1
		if d.Day() == 1 {
			want = 0
		}
		if seen[d.Format(time.DateOnly)] != want {
			t.Fatalf("date %s count %d want %d", d, seen[d.Format(time.DateOnly)], want)
		}
	}
	for i, r := range progress {
		if r.DatesProcessed+r.DatesSkipped != i+1 {
			t.Fatalf("out of order progress %+v", progress)
		}
	}
	if progress[len(progress)-1] != result {
		t.Fatal("last progress differs from result")
	}
}

func TestLongRangeProducerStopsOnFailure(t *testing.T) {
	options := gameOptions(1)
	options.To = options.From.AddDate(20, 0, 0)
	failure := errors.New("stop")
	source := gameSource{fetch: func(context.Context, string, time.Time) ([]game.Game, error) { return nil, failure }}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := ingest.IngestGames(ctx, source, gameStore{}, options)
	if !errors.Is(err, failure) {
		t.Fatalf("producer failed to cancel: %v", err)
	}
}
