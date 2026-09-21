package config

import "testing"

func TestForecastWithoutDatabase(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, key := range []string{"API_ADDRESS", "FRONTEND_ORIGIN", "FORECAST_MODEL_DIR", "ESPN_BASE_URL", "ESPN_REQUEST_INTERVAL", "ESPN_HTTP_TIMEOUT", "PORT", "DATABASE_URL", "API_DATABASE_URL", "STATS_DATABASE_URL"} {
		t.Setenv(key, "")
	}
	cfg, err := LoadForecast()
	if err != nil || cfg.ModelDir != "models/serving" || cfg.Address != "127.0.0.1:8080" {
		t.Fatal(cfg, err)
	}
	t.Setenv("ESPN_REQUEST_INTERVAL", "1s")
	if _, err = LoadForecast(); err == nil {
		t.Fatal("unsafe interval accepted")
	}
}
