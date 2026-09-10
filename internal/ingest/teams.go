package ingest

import (
	"context"
	"fmt"

	"github.com/madman321000/sports-predictions/internal/team"
)

type TeamSource interface {
	NBA(context.Context) ([]team.Team, error)
}
type TeamStore interface {
	LeagueID(context.Context, string) (int64, error)
	UpsertTeams(context.Context, int64, string, []team.Team) error
}

// NBATeams checks prerequisites before using the network and persists one batch.
func NBATeams(ctx context.Context, source TeamSource, store TeamStore) (int, error) {
	id, err := store.LeagueID(ctx, "NBA")
	if err != nil {
		return 0, fmt.Errorf("look up NBA league (run migrations and seed first): %w", err)
	}
	teams, err := source.NBA(ctx)
	if err != nil {
		return 0, fmt.Errorf("ingest NBA teams: %w", err)
	}
	if len(teams) == 0 {
		return 0, fmt.Errorf("ingest NBA teams: empty team list")
	}
	if err := store.UpsertTeams(ctx, id, "espn", teams); err != nil {
		return 0, fmt.Errorf("save NBA teams: %w", err)
	}
	return len(teams), nil
}
