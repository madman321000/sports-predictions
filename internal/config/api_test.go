package config

import (
	"os"
	"testing"
)

func TestAPIEnvironment(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, key := range []string{"API_ADDRESS", "API_RUNS_DIR", "API_DATABASE_URL", "FRONTEND_ORIGIN", "PORT"} {
		t.Setenv(key, "")
	}
	cfg, err := LoadAPI()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != "127.0.0.1:8080" || cfg.RunsDir != "published" || cfg.DatabaseURL != "" {
		t.Fatal("bad local defaults")
	}
	t.Setenv("PORT", "10000")
	cfg, err = LoadAPI()
	if err != nil || cfg.Address != ":10000" {
		t.Fatal("platform port ignored")
	}
	t.Setenv("API_ADDRESS", ":8080")
	t.Setenv("API_DATABASE_URL", "postgres://environment/db")
	if err = os.WriteFile(".env", []byte("API_DATABASE_URL=postgres://file/db\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadAPI()
	if err != nil || cfg.Address != ":8080" || cfg.DatabaseURL != "postgres://environment/db" {
		t.Fatal("environment override failed")
	}
}
