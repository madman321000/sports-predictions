// Package dashboard serves published backtest artifacts, never models or credentials.
package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

var validID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$`)

type prediction struct {
	Candidate   string  `json:"candidate"`
	GameID      string  `json:"game_id"`
	Date        string  `json:"date"`
	HomeWin     int     `json:"home_win"`
	Probability float64 `json:"probability"`
	Home        string  `json:"home_team_id,omitempty"`
	Away        string  `json:"away_team_id,omitempty"`
	HomeScore   *int    `json:"home_score,omitempty"`
	AwayScore   *int    `json:"away_score,omitempty"`
}
type metric struct {
	Games    int     `json:"games"`
	Accuracy float64 `json:"accuracy"`
	LogLoss  float64 `json:"log_loss"`
	Brier    float64 `json:"brier_score"`
}
type bin struct {
	Lower    float64  `json:"lower"`
	Upper    float64  `json:"upper"`
	Games    int      `json:"games"`
	Mean     *float64 `json:"mean_probability"`
	Observed *float64 `json:"observed_home_win_rate"`
}
type candidate struct {
	Metrics     metric `json:"metrics"`
	Calibration []bin  `json:"calibration"`
}
type report struct {
	Schema         int                  `json:"schema_version"`
	Kind           string               `json:"kind"`
	League         string               `json:"league"`
	Title          string               `json:"title"`
	Generated      string               `json:"generated_at"`
	TrainingSeason int                  `json:"training_season,omitempty"`
	TestSeason     int                  `json:"test_season"`
	Limitations    []string             `json:"limitations"`
	Candidates     map[string]candidate `json:"candidates"`
	Predictions    []prediction         `json:"predictions,omitempty"`
}

// Load loads an immutable snapshot. Only explicitly published JSON files are read.
func Load(directory string) (map[string]report, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	reports := map[string]report{}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-5]
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !validID.MatchString(id) || !info.Mode().IsRegular() || info.Size() > 32<<20 {
			return nil, fmt.Errorf("invalid published artifact %s", entry.Name())
		}
		body, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		r, err := decodeReport(id, body)
		if err != nil {
			return nil, err
		}
		reports[id] = r
	}
	return reports, nil
}

func decodeReport(id string, body []byte) (report, error) {
	if !validID.MatchString(id) || len(body) > 32<<20 {
		return report{}, fmt.Errorf("invalid report size or ID")
	}
	var r report
	if err := json.Unmarshal(body, &r); err != nil {
		return report{}, fmt.Errorf("decode %s: %w", id, err)
	}
	if r.Schema != 1 || (r.League != "NBA" && r.League != "NFL") || r.Title == "" || len(r.Candidates) == 0 || len(r.Predictions) == 0 {
		return report{}, fmt.Errorf("invalid report %s", id)
	}
	for _, c := range r.Candidates {
		if c.Metrics.Games <= 0 || c.Metrics.Accuracy < 0 || c.Metrics.Accuracy > 1 || c.Metrics.LogLoss < 0 || c.Metrics.Brier < 0 || c.Metrics.Brier > 1 {
			return report{}, fmt.Errorf("invalid metrics in %s", id)
		}
	}

	for _, p := range r.Predictions {
		if _, ok := r.Candidates[p.Candidate]; !ok || p.Probability < 0 || p.Probability > 1 || (p.HomeWin != 0 && p.HomeWin != 1) || p.GameID == "" {
			return report{}, fmt.Errorf("invalid prediction in %s", id)
		}
	}
	return r, nil
}

func New(reports map[string]report, origin string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "ok", "runs": len(reports)})
	})
	mux.HandleFunc("GET /api/runs", func(w http.ResponseWriter, r *http.Request) {
		ids := make([]string, 0, len(reports))
		for id := range reports {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		list := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			v := reports[id]
			list = append(list, map[string]any{"id": id, "title": v.Title, "league": v.League, "kind": v.Kind, "generated_at": v.Generated})
		}
		writeJSON(w, list)
	})
	mux.HandleFunc("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, ok := reports[r.PathValue("id")]
		if !ok {
			http.Error(w, "run not found", 404)
			return
		}
		v.Predictions = nil
		writeJSON(w, v)
	})
	mux.HandleFunc("GET /api/runs/{id}/predictions", func(w http.ResponseWriter, r *http.Request) {
		v, ok := reports[r.PathValue("id")]
		if !ok {
			http.Error(w, "run not found", 404)
			return
		}
		limit, offset := 50, 0
		for key, dest := range map[string]*int{"limit": &limit, "offset": &offset} {
			if raw := r.URL.Query().Get(key); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil || n < 0 {
					http.Error(w, "invalid pagination", 400)
					return
				}
				*dest = n
			}
		}
		if limit < 1 || limit > 200 {
			http.Error(w, "limit must be 1–200", 400)
			return
		}
		candidate := r.URL.Query().Get("candidate")
		if candidate != "" {
			if _, ok := v.Candidates[candidate]; !ok {
				http.Error(w, "unknown candidate", 400)
				return
			}
		}
		filtered := make([]prediction, 0)
		for _, p := range v.Predictions {
			if candidate == "" || p.Candidate == candidate {
				filtered = append(filtered, p)
			}
		}
		total := len(filtered)
		if offset > total {
			offset = total
		}
		end := offset + min(limit, total-offset)
		writeJSON(w, map[string]any{"total": total, "offset": offset, "limit": limit, "items": filtered[offset:end]})
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "not found", 404) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin != "" && r.Header.Get("Origin") == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; object-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
