package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/madman321000/sports-predictions/internal/player"
)

type playerSummary struct {
	Header struct {
		ID           string
		League       struct{ Abbreviation string }
		Season       struct{ Year, Type int }
		Competitions []struct {
			Status struct {
				Type struct {
					Completed bool
					State     string
				}
			}
		}
	}
	Boxscore struct {
		Players []struct {
			Team       struct{ ID string }
			Statistics []struct {
				Name     string
				Keys     []string
				Athletes []struct {
					Athlete struct {
						ID, DisplayName, Jersey string
						Position                struct{ Abbreviation string }
					}
					DidNotPlay bool
					Starter    *bool
					Stats      []string
				}
			}
		}
	}
}

// FetchPlayerGame uses the shared client gate and validates historical identity
// against the database before accepting a box score.
func (c *Client) FetchPlayerGame(ctx context.Context, league string, game player.GameRef) (player.BoxScore, error) {
	path, err := leaguePath(league)
	if err != nil {
		return player.BoxScore{}, err
	}
	response, err := c.getSnapshot(ctx, path+"/summary?event="+url.QueryEscape(game.ExternalID))
	if err != nil {
		return player.BoxScore{}, err
	}
	result, err := decodePlayerGame(response.body, league, game)
	result.ObservedAt = response.observedAt
	return result, err
}

func decodePlayerGame(body []byte, league string, game player.GameRef) (player.BoxScore, error) {
	var result player.BoxScore
	var data playerSummary
	if err := json.Unmarshal(body, &data); err != nil {
		return result, fmt.Errorf("decode player summary: %w", err)
	}
	h := data.Header
	if h.ID != game.ExternalID || h.League.Abbreviation != league || h.Season.Year != game.Season || h.Season.Type != game.SeasonType {
		return result, fmt.Errorf("player summary game, league or season mismatch")
	}
	if len(h.Competitions) != 1 || !h.Competitions[0].Status.Type.Completed || h.Competitions[0].Status.Type.State != "post" {
		return result, fmt.Errorf("player summary is not final")
	}
	if len(data.Boxscore.Players) != 2 {
		return result, fmt.Errorf("player summary must contain both teams")
	}
	teams, identities := map[string]bool{}, map[string]bool{}
	playerTeams := map[string]string{}
	for _, team := range data.Boxscore.Players {
		if teams[team.Team.ID] || (team.Team.ID != game.HomeTeamExternalID && team.Team.ID != game.AwayTeamExternalID) {
			return result, fmt.Errorf("unexpected box score team")
		}
		teams[team.Team.ID] = true
		played := 0
		categories := map[string]bool{}
		for _, category := range team.Statistics {
			name := category.Name
			if name == "" && league == "NBA" {
				name = "general"
			}
			if name == "" || len(category.Keys) == 0 || categories[name] {
				return result, fmt.Errorf("missing player statistic category or keys")
			}
			categories[name] = true
			keys := map[string]bool{}
			for _, key := range category.Keys {
				if strings.TrimSpace(key) == "" || keys[key] {
					return result, fmt.Errorf("invalid or duplicate statistic key")
				}
				keys[key] = true
			}
			for _, row := range category.Athletes {
				a := row.Athlete
				if a.ID == "" && row.DidNotPlay && len(row.Stats) == 0 {
					result.UnidentifiedDNP++
					continue
				}
				identity := a.ID + "/" + name
				if a.ID == "" || strings.TrimSpace(a.DisplayName) == "" {
					return result, fmt.Errorf("team %s category %s: player missing ID or display name", team.Team.ID, name)
				}
				if identities[identity] {
					return result, fmt.Errorf("duplicate player %s category %s", a.ID, name)
				}
				if prior, ok := playerTeams[a.ID]; ok && prior != team.Team.ID {
					return result, fmt.Errorf("player appears for both teams")
				}
				playerTeams[a.ID] = team.Team.ID
				identities[identity] = true
				if len(row.Stats) != len(category.Keys) && (!row.DidNotPlay || len(row.Stats) != 0) {
					return result, fmt.Errorf("player %s statistic alignment mismatch", a.ID)
				}
				stats := map[string]string{}
				for i, value := range row.Stats {
					stats[category.Keys[i]] = value
				}
				result.Lines = append(result.Lines, player.Line{ExternalID: a.ID, Name: a.DisplayName, Position: a.Position.Abbreviation, Jersey: a.Jersey, TeamExternalID: team.Team.ID, Category: name, DidNotPlay: row.DidNotPlay, Starter: row.Starter, Stats: stats})
				if !row.DidNotPlay && len(row.Stats) > 0 {
					played++
				}
			}
		}
		if league == "NFL" {
			for _, required := range []string{"passing", "rushing", "receiving", "defensive", "kicking", "punting"} {
				if !categories[required] {
					return result, fmt.Errorf("missing NFL player category %s", required)
				}
			}
		}
		if played == 0 {
			return result, fmt.Errorf("missing team player statistics")
		}
	}
	return result, nil
}
