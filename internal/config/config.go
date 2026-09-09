package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL string
}

func LoadConfig() (*Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL environment variable is not set")
	}

	return &Config{
		DatabaseURL: databaseURL,
	}, nil
}
