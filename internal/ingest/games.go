package ingest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/madman321000/sports-predictions/internal/game"
	"golang.org/x/sync/errgroup"
)

type GameSource interface {
	FetchGames(context.Context, string, time.Time) ([]game.Game, error)
}
type GameStore interface {
	LeagueID(context.Context, string) (int64, error)
	UpsertGames(context.Context, int64, string, []game.Game) error
}

type GameOptions struct {
	League   string
	From, To time.Time
	Workers  int
}
type GameResult struct {
	DatesProcessed int
	GamesProcessed int
}

func (o GameOptions) Validate() error {
	if o.League != "NBA" && o.League != "NFL" {
		return fmt.Errorf("league must be NBA or NFL")
	}
	if o.Workers < 1 || o.Workers > 4 {
		return fmt.Errorf("workers must be between 1 and 4")
	}
	if o.From.IsZero() || o.To.IsZero() || o.To.Before(o.From) {
		return fmt.Errorf("valid from/to dates in ascending order are required")
	}
	if o.From.Hour() != 0 || o.From.Minute() != 0 || o.From.Second() != 0 || o.From.Nanosecond() != 0 || o.To.Hour() != 0 || o.To.Minute() != 0 || o.To.Second() != 0 || o.To.Nanosecond() != 0 {
		return fmt.Errorf("from/to must be calendar dates at midnight")
	}
	_, fromOffset := o.From.Zone()
	_, toOffset := o.To.Zone()
	if fromOffset != 0 || toOffset != 0 {
		return fmt.Errorf("from/to must use UTC calendar dates")
	}
	if o.To.Sub(o.From) >= 31*24*time.Hour {
		return fmt.Errorf("import at most 31 days per run")
	}
	return nil
}

// IngestGames bounds workers while the source owns shared request pacing.
// On error, committed dates remain; the returned result reports partial progress.
func IngestGames(ctx context.Context, source GameSource, store GameStore, options GameOptions) (GameResult, error) {
	var result GameResult
	if err := options.Validate(); err != nil {
		return result, err
	}
	id, err := store.LeagueID(ctx, options.League)
	if err != nil {
		return result, fmt.Errorf("look up league (run migrations and seed first): %w", err)
	}
	dates := make(chan time.Time, 31)
	for date := options.From.UTC(); !date.After(options.To); date = date.AddDate(0, 0, 1) {
		dates <- date
	}
	close(dates)
	group, workCtx := errgroup.WithContext(ctx)
	var mu sync.Mutex
	for i := 0; i < options.Workers; i++ {
		group.Go(func() error {
			for date := range dates {
				if err := workCtx.Err(); err != nil {
					return err
				}
				games, err := source.FetchGames(workCtx, options.League, date)
				if err != nil {
					return fmt.Errorf("fetch %s %s: %w", options.League, date.Format(time.DateOnly), err)
				}
				if err := store.UpsertGames(workCtx, id, "espn", games); err != nil {
					return fmt.Errorf("save %s %s: %w", options.League, date.Format(time.DateOnly), err)
				}
				mu.Lock()
				result.DatesProcessed++
				result.GamesProcessed += len(games)
				mu.Unlock()
			}
			return nil
		})
	}
	err = group.Wait()
	if err == nil {
		err = ctx.Err()
	}
	return result, err
}
