package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/madman321000/sports-predictions/internal/config"
	"github.com/madman321000/sports-predictions/internal/database"
	"github.com/madman321000/sports-predictions/internal/ingest"
	"github.com/madman321000/sports-predictions/internal/postgres"
	"github.com/madman321000/sports-predictions/internal/provider/espn"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	league := flag.String("league", "NBA", "league to import (NBA supported)")
	interval := flag.Duration("request-interval", cfg.ESPNRequestInterval, "minimum ESPN request spacing (at least 5s)")
	flag.Parse()
	if *league != "NBA" || flag.NArg() != 0 {
		return fmt.Errorf("only -league NBA is supported; no positional arguments expected")
	}
	client, err := espn.NewClient(espn.Options{BaseURL: cfg.ESPNBaseURL, RequestInterval: *interval, HTTPTimeout: cfg.ESPNHTTPTimeout})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, cfg.IngestTimeout)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	count, err := ingest.IngestNBATeams(ctx, client, postgres.NewPostgresTeamRepository(pool))
	if err != nil {
		return err
	}
	log.Printf("imported %d NBA teams from ESPN", count)
	return nil
}
