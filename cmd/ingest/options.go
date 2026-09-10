package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/madman321000/sports-predictions/internal/config"
	"github.com/madman321000/sports-predictions/internal/ingest"
	"github.com/madman321000/sports-predictions/internal/provider/espn"
)

type commandOptions struct {
	resource, league string
	interval         time.Duration
	games            ingest.GameOptions
}

func parseOptions(args []string, cfg *config.Config, output io.Writer) (commandOptions, error) {
	var options commandOptions
	flags := flag.NewFlagSet("ingest", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&options.resource, "resource", "teams", "resource to import: teams or games")
	flags.StringVar(&options.league, "league", "NBA", "league to import: NBA or NFL")
	flags.DurationVar(&options.interval, "request-interval", cfg.ESPNRequestInterval, "minimum ESPN request spacing (at least 5s)")
	from := flags.String("from", "", "first ESPN calendar date, YYYY-MM-DD (games only)")
	to := flags.String("to", "", "last ESPN calendar date, inclusive (games only)")
	workers := flags.Int("workers", cfg.IngestWorkers, "date workers, 1-4; HTTP requests remain paced")
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if flags.NArg() != 0 {
		return options, fmt.Errorf("no positional arguments expected")
	}
	if options.league != "NBA" && options.league != "NFL" {
		return options, fmt.Errorf("league must be NBA or NFL")
	}
	if options.interval < espn.MinInterval || options.interval > time.Hour {
		return options, fmt.Errorf("request interval must be between 5s and 1h")
	}
	if *workers < 1 || *workers > 4 {
		return options, fmt.Errorf("workers must be between 1 and 4")
	}
	switch options.resource {
	case "teams":
		if *from != "" || *to != "" {
			return options, fmt.Errorf("date flags only apply to games")
		}
	case "games":
		start, err := time.Parse(time.DateOnly, *from)
		if err != nil {
			return options, fmt.Errorf("from must be YYYY-MM-DD")
		}
		end, err := time.Parse(time.DateOnly, *to)
		if err != nil {
			return options, fmt.Errorf("to must be YYYY-MM-DD")
		}
		options.games = ingest.GameOptions{League: options.league, From: start, To: end, Workers: *workers}
		if err := options.games.Validate(); err != nil {
			return options, err
		}
	default:
		return options, fmt.Errorf("resource must be teams or games")
	}
	return options, nil
}
