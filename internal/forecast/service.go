package forecast

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/madman321000/sports-predictions/internal/game"
	"github.com/madman321000/sports-predictions/internal/team"
)

type Provider interface {
	FetchGameRange(context.Context, string, time.Time, time.Time) ([]game.Game, error)
	FetchTeams(context.Context, string) ([]team.Team, error)
}
type Prediction struct {
	ModelID          string    `json:"model_id"`
	HomeProbability  float64   `json:"home_probability"`
	AwayProbability  float64   `json:"away_probability"`
	GeneratedAt      time.Time `json:"generated_at"`
	HomeHistoryGames int       `json:"home_history_games"`
	AwayHistoryGames int       `json:"away_history_games"`
}
type Match struct {
	ID          string      `json:"id"`
	StartsAt    time.Time   `json:"starts_at"`
	Status      string      `json:"status"`
	HomeTeam    string      `json:"home_team"`
	AwayTeam    string      `json:"away_team"`
	HomeScore   *int        `json:"home_score"`
	AwayScore   *int        `json:"away_score"`
	Prediction  *Prediction `json:"prediction"`
	Unavailable string      `json:"unavailable,omitempty"`
}
type Today struct {
	League        string    `json:"league"`
	Date          string    `json:"date"`
	Timezone      string    `json:"timezone"`
	UpdatedAt     time.Time `json:"updated_at"`
	ModelID       string    `json:"model_id,omitempty"`
	HistoryStatus string    `json:"history_status"`
	Games         []Match   `json:"games"`
}
type savedPrediction struct {
	start      time.Time
	prediction Prediction
}
type leagueState struct {
	mu                    sync.Mutex
	games                 []game.Game
	scheduleAt, timeTeams time.Time
	names                 map[string]string
	scheduleError         bool
	history               []game.Game
	season                int
	historyAt, retryAt    time.Time
	loading, historyError bool
	saved                 map[string]savedPrediction
}
type Service struct {
	provider Provider
	models   map[string]Model
	states   map[string]*leagueState
	ctx      context.Context
	now      func() time.Time
}

func NewService(ctx context.Context, p Provider, models map[string]Model) *Service {
	return &Service{provider: p, models: models, ctx: ctx, now: time.Now, states: map[string]*leagueState{"NBA": {names: map[string]string{}, saved: map[string]savedPrediction{}}, "NFL": {names: map[string]string{}, saved: map[string]savedPrediction{}}}}
}

// Today shares provider caches across visitors and timezones. No client can request
// arbitrary historical ranges or trigger parallel season backfills.
func (s *Service) Today(ctx context.Context, league string, zone *time.Location) (Today, error) {
	state, ok := s.states[league]
	if !ok {
		return Today{}, fmt.Errorf("unsupported league")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Today{}, err
	}
	// Shared cache refreshes survive a visitor navigating away.
	providerCtx, cancel := context.WithTimeout(s.ctx, 70*time.Second)
	defer cancel()
	now := s.now().UTC()

	// Refresh at most once per five minutes, including after provider errors.
	if state.scheduleAt.IsZero() || now.Sub(state.scheduleAt) >= 5*time.Minute {
		state.scheduleAt = now
		games, err := s.provider.FetchGameRange(providerCtx, league, utcDate(now).AddDate(0, 0, -1), utcDate(now).AddDate(0, 0, 2))
		state.scheduleError = err != nil
		if err == nil {
			state.games = games
		}
	}
	if state.scheduleError {
		return Today{}, fmt.Errorf("schedule unavailable")
	}
	if len(state.names) == 0 || now.Sub(state.timeTeams) >= 24*time.Hour {
		// Failed team metadata is retried on the same conservative schedule.
		if state.timeTeams.IsZero() || now.Sub(state.timeTeams) >= 5*time.Minute {
			state.timeTeams = now
			teams, err := s.provider.FetchTeams(providerCtx, league)
			if err == nil {
				for _, t := range teams {
					state.names[t.ExternalID] = t.Name
				}
			}
		}
	}
	now = s.now().UTC() // Provider calls can cross tipoff or local midnight.
	local := now.In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	end := start.AddDate(0, 0, 1)
	result := Today{League: league, Date: start.Format("2006-01-02"), Timezone: zone.String(), Games: []Match{}, HistoryStatus: "not_needed"}
	result.UpdatedAt = state.scheduleAt
	model, hasModel := s.models[league]
	if hasModel {
		result.ModelID = model.ID
	}
	season := 0
	for _, g := range state.games {
		if !g.StartsAt.Before(start) && g.StartsAt.Before(end) && g.Status == "scheduled" && now.Before(g.StartsAt) && g.SeasonType == 2 && g.Season >= now.Year()-1 && g.Season <= now.Year()+1 {
			season = g.Season
			break
		}
	}
	if season != 0 && hasModel {
		if state.season != season && !state.loading {
			state.season = season
			state.history = nil
			state.historyAt = time.Time{}
			state.retryAt = time.Time{}
		}
		if !state.loading && (state.historyAt.IsZero() || now.Sub(state.historyAt) >= 5*time.Minute) && !now.Before(state.retryAt) {
			state.loading = true
			state.retryAt = now.Add(5 * time.Minute)
			history := append([]game.Game(nil), state.history...)
			previous := state.historyAt
			go s.refresh(league, season, history, previous)
		}
		result.HistoryStatus = "ready"
		if state.historyAt.IsZero() {
			result.HistoryStatus = "warming"
		} else if now.Sub(state.historyAt) > 15*time.Minute {
			result.HistoryStatus = "stale"
		}
		if state.historyError {
			result.HistoryStatus = "unavailable"
		}
	}
	for key, p := range state.saved {
		if now.Sub(p.start) > 72*time.Hour {
			delete(state.saved, key)
		}
	}
	for _, g := range state.games {
		if g.StartsAt.Before(start) || !g.StartsAt.Before(end) {
			continue
		}
		name := func(id string) string {
			if n := state.names[id]; n != "" {
				return n
			}
			return "Team " + id
		}
		match := Match{ID: g.ExternalID, StartsAt: g.StartsAt, Status: g.Status, HomeTeam: name(g.HomeTeamExternalID), AwayTeam: name(g.AwayTeamExternalID), HomeScore: g.HomeScore, AwayScore: g.AwayScore}
		key := model.ID + ":" + g.ExternalID
		if !hasModel {
			match.Unavailable = "No locally trained model has been installed for this league."
		} else if g.SeasonType != 2 {
			match.Unavailable = "This model supports regular-season games only."
		} else if g.Status != "scheduled" || !now.Before(g.StartsAt) {
			if saved, ok := state.saved[key]; ok && saved.start.Equal(g.StartsAt) && (g.Status == "in_progress" || g.Status == "final" || g.Status == "scheduled") {
				p := saved.prediction
				match.Prediction = &p
			} else {
				match.Unavailable = "No pregame prediction recorded; predictions are not generated after tipoff or kickoff."
			}
		} else if utcDate(g.StartsAt).Format("2006-01-02") <= model.TrainedThrough {
			match.Unavailable = "Model training overlaps this game date."
		} else if result.HistoryStatus != "ready" {
			match.Unavailable = "Waiting for complete, current team history."
		} else {
			history := append(append([]game.Game(nil), state.history...), state.games...)
			values, home, away := Features(g, history)
			if values == nil {
				match.Unavailable = "At least five completed games per team in this season are required."
			} else {
				p := Prediction{ModelID: model.ID, HomeProbability: model.Probability(values), GeneratedAt: now, HomeHistoryGames: home, AwayHistoryGames: away}
				p.AwayProbability = 1 - p.HomeProbability
				state.saved[key] = savedPrediction{start: g.StartsAt, prediction: p}
				match.Prediction = &p
			}
		}
		result.Games = append(result.Games, match)
	}
	sort.Slice(result.Games, func(i, j int) bool {
		if result.Games[i].StartsAt.Equal(result.Games[j].StartsAt) {
			return result.Games[i].ID < result.Games[j].ID
		}
		return result.Games[i].StartsAt.Before(result.Games[j].StartsAt)
	})
	return result, nil
}

func (s *Service) refresh(league string, season int, previous []game.Game, previousAt time.Time) {
	ctx, cancel := context.WithTimeout(s.ctx, 4*time.Minute)
	defer cancel()
	now := s.now().UTC()
	year := season
	if league == "NBA" {
		year--
	}
	from := time.Date(year, time.July, 1, 0, 0, 0, 0, time.UTC)
	if !previousAt.IsZero() {
		from = utcDate(previousAt).AddDate(0, 0, -2)
	}
	to := utcDate(now)
	history := map[string]game.Game{}
	for _, g := range previous {
		history[g.ExternalID] = g
	}
	var fetchErr error
	for date := from; !date.After(to); {
		until := date.AddDate(0, 0, 30)
		if until.After(to) {
			until = to
		}
		games, err := s.provider.FetchGameRange(ctx, league, date, until)
		if err != nil {
			fetchErr = err
			break
		}
		for _, g := range games {
			if g.Season == season && g.SeasonType == 2 {
				history[g.ExternalID] = g
			}
		}
		date = until.AddDate(0, 0, 1)
	}
	state := s.states[league]
	state.mu.Lock()
	defer state.mu.Unlock()
	state.loading = false
	state.historyError = fetchErr != nil
	if fetchErr == nil && state.season == season {
		state.history = make([]game.Game, 0, len(history))
		for _, g := range history {
			state.history = append(state.history, g)
		}
		state.historyAt = now
	}
}
