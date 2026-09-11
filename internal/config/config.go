package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL         string
	ESPNBaseURL         string
	ESPNRequestInterval time.Duration
	ESPNHTTPTimeout     time.Duration
	IngestTimeout       time.Duration
	IngestWorkers       int
}

// LoadConfig reads .env from the working directory. Exported environment
// variables take precedence, and a missing file is allowed for deployments.
func LoadConfig() (*Config, error) { return loadConfig(".env") }

func loadConfig(path string) (*Config, error) {
	value, err := environment(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{DatabaseURL: value("DATABASE_URL"), ESPNBaseURL: value("ESPN_BASE_URL")}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required in .env or the environment")
	}
	for _, setting := range []struct {
		name     string
		fallback time.Duration
		dest     *time.Duration
	}{
		{"ESPN_REQUEST_INTERVAL", 5 * time.Second, &cfg.ESPNRequestInterval},
		{"ESPN_HTTP_TIMEOUT", 20 * time.Second, &cfg.ESPNHTTPTimeout},
		{"INGEST_TIMEOUT", 5 * time.Minute, &cfg.IngestTimeout},
	} {
		duration := setting.fallback
		if raw := value(setting.name); raw != "" {
			duration, err = time.ParseDuration(raw)
			if err != nil {
				return nil, fmt.Errorf("%s must be a duration such as 5s or 1m", setting.name)
			}
		}
		if duration <= 0 {
			return nil, fmt.Errorf("%s must be positive", setting.name)
		}
		*setting.dest = duration
	}
	if cfg.ESPNRequestInterval < 5*time.Second || cfg.ESPNRequestInterval > time.Hour {
		return nil, errors.New("ESPN_REQUEST_INTERVAL must be between 5s and 1h")
	}
	cfg.IngestWorkers = 1
	if raw := value("INGEST_WORKERS"); raw != "" {
		cfg.IngestWorkers, err = strconv.Atoi(raw)
		if err != nil {
			return nil, errors.New("INGEST_WORKERS must be an integer between 1 and 4")
		}
	}
	if cfg.IngestWorkers < 1 || cfg.IngestWorkers > 4 {
		return nil, errors.New("INGEST_WORKERS must be between 1 and 4")
	}
	return cfg, nil
}

// LoadTestDatabaseURL loads only the test connection, without requiring runtime
// application settings. Tests pass the repository-root .env path explicitly.
func LoadTestDatabaseURL(path string) (string, error) {
	value, err := environment(path)
	if err != nil {
		return "", err
	}
	return value("TEST_DATABASE_URL"), nil
}

// Read rather than load into os.Environ, so config reads do not mutate global
// state. Do not include parser errors: they can contain secret file contents.
func environment(path string) (func(string) string, error) {
	values, err := godotenv.Read(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("cannot read .env configuration; check file syntax and permissions")
	}
	return func(key string) string {
		if value, ok := os.LookupEnv(key); ok {
			return value
		}
		return values[key]
	}, nil
}

// LoadDatabaseURL loads only the connection needed by offline data commands.
func LoadDatabaseURL() (string, error) {
	value, err := environment(".env")
	if err != nil {
		return "", err
	}
	url := value("DATABASE_URL")
	if url == "" {
		return "", errors.New("DATABASE_URL is required in .env or the environment")
	}
	return url, nil
}
