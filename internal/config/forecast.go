package config

import (
	"fmt"
	"time"
)

type Forecast struct {
	Address, Origin, ModelDir, ESPNBaseURL string
	Interval, Timeout                      time.Duration
}

func LoadForecast() (Forecast, error) {
	value, err := environment(".env")
	if err != nil {
		return Forecast{}, err
	}
	cfg := Forecast{Address: value("API_ADDRESS"), Origin: value("FRONTEND_ORIGIN"), ModelDir: value("FORECAST_MODEL_DIR"), ESPNBaseURL: value("ESPN_BASE_URL"), Interval: 5 * time.Second, Timeout: 20 * time.Second}
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:8080"
		if p := value("PORT"); p != "" {
			cfg.Address = ":" + p
		}
	}
	if cfg.ModelDir == "" {
		cfg.ModelDir = "models/serving"
	}
	if cfg.ESPNBaseURL == "" {
		cfg.ESPNBaseURL = "https://site.api.espn.com"
	}
	for key, target := range map[string]*time.Duration{"ESPN_REQUEST_INTERVAL": &cfg.Interval, "ESPN_HTTP_TIMEOUT": &cfg.Timeout} {
		if raw := value(key); raw != "" {
			duration, err := time.ParseDuration(raw)
			if err != nil || duration <= 0 {
				return Forecast{}, fmt.Errorf("invalid %s", key)
			}
			*target = duration
		}
	}
	if cfg.Interval < 5*time.Second || cfg.Interval > time.Hour {
		return Forecast{}, fmt.Errorf("ESPN_REQUEST_INTERVAL must be 5s–1h")
	}
	return cfg, nil
}
