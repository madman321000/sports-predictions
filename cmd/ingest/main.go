package main

import (
	"context"
	"log"
	"os"

	"github.com/madman321000/sports-predictions/internal/database"
)

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
	defer pool.Close()

	log.Println("connected to postgres")
}
