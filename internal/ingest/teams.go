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
	TeamsImported(context.Context, int64, string) (bool, error)
	SaveTeamImport(context.Context, int64, string, []team.Team) error
}

type TeamResult struct {
	TeamsProcessed int
	Skipped        bool
}

// IngestTeams checks prerequisites before using the network and persists one batch.
func IngestTeams(ctx context.Context, league string, source TeamSource, store TeamStore, force bool) (TeamResult, error) {
	if league != "NBA" && league != "NFL" {
		return TeamResult{}, fmt.Errorf("unsupported league %q", league)
	}
	id, err := store.LeagueID(ctx, league)
	if err != nil {
		return TeamResult{}, fmt.Errorf("look up %s league (run migrations and seed first): %w", league, err)
	}
	if !force {
		complete, err := store.TeamsImported(ctx, id, "espn")
		if err != nil {
			return TeamResult{}, fmt.Errorf("check stored %s teams: %w", league, err)
		}
		if complete {
			return TeamResult{Skipped: true}, nil
		}
	}
	teams, err := source.FetchTeams(ctx, league)
	if err != nil {
		return TeamResult{}, fmt.Errorf("ingest %s teams: %w", league, err)
	}
	if len(teams) == 0 {
		return TeamResult{}, fmt.Errorf("ingest %s teams: empty team list", league)
	}
	if err := store.SaveTeamImport(ctx, id, "espn", teams); err != nil {
		return TeamResult{}, fmt.Errorf("save %s teams: %w", league, err)
	}
	return TeamResult{TeamsProcessed: len(teams)}, nil
}
