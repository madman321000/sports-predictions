package ingest

import (
	"context"
	"fmt"

	"github.com/madman321000/sports-predictions/internal/team"
)

type TeamSource interface {
	FetchTeams(context.Context, string) ([]team.Team, error)
}
type TeamStore interface {
	LeagueID(context.Context, string) (int64, error)
	UpsertTeams(context.Context, int64, string, []team.Team) error
}

// IngestTeams checks prerequisites before using the network and persists one batch.
func IngestTeams(ctx context.Context, league string, source TeamSource, store TeamStore) (int, error) {
	if league != "NBA" && league != "NFL" {
		return 0, fmt.Errorf("unsupported league %q", league)
	}
	id, err := store.LeagueID(ctx, league)
	if err != nil {
		return 0, fmt.Errorf("look up %s league (run migrations and seed first): %w", league, err)
	}
	teams, err := source.FetchTeams(ctx, league)
	if err != nil {
		return 0, fmt.Errorf("ingest %s teams: %w", league, err)
	}
	if len(teams) == 0 {
		return 0, fmt.Errorf("ingest %s teams: empty team list", league)
	}
	if err := store.UpsertTeams(ctx, id, "espn", teams); err != nil {
		return 0, fmt.Errorf("save %s teams: %w", league, err)
	}
	return len(teams), nil
}
