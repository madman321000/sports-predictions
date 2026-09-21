package forecast

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
	_ "time/tzdata" // Browser IANA timezones also work in the minimal container.
)

func NewHandler(service *Service, origin string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]any{"status": "ok", "models": len(service.models)})
	})
	mux.HandleFunc("GET /api/today", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, values := range q {
			if (key != "league" && key != "timezone") || len(values) != 1 {
				http.Error(w, "invalid query", 400)
				return
			}
		}
		league := q.Get("league")
		if league != "NBA" && league != "NFL" {
			http.Error(w, "league must be NBA or NFL", 400)
			return
		}
		zone := q.Get("timezone")
		if zone == "" {
			zone = "UTC"
		}
		if len(zone) > 80 || zone == "Local" {
			http.Error(w, "invalid timezone", 400)
			return
		}
		location, err := time.LoadLocation(zone)
		if err != nil {
			http.Error(w, "invalid timezone", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
		defer cancel()
		result, err := service.Today(ctx, league, location)
		if err != nil {
			http.Error(w, "Game schedules are temporarily unavailable. Please retry in a few minutes.", http.StatusServiceUnavailable)
			return
		}
		write(w, result)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin != "" && r.Header.Get("Origin") == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}
func write(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
