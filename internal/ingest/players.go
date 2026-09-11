package ingest

import (
	"context"
	"fmt"
	"sync"

	"github.com/madman321000/sports-predictions/internal/player"
	"golang.org/x/sync/errgroup"
)

type PlayerSource interface {
	FetchPlayerGame(context.Context, string, player.GameRef) (player.BoxScore, error)
}
type PlayerStore interface {
	PlayerGames(context.Context, string, int, int) ([]player.GameRef, error)
	PlayerGameComplete(context.Context, player.GameRef) (bool, error)
	SavePlayerGame(context.Context, player.GameRef, player.BoxScore) error
}
type PlayerOptions struct {
	League                      string
	Season, SeasonType, Workers int
	Force                       bool
}
type PlayerResult struct{ GamesProcessed, GamesSkipped, LinesProcessed, UnidentifiedDNP int }

func (o PlayerOptions) Validate() error {
	if o.League != "NBA" && o.League != "NFL" {
		return fmt.Errorf("league must be NBA or NFL")
	}
	if o.Season < 1900 || o.Season > 2200 {
		return fmt.Errorf("explicit season year between 1900 and 2200 is required")
	}
	if o.SeasonType != 2 && o.SeasonType != 3 {
		return fmt.Errorf("season-type must be 2 (regular) or 3 (postseason)")
	}
	if o.Workers < 1 || o.Workers > 4 {
		return fmt.Errorf("workers must be between 1 and 4")
	}
	return nil
}

// IngestPlayers imports only final games already in the database. One atomic
// game is a checkpoint; an interrupted season can be resumed without refetching.
func IngestPlayers(ctx context.Context, source PlayerSource, store PlayerStore, o PlayerOptions) (PlayerResult, error) {
	var result PlayerResult
	if err := o.Validate(); err != nil {
		return result, err
	}
	games, err := store.PlayerGames(ctx, o.League, o.Season, o.SeasonType)
	if err != nil {
		return result, fmt.Errorf("list stored player games: %w", err)
	}
	if len(games) == 0 {
		return result, fmt.Errorf("no final games for %s season %d type %d; import games first", o.League, o.Season, o.SeasonType)
	}
	jobs := make(chan player.GameRef, len(games))
	for _, g := range games {
		jobs <- g
	}
	close(jobs)
	group, workCtx := errgroup.WithContext(ctx)
	var mu sync.Mutex
	for i := 0; i < o.Workers; i++ {
		group.Go(func() error {
			for g := range jobs {
				if err := workCtx.Err(); err != nil {
					return err
				}
				if !o.Force {
					complete, err := store.PlayerGameComplete(workCtx, g)
					if err != nil {
						return fmt.Errorf("check player game %s: %w", g.ExternalID, err)
					}
					if complete {
						mu.Lock()
						result.GamesSkipped++
						mu.Unlock()
						continue
					}
				}
				box, err := source.FetchPlayerGame(workCtx, o.League, g)
				if err != nil {
					return fmt.Errorf("fetch player game %s: %w", g.ExternalID, err)
				}
				if len(box.Lines) == 0 {
					return fmt.Errorf("empty player game %s", g.ExternalID)
				}
				if err := store.SavePlayerGame(workCtx, g, box); err != nil {
					return fmt.Errorf("save player game %s: %w", g.ExternalID, err)
				}
				mu.Lock()
				result.GamesProcessed++
				result.LinesProcessed += len(box.Lines)
				result.UnidentifiedDNP += box.UnidentifiedDNP
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
