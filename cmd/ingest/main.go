package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	options, err := parseOptions(os.Args[1:], cfg, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	client, err := espn.NewClient(espn.Options{BaseURL: cfg.ESPNBaseURL, RequestInterval: options.interval, HTTPTimeout: cfg.ESPNHTTPTimeout})
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

	if options.resource == "teams" {
		result, err := ingest.IngestTeams(ctx, options.league, client, postgres.NewPostgresTeamRepository(pool), options.force)
		if err != nil {
			return err
		}
		if result.Skipped {
			log.Printf("skipped %s teams: complete import already stored", options.league)
		} else {
			log.Printf("imported %d %s teams from ESPN", result.TeamsProcessed, options.league)
		}
		return nil
	}
	if options.resource == "players" {
		result, err := ingest.IngestPlayers(ctx, client, postgres.NewPostgresPlayerRepository(pool), options.players)
		log.Printf("imported %d player statistic lines across %d games; skipped %d complete games", result.LinesProcessed, result.GamesProcessed, result.GamesSkipped)
		return err
	}
	total := (options.games.To.Unix()-options.games.From.Unix())/86400 + 1
	log.Printf("starting %s game import: %d dates, %d workers, request interval %s", options.league, total, options.games.Workers, options.interval)
	lastProgress := time.Now()
	options.games.Progress = func(result ingest.GameResult) {
		done := result.DatesProcessed + result.DatesSkipped
		if done%10 == 0 || int64(done) == total || time.Since(lastProgress) >= 30*time.Second {
			log.Printf("%s progress: %d/%d dates finished; %d imported, %d skipped, %d game records", options.league, done, total, result.DatesProcessed, result.DatesSkipped, result.GamesProcessed)
			lastProgress = time.Now()
		}
	}
	result, err := ingest.IngestGames(ctx, client, postgres.NewPostgresGameRepository(pool), options.games)
	log.Printf("processed %d %s game records across %d committed dates; skipped %d complete dates", result.GamesProcessed, options.league, result.DatesProcessed, result.DatesSkipped)
	return err
}
