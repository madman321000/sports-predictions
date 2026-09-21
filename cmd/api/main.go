package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/madman321000/sports-predictions/internal/config"
	"github.com/madman321000/sports-predictions/internal/forecast"
	"github.com/madman321000/sports-predictions/internal/provider/espn"
)

func main() {
	if err := run(); err != nil {
		log.Print("API startup or shutdown failed; check FORECAST_MODEL_DIR and ESPN/API configuration")
		os.Exit(1)
	}
}
func run() error {
	cfg, err := config.LoadForecast()
	if err != nil {
		return err
	}
	models, err := forecast.LoadModels(cfg.ModelDir)
	if err != nil {
		return err
	}
	client, err := espn.NewClient(espn.Options{BaseURL: cfg.ESPNBaseURL, RequestInterval: cfg.Interval, HTTPTimeout: cfg.Timeout})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	handler := forecast.NewHandler(forecast.NewService(ctx, client, models), cfg.Origin)
	server := &http.Server{Addr: cfg.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	log.Printf("today predictions listening on %s", cfg.Address)
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
