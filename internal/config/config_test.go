package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func configFile(t *testing.T, contents string) string {
	t.Helper()
	for _, key := range []string{"DATABASE_URL", "TEST_DATABASE_URL", "ESPN_BASE_URL", "ESPN_REQUEST_INTERVAL", "ESPN_HTTP_TIMEOUT", "INGEST_TIMEOUT", "INGEST_WORKERS"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigFromEnvFile(t *testing.T) {
	path := configFile(t, "DATABASE_URL='postgres://local/db'\nESPN_BASE_URL=https://example.com\nESPN_REQUEST_INTERVAL=10s\nESPN_HTTP_TIMEOUT=30s\nINGEST_TIMEOUT=2m\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://local/db" || cfg.ESPNBaseURL != "https://example.com" || cfg.ESPNRequestInterval != 10*time.Second || cfg.ESPNHTTPTimeout != 30*time.Second || cfg.IngestTimeout != 2*time.Minute {
		t.Fatalf("unexpected configuration: %+v", cfg)
	}
	if _, set := os.LookupEnv("DATABASE_URL"); set {
		t.Fatal("loading config changed process environment")
	}
}

func TestLoadConfigEnvironmentOverridesFile(t *testing.T) {
	path := configFile(t, "DATABASE_URL=postgres://file/db\nESPN_REQUEST_INTERVAL=10s\n")
	t.Setenv("DATABASE_URL", "postgres://exported/db")
	t.Setenv("ESPN_REQUEST_INTERVAL", "15s")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://exported/db" || cfg.ESPNRequestInterval != 15*time.Second {
		t.Fatal("environment did not take precedence")
	}
	t.Setenv("DATABASE_URL", "")
	if _, err := loadConfig(path); err == nil {
		t.Fatal("explicit empty required setting should fail")
	}
}

func TestLoadConfigWithoutFile(t *testing.T) {
	path := configFile(t, "")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "postgres://local/db")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ESPNRequestInterval != 5*time.Second || cfg.ESPNHTTPTimeout != 20*time.Second || cfg.IngestTimeout != 5*time.Minute {
		t.Fatal("unexpected duration defaults")
	}
}

func TestLoadConfigRejectsInvalidDurations(t *testing.T) {
	for _, setting := range []string{"ESPN_REQUEST_INTERVAL=1s", "ESPN_REQUEST_INTERVAL=2h", "ESPN_HTTP_TIMEOUT=0s", "INGEST_TIMEOUT=-1s", "INGEST_TIMEOUT=invalid", "INGEST_WORKERS=0", "INGEST_WORKERS=5", "INGEST_WORKERS=bad"} {
		t.Run(setting, func(t *testing.T) {
			path := configFile(t, "DATABASE_URL=postgres://local/db\n"+setting+"\n")
			if _, err := loadConfig(path); err == nil {
				t.Fatal("accepted invalid duration")
			}
		})
	}
}

func TestLoadConfigDoesNotExposeMalformedFile(t *testing.T) {
	path := configFile(t, "SECRET='private-value\n")
	_, err := loadConfig(path)
	if err == nil || strings.Contains(err.Error(), "private-value") {
		t.Fatalf("unsafe parser error: %v", err)
	}
}

func TestLoadTestDatabaseURL(t *testing.T) {
	path := configFile(t, "TEST_DATABASE_URL=postgres://file/test\n")
	got, err := LoadTestDatabaseURL(path)
	if err != nil || got != "postgres://file/test" {
		t.Fatalf("got %q, %v", got, err)
	}
	t.Setenv("TEST_DATABASE_URL", "postgres://exported/test")
	got, err = LoadTestDatabaseURL(path)
	if err != nil || got != "postgres://exported/test" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestLoadDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example/database")
	t.Setenv("ESPN_REQUEST_INTERVAL", "invalid")
	got, err := LoadDatabaseURL()
	if err != nil || got != "postgres://example/database" {
		t.Fatalf("%q %v", got, err)
	}
	t.Setenv("DATABASE_URL", "")
	if _, err := LoadDatabaseURL(); err == nil {
		t.Fatal("accepted missing database URL")
	}
}
