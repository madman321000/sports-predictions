// Package forecast runs locally trained models without a training runtime or database.
package forecast

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/madman321000/sports-predictions/internal/game"
)

type Model struct {
	Schema         int       `json:"schema_version"`
	ID             string    `json:"model_id"`
	League         string    `json:"league"`
	SeasonType     int       `json:"season_type"`
	Policy         string    `json:"feature_policy"`
	MinimumGames   int       `json:"minimum_games"`
	TrainedThrough string    `json:"trained_through"`
	Features       []string  `json:"features"`
	Mean           []float64 `json:"mean"`
	Scale          []float64 `json:"scale"`
	Coefficients   []float64 `json:"coefficients"`
	Intercept      float64   `json:"intercept"`
}

var featureNames = []string{"win_rate_diff", "points_for_diff", "points_against_diff", "margin_diff", "last5_win_rate_diff", "last5_margin_diff", "rest_days_diff"}

func LoadModels(directory string) (map[string]Model, error) {
	models := map[string]Model{}
	for _, league := range []string{"NBA", "NFL"} {
		name := "nba.json"
		if league == "NFL" {
			name = "nfl.json"
		}
		path := filepath.Join(directory, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > 65536 {
			return nil, fmt.Errorf("invalid model artifact")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var m Model
		if err = json.Unmarshal(body, &m); err != nil {
			return nil, fmt.Errorf("invalid %s model JSON", league)
		}
		if err = m.Validate(); err != nil || m.League != league {
			return nil, fmt.Errorf("invalid %s model schema", league)
		}
		models[league] = m
	}
	return models, nil
}
func (m Model) Validate() error {
	n := len(m.Features)
	if m.Schema != 1 || m.ID == "" || len(m.ID) > 100 || (m.League != "NBA" && m.League != "NFL") || m.SeasonType != 2 || m.Policy != "strictly_prior_utc_dates_v1" || m.MinimumGames != 5 || n == 0 || n > len(featureNames) || len(m.Mean) != n || len(m.Scale) != n || len(m.Coefficients) != n {
		return fmt.Errorf("invalid model schema")
	}
	if _, err := time.Parse("2006-01-02", m.TrainedThrough); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, name := range featureNames {
		known[name] = true
	}
	seen := map[string]bool{}
	for i, name := range m.Features {
		if !known[name] || seen[name] || m.Scale[i] <= 0 || !finite(m.Scale[i]) || !finite(m.Mean[i]) || !finite(m.Coefficients[i]) {
			return fmt.Errorf("invalid model weights")
		}
		seen[name] = true
	}
	if !finite(m.Intercept) {
		return fmt.Errorf("invalid intercept")
	}
	return nil
}
func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func (m Model) Probability(values map[string]float64) float64 {
	z := m.Intercept
	for i, name := range m.Features {
		z += (values[name] - m.Mean[i]) / m.Scale[i] * m.Coefficients[i]
	}
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}
	e := math.Exp(z)
	return e / (1 + e)
}

type observation struct {
	date               time.Time
	forPoints, against int
}

func vector(rows []observation, date time.Time) []float64 {
	result := make([]float64, 7)
	for i, r := range rows {
		win := 0.0
		if r.forPoints > r.against {
			win = 1
		} else if r.forPoints == r.against {
			win = .5
		}
		result[0] += win
		result[1] += float64(r.forPoints)
		result[2] += float64(r.against)
		result[3] += float64(r.forPoints - r.against)
		if i >= len(rows)-5 {
			result[4] += win
			result[5] += float64(r.forPoints - r.against)
		}
	}
	for i := 0; i < 4; i++ {
		result[i] /= float64(len(rows))
	}
	result[4] /= 5
	result[5] /= 5
	result[6] = date.Sub(rows[len(rows)-1].date).Hours() / 24
	return result
}
func utcDate(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Features deliberately ignores today's outcomes, including games completed earlier today.
func Features(target game.Game, history []game.Game) (map[string]float64, int, int) {
	latest := map[string]game.Game{}
	for _, g := range history {
		old, ok := latest[g.ExternalID]
		if !ok || !g.ObservedAt.Before(old.ObservedAt) {
			latest[g.ExternalID] = g
		}
	}
	ordered := make([]game.Game, 0, len(latest))
	for _, g := range latest {
		ordered = append(ordered, g)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].StartsAt.Equal(ordered[j].StartsAt) {
			return ordered[i].ExternalID < ordered[j].ExternalID
		}
		return ordered[i].StartsAt.Before(ordered[j].StartsAt)
	})
	var home, away []observation
	seen := map[string]bool{}
	date := utcDate(target.StartsAt)
	for _, g := range ordered {
		if seen[g.ExternalID] || g.Status != "final" || g.Season != target.Season || g.SeasonType != 2 || g.HomeScore == nil || g.AwayScore == nil || !utcDate(g.StartsAt).Before(date) {
			continue
		}
		seen[g.ExternalID] = true
		row := func(team string) (observation, bool) {
			if g.HomeTeamExternalID == team {
				return observation{utcDate(g.StartsAt), *g.HomeScore, *g.AwayScore}, true
			}
			if g.AwayTeamExternalID == team {
				return observation{utcDate(g.StartsAt), *g.AwayScore, *g.HomeScore}, true
			}
			return observation{}, false
		}
		if r, ok := row(target.HomeTeamExternalID); ok {
			home = append(home, r)
		}
		if r, ok := row(target.AwayTeamExternalID); ok {
			away = append(away, r)
		}
	}
	if len(home) < 5 || len(away) < 5 {
		return nil, len(home), len(away)
	}
	h, a := vector(home, date), vector(away, date)
	values := map[string]float64{}
	for i, name := range featureNames {
		values[name] = h[i] - a[i]
	}
	return values, len(home), len(away)
}
