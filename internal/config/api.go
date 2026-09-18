package config

type API struct{ Address, RunsDir, DatabaseURL, StatsDatabaseURL, Origin string }

func LoadAPI() (API, error) {
	value, err := environment(".env")
	if err != nil {
		return API{}, err
	}
	cfg := API{StatsDatabaseURL: value("STATS_DATABASE_URL"), Address: value("API_ADDRESS"), RunsDir: value("API_RUNS_DIR"), DatabaseURL: value("API_DATABASE_URL"), Origin: value("FRONTEND_ORIGIN")}
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:8080"
		if port := value("PORT"); port != "" {
			cfg.Address = ":" + port
		}
	}
	if cfg.RunsDir == "" {
		cfg.RunsDir = "published"
	}
	return cfg, nil
}
