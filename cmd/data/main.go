package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/madman321000/sports-predictions/internal/config"
	"github.com/madman321000/sports-predictions/internal/database"
	"github.com/madman321000/sports-predictions/internal/dataset"
	"github.com/madman321000/sports-predictions/internal/postgres"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	o, err := parseOptions(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	url, err := config.LoadDatabaseURL()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	pool, err := database.NewPool(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	data, err := postgres.NewPostgresDatasetRepository(pool).Read(ctx, o.scope)
	if err != nil {
		return fmt.Errorf("read dataset (apply migrations through 000005 first): %w", err)
	}
	report := dataset.Audit(o.scope, data)
	if o.action == "export" {
		report, err = dataset.Export(o.out, o.scope, data, dataset.ExportOptions{AllowIncomplete: o.allow, Overwrite: o.overwrite})
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if writeErr := encoder.Encode(report); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return err
	}
	if o.action == "quality" && !report.Ready {
		return fmt.Errorf("data quality findings require attention")
	}
	return nil
}
