package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/madman321000/sports-predictions/internal/team"
)

// FetchTeams fetches the team list once; no per-team requests or polling.
func (c *Client) FetchTeams(ctx context.Context, league string) ([]team.Team, error) {
	path, err := leaguePath(league)
	if err != nil {
		return nil, err
	}
	body, err := c.get(ctx, path+"/teams?limit=100")
	if err != nil {
		return nil, err
	}
	return decodeTeams(body, league)
}

func decodeTeams(body []byte, abbreviation string) ([]team.Team, error) {
	var response struct {
		Sports []struct {
			Leagues []struct {
				Abbreviation string `json:"abbreviation"`
				Teams        []struct {
					Team struct {
						ID           string `json:"id"`
						Name         string `json:"displayName"`
						Abbreviation string `json:"abbreviation"`
					} `json:"team"`
				} `json:"teams"`
			} `json:"leagues"`
		} `json:"sports"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode ESPN teams: %w", err)
	}
	var result []team.Team
	seen := make(map[string]bool)
	for _, sport := range response.Sports {
		for _, league := range sport.Leagues {
			if league.Abbreviation != abbreviation {
				continue
			}
			for _, entry := range league.Teams {
				t := entry.Team
				if strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.Abbreviation) == "" {
					return nil, fmt.Errorf("ESPN team is missing required fields")
				}
				if seen[t.ID] {
					return nil, fmt.Errorf("duplicate ESPN team ID %q", t.ID)
				}
				seen[t.ID] = true
				result = append(result, team.Team{ExternalID: t.ID, Name: t.Name, Abbreviation: t.Abbreviation})
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("ESPN returned no %s teams", abbreviation)
	}
	return result, nil
}
