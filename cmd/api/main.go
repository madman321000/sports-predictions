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
	"github.com/madman321000/sports-predictions/internal/dashboard"
)

func main() {
	if err := run(); err != nil {
		log.Print("API startup or shutdown failed; check API_DATABASE_URL, publication migration, and report configuration")
		os.Exit(1)
	}
}
func run() error {
	cfg, err := config.LoadAPI()
	if err != nil {
		return err
	}
	ctxLoad, cancelLoad := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelLoad()
	reports, err := dashboard.Read(ctxLoad, cfg.RunsDir, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	handler := dashboard.New(reports, cfg.Origin)
	server := &http.Server{Addr: cfg.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	log.Printf("dashboard listening on %s", cfg.Address)
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
