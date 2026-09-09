package seed

import (
	"context"
	"fmt"

	"github.com/madman321000/sports-predictions/internal/league"
)

type LeagueWriter interface {
	Create(ctx context.Context, league *league.League) error
}

func SeedLeagues(ctx context.Context, writer LeagueWriter) error {
	for _, l := range defaultLeagues() {
		if err := writer.Create(ctx, &l); err != nil {
			return fmt.Errorf("failed to seed league %s: %w", l.Name, err)
		}
	}
	return nil
}

func defaultLeagues() []league.League {
	return []league.League{
		{
			Name:         "National Football League",
			Abbreviation: "NFL",
			Sport:        "Football",
		},
		{
			Name:         "National Basketball Association",
			Abbreviation: "NBA",
			Sport:        "Basketball",
		},
	}
}
