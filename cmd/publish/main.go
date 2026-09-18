package main

import (
	"context"
	"log"
	"time"

	"github.com/madman321000/sports-predictions/internal/config"
	"github.com/madman321000/sports-predictions/internal/dashboard"
)

func main() {
	cfg, err := config.LoadAPI()
	if err != nil {
		log.Fatal("Publication failed; check API_DATABASE_URL, migration and reports (details suppressed to protect credentials)")
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("API_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := dashboard.Publish(ctx, cfg.RunsDir, cfg.DatabaseURL); err != nil {
		log.Fatal("Publication failed; check API_DATABASE_URL, migration and reports (details suppressed to protect credentials)")
	}
	log.Print("Published reports; restart API to load the new snapshot")
}
