// Package stats exposes a bounded, read-only view of imported sports data.
package stats

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Filter struct {
	League                            string
	Season, SeasonType, Limit, Offset int
	TeamID, PlayerID, GameID          int64
	Category, Search, Status          string
}

type Page struct {
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
	Items  json.RawMessage `json:"items"`
}

// Repository implementations must use parameterized queries and deterministic ordering.
type Repository interface {
	Browse(context.Context, string, Filter) (Page, error)
}

var resources = map[string]bool{"leagues": true, "seasons": true, "teams": true, "games": true, "players": true, "player-games": true, "totals": true}

func NewHandler(repo Repository) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/stats/{resource}", func(w http.ResponseWriter, r *http.Request) {
		resource := r.PathValue("resource")
		if !resources[resource] {
			http.Error(w, "resource not found", http.StatusNotFound)
			return
		}
		if repo == nil {
			http.Error(w, "Statistics browsing is not configured. Set STATS_DATABASE_URL and restart the API.", http.StatusServiceUnavailable)
			return
		}
		f, ok := parseFilter(r, resource)
		if !ok {
			http.Error(w, "invalid filters: use NBA or NFL, a season, and valid pagination", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		page, err := repo.Browse(ctx, resource, f)
		if err != nil {
			http.Error(w, "Statistics are temporarily unavailable. Check database access and migrations.", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	})
	return mux
}

func parseFilter(r *http.Request, resource string) (Filter, bool) {
	q := r.URL.Query()
	f := Filter{League: q.Get("league"), SeasonType: 2, Limit: 25}
	allowed := map[string]bool{"limit": true, "offset": true}
	if resource != "leagues" {
		allowed["league"] = true
	}
	if resource != "leagues" && resource != "seasons" {
		allowed["season"] = true
		allowed["season_type"] = true
	}
	if resource == "games" || resource == "players" || resource == "player-games" || resource == "totals" {
		allowed["team_id"] = true
	}
	if resource == "player-games" || resource == "totals" {
		allowed["player_id"] = true
		allowed["category"] = true
	}
	if resource == "player-games" {
		allowed["game_id"] = true
	}
	if resource == "players" {
		allowed["q"] = true
	}
	if resource == "games" {
		allowed["status"] = true
	}
	for key, values := range q {
		if !allowed[key] || len(values) != 1 {
			return f, false
		}
	}
	if resource != "leagues" && f.League != "NBA" && f.League != "NFL" {
		return f, false
	}
	for key, dest := range map[string]*int{"season": &f.Season, "season_type": &f.SeasonType, "limit": &f.Limit, "offset": &f.Offset} {
		if raw := q.Get(key); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				return f, false
			}
			*dest = n
		}
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 1000000 || f.SeasonType < 1 || f.SeasonType > 4 {
		return f, false
	}
	if resource != "leagues" && resource != "seasons" && (f.Season < 1 || f.Season > 9999) {
		return f, false
	}
	for key, dest := range map[string]*int64{"team_id": &f.TeamID, "player_id": &f.PlayerID, "game_id": &f.GameID} {
		if raw := q.Get(key); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n < 1 {
				return f, false
			}
			*dest = n
		}
	}
	f.Category = q.Get("category")
	f.Search = strings.TrimSpace(q.Get("q"))
	f.Status = q.Get("status")
	if len(f.Category) > 80 || len(f.Search) > 100 {
		return f, false
	}
	if f.Status != "" && !map[string]bool{"scheduled": true, "in_progress": true, "final": true, "postponed": true, "canceled": true, "suspended": true, "delayed": true}[f.Status] {
		return f, false
	}
	return f, true
}
