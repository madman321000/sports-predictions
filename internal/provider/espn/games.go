package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/madman321000/sports-predictions/internal/game"
)

// FetchGames uses ESPN's calendar date selector, which is not a UTC-time filter.
func (c *Client) FetchGames(ctx context.Context, league string, date time.Time) ([]game.Game, error) {
	path, err := leaguePath(league)
	if err != nil {
		return nil, err
	}
	response, err := c.getSnapshot(ctx, path+"/scoreboard?dates="+date.Format("20060102")+"&limit=1000")
	if err != nil {
		return nil, err
	}
	return decodeGamesWithSkips(response.body, league, response.observedAt, c.onSkippedEvent)
}

// FetchGameRange reads at most 31 calendar dates through the shared request gate.
// ESPN's scoreboard rejects multi-date selectors for some leagues; use the same
// single-date endpoint as ingestion and discard partial results on any failure.
func (c *Client) FetchGameRange(ctx context.Context, league string, from, to time.Time) ([]game.Game, error) {
	if to.Before(from) || to.Sub(from) > 30*24*time.Hour {
		return nil, fmt.Errorf("scoreboard range must span 1–31 dates")
	}
	if _, err := leaguePath(league); err != nil {
		return nil, err
	}
	result := []game.Game{}
	positions := map[string]int{}
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		games, err := c.FetchGames(ctx, league, date)
		if err != nil {
			return nil, err
		}
		for _, g := range games {
			if i, ok := positions[g.ExternalID]; ok {
				result[i] = g
			} else {
				positions[g.ExternalID] = len(result)
				result = append(result, g)
			}
		}
	}
	return result, nil
}

type scoreboard struct {
	Leagues []struct {
		Abbreviation string `json:"abbreviation"`
	} `json:"leagues"`
	Events json.RawMessage `json:"events"`
}
type event struct {
	ID     string `json:"id"`
	Season struct {
		Year int `json:"year"`
		Type int `json:"type"`
	} `json:"season"`
	Week *struct {
		Number int `json:"number"`
	} `json:"week"`
	Competitions []competition `json:"competitions"`
}
type competition struct {
	Type struct {
		Abbreviation string `json:"abbreviation"`
	} `json:"type"`
	Date   string `json:"date"`
	Status struct {
		Type struct {
			Name      string `json:"name"`
			State     string `json:"state"`
			Completed bool   `json:"completed"`
		} `json:"type"`
	} `json:"status"`
	Competitors []struct {
		HomeAway string `json:"homeAway"`
		Team     struct {
			ID string `json:"id"`
		} `json:"team"`
		Score *string `json:"score"`
	} `json:"competitors"`
}

func decodeGames(body []byte, league string, observedAt time.Time) ([]game.Game, error) {
	return decodeGamesWithSkips(body, league, observedAt, nil)
}

func decodeGamesWithSkips(body []byte, league string, observedAt time.Time, onSkip func(string, string)) ([]game.Game, error) {
	var response scoreboard
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode scoreboard: %w", err)
	}
	found := false
	for _, l := range response.Leagues {
		if l.Abbreviation == league {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("scoreboard does not identify league %s", league)
	}
	if len(response.Events) == 0 || string(response.Events) == "null" {
		return nil, fmt.Errorf("scoreboard missing events array")
	}
	var events []event
	if err := json.Unmarshal(response.Events, &events); err != nil {
		return nil, fmt.Errorf("decode events: %w", err)
	}
	if len(events) >= 1000 {
		return nil, fmt.Errorf("scoreboard date may be truncated")
	}
	result := make([]game.Game, 0, len(events))
	seen := make(map[string]bool)
	for _, e := range events {
		if e.ID == "" || seen[e.ID] || len(e.Competitions) != 1 || e.Season.Year <= 0 || e.Season.Type <= 0 {
			return nil, fmt.Errorf("invalid or duplicate ESPN event %q", e.ID)
		}
		seen[e.ID] = true
		c := e.Competitions[0]
		// ESPN labels All-Star events as regular season, so season.type alone
		// cannot distinguish exhibitions from league games.
		reason := ""
		if e.Season.Type == 1 {
			reason = "preseason"
		}
		if c.Type.Abbreviation == "ALLSTAR" {
			reason = "All-Star exhibition"
		}
		if reason != "" {
			if onSkip != nil {
				onSkip(e.ID, reason)
			}
			continue
		}

		start, err := time.Parse(time.RFC3339, c.Date)
		if err != nil {
			start, err = time.Parse("2006-01-02T15:04Z07:00", c.Date)
		}
		if err != nil {
			return nil, fmt.Errorf("event %s start time: %w", e.ID, err)
		}
		status, err := gameStatus(c.Status.Type.Name, c.Status.Type.State, c.Status.Type.Completed)
		if err != nil {
			return nil, fmt.Errorf("event %s: %w", e.ID, err)
		}
		g := game.Game{ExternalID: e.ID, StartsAt: start.UTC(), Status: status, Season: e.Season.Year, SeasonType: e.Season.Type, ObservedAt: observedAt}
		if league == "NFL" && e.Week != nil {
			if e.Week.Number <= 0 {
				return nil, fmt.Errorf("event %s invalid week", e.ID)
			}
			g.Week = &e.Week.Number
		}
		if len(c.Competitors) != 2 {
			return nil, fmt.Errorf("event %s must have two competitors", e.ID)
		}
		for _, competitor := range c.Competitors {
			if competitor.Team.ID == "" {
				return nil, fmt.Errorf("event %s missing team ID", e.ID)
			}
			var score *int
			if status == "in_progress" || status == "final" || status == "suspended" {
				if competitor.Score != nil {
					parsed, err := strconv.Atoi(*competitor.Score)
					if err != nil || parsed < 0 {
						return nil, fmt.Errorf("event %s invalid score", e.ID)
					}
					score = &parsed
				}
				if status == "final" && score == nil {
					return nil, fmt.Errorf("event %s final score missing", e.ID)
				}
			}
			switch competitor.HomeAway {
			case "home":
				if g.HomeTeamExternalID != "" {
					return nil, fmt.Errorf("event %s duplicate home team", e.ID)
				}
				g.HomeTeamExternalID = competitor.Team.ID
				g.HomeScore = score
			case "away":
				if g.AwayTeamExternalID != "" {
					return nil, fmt.Errorf("event %s duplicate away team", e.ID)
				}
				g.AwayTeamExternalID = competitor.Team.ID
				g.AwayScore = score
			default:
				return nil, fmt.Errorf("event %s invalid home/away designation", e.ID)
			}
		}
		if g.HomeTeamExternalID == g.AwayTeamExternalID {
			return nil, fmt.Errorf("event %s teams must differ", e.ID)
		}
		result = append(result, g)
	}
	return result, nil
}
