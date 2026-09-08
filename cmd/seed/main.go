package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/madman321000/sports-predictions/internal/database"
)

type League struct {
	Name         string
	Abbreviation string
	Sport        string
}

func SeedLeagues(
	ctx context.Context,
	db *pgxpool.Pool,
	leagues []League,
) error {
	for _, league := range leagues {
		_, err := db.Exec(
			ctx,
			"INSERT INTO leagues (name, abbreviation, sport) VALUES ($1, $2, $3) DO UPDATE SET name = EXCLUDED.name, sport = EXCLUDED.sport, updated_at = NOW();",
			league.Name,
			league.Abbreviation,
			league.Sport,
		)
		if err != nil {
			log.Fatal(err)
			return err
		}
	}
	return nil
}

var leagues = []League{
	{Name: "National Football League", Abbreviation: "NFL", Sport: "Football"},
	{Name: "National Basketball Association", Abbreviation: "NBA", Sport: "Basketball"},
}

func main() {
	ctx := context.Background()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}

	pool, err := database.NewPool(ctx, databaseURL)
	if err != nil {
		log.Fatalf("failed to create database pool: %v", err)
	}

	err = SeedLeagues(ctx, pool, leagues)
	if err != nil {
		log.Fatalf("failed to seed leagues: %v", err)
	}

	defer pool.Close()

	log.Println("connected to postgres")
}
