package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/madman321000/sports-predictions/internal/config"
	"github.com/madman321000/sports-predictions/internal/database"
	"github.com/madman321000/sports-predictions/internal/postgres"
	"github.com/madman321000/sports-predictions/internal/seed"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("failed to run: %v", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := postgres.NewPostgresLeagueRepository(pool)
	if err := seed.SeedLeagues(ctx, repo); err != nil {
		return err
	}

	log.Printf("Seeded Leagues")
	return nil
}
