package main

import (
	"io"
	"testing"
	"time"

	"github.com/madman321000/sports-predictions/internal/config"
)

func TestParseOptions(t *testing.T) {
	cfg := &config.Config{ESPNRequestInterval: 5 * time.Second, IngestWorkers: 1}
	for _, args := range [][]string{
		{"-league", "NFL", "-resource", "teams"},
		{"-league", "NBA", "-resource", "games", "-from", "2026-01-01", "-to", "2026-01-31", "-workers", "3"},
		{"-league", "NFL", "-resource", "games", "-from", "2025-09-07", "-to", "2025-09-07"},
	} {
		if _, err := parseOptions(args, cfg, io.Discard); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"-resource", "games"}, {"-league", "MLB"}, {"-workers", "5"}, {"-request-interval", "1s"},
		{"-resource", "teams", "-from", "2026-01-01"},
		{"-resource", "games", "-from", "2026-02-30", "-to", "2026-03-01"},
		{"-resource", "games", "-from", "2026-02-01", "-to", "2026-01-01"},
		{"-resource", "games", "-from", "2026-01-01", "-to", "2026-02-01"},
	} {
		if _, err := parseOptions(args, cfg, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestForceRefreshOptions(t *testing.T) {
	cfg := &config.Config{ESPNRequestInterval: 5 * time.Second, IngestWorkers: 1}
	options, err := parseOptions([]string{"-resource", "teams", "-league", "NFL", "-force"}, cfg, io.Discard)
	if err != nil || !options.force {
		t.Fatalf("force teams: %+v %v", options, err)
	}
	options, err = parseOptions([]string{"-resource", "games", "-force", "-from", "2026-01-01", "-to", "2026-01-01"}, cfg, io.Discard)
	if err != nil || !options.games.Force {
		t.Fatalf("force games: %+v %v", options, err)
	}
}

func TestPlayerOptions(t *testing.T) {
	cfg := &config.Config{ESPNRequestInterval: 5 * time.Second, IngestWorkers: 1}
	good, err := parseOptions([]string{"-resource", "players", "-league", "NFL", "-season", "2025", "-season-type", "3", "-workers", "2", "-force"}, cfg, io.Discard)
	if err != nil || good.players.Season != 2025 || good.players.SeasonType != 3 || !good.players.Force || good.players.Workers != 2 {
		t.Fatalf("%+v %v", good, err)
	}
	for _, args := range [][]string{
		{"-resource", "players"},
		{"-resource", "players", "-season", "2025", "-season-type", "1"},
		{"-resource", "players", "-season", "2025", "-from", "2025-09-01"},
		{"-resource", "teams", "-season", "2025"},
	} {
		if _, err := parseOptions(args, cfg, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
