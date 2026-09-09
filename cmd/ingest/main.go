package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/madman321000/sports-predictions/internal/config"
	"github.com/madman321000/sports-predictions/internal/database"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
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

	log.Println("connected to postgres")
	return nil
}
