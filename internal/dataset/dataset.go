// Package dataset audits and exports stored observations. It does not fetch data
// or compute predictive features.
package dataset

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Scope struct {
	League     string `json:"league"`
	Season     int    `json:"season"`
	SeasonType int    `json:"season_type"`
	From       string `json:"expected_import_from,omitempty"`
	To         string `json:"expected_import_to,omitempty"`
}

func (s Scope) Validate() error {
	if s.League != "NBA" && s.League != "NFL" {
		return fmt.Errorf("league must be NBA or NFL")
	}
	if s.Season < 1900 || s.Season > 2200 {
		return fmt.Errorf("explicit season between 1900 and 2200 required")
	}
	if s.SeasonType != 2 && s.SeasonType != 3 {
		return fmt.Errorf("season-type must be 2 or 3")
	}
	if s.From != "" || s.To != "" {
		from, e1 := time.Parse(time.DateOnly, s.From)
		to, e2 := time.Parse(time.DateOnly, s.To)
		if e1 != nil || e2 != nil || to.Before(from) || to.Sub(from) > 366*24*time.Hour {
			return fmt.Errorf("from/to must be ascending YYYY-MM-DD dates, at most 367 days inclusive")
		}
	}
	return nil
}

type Game struct {
	ID                   string
	StartsAt             time.Time
	Home, Away, Status   string
	HomeScore, AwayScore *int
	PlayersComplete      bool
}
type Player struct {
	GameID, PlayerID, Name, Team, Category, Position, Jersey string
	DidNotPlay                                               bool
	Starter                                                  *bool
	Stats                                                    string
}
type Snapshot struct {
	Games          []Game
	Players        []Player
	ImportDates    []string
	MissingGameIDs []string
}
type Issue struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}
type Report struct {
	SchemaVersion       int       `json:"schema_version"`
	Scope               Scope     `json:"scope"`
	GeneratedAt         time.Time `json:"generated_at"`
	StoredGames         int       `json:"stored_games"`
	FinalGames          int       `json:"final_games"`
	CompletePlayerGames int       `json:"complete_player_games"`
	PlayerRows          int       `json:"player_category_rows"`
	MissingImportDates  []string  `json:"missing_import_dates"`
	Issues              []Issue   `json:"issues"`
	Ready               bool      `json:"ready_for_export"`
	Limitations         []string  `json:"limitations"`
}

func Audit(s Scope, data Snapshot) Report {
	r := Report{SchemaVersion: 1, Scope: s, GeneratedAt: time.Now().UTC(), StoredGames: len(data.Games), PlayerRows: len(data.Players), MissingImportDates: []string{}, Issues: []Issue{}, Limitations: []string{"Checks stored records only; cannot prove ESPN returned every scheduled game.", "Player history contains box-score participants, not complete rosters.", "Export contains outcomes and post-game statistics; use only earlier games when building predictive features."}}
	if len(data.Games) == 0 {
		r.Issues = append(r.Issues, Issue{"no_games", "No games stored for the requested season and season type."})
	}
	for _, id := range data.MissingGameIDs {
		r.Issues = append(r.Issues, Issue{"missing_game_row", id + ": present in an expected date import but absent from games"})
	}
	for _, g := range data.Games {
		if g.Status != "final" {
			r.Issues = append(r.Issues, Issue{"non_final_game", g.ID + ": " + g.Status})
			continue
		}
		r.FinalGames++
		if g.HomeScore == nil || g.AwayScore == nil {
			r.Issues = append(r.Issues, Issue{"missing_score", g.ID})
		}
		if g.PlayersComplete {
			r.CompletePlayerGames++
		} else {
			r.Issues = append(r.Issues, Issue{"missing_player_import", g.ID})
		}
	}

	for _, p := range data.Players {
		if p.DidNotPlay {
			continue
		}
		var stats map[string]string
		if err := json.Unmarshal([]byte(p.Stats), &stats); err != nil || len(stats) == 0 {
			r.Issues = append(r.Issues, Issue{"invalid_player_stats", p.GameID + "/" + p.PlayerID + "/" + p.Category})
			continue
		}
		for _, value := range stats {
			if strings.TrimSpace(value) == "" || value == "--" || value == "NaN" || value == "Infinity" {
				r.Issues = append(r.Issues, Issue{"missing_player_stat_value", p.GameID + "/" + p.PlayerID + "/" + p.Category})
				break
			}
		}
	}
	if s.From == "" {
		r.Issues = append(r.Issues, Issue{"unchecked_dates", "Supply -from and -to to check expected ESPN import dates."})
	} else {
		dates := map[string]bool{}
		for _, d := range data.ImportDates {
			dates[d] = true
		}
		from, _ := time.Parse(time.DateOnly, s.From)
		to, _ := time.Parse(time.DateOnly, s.To)
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			key := d.Format(time.DateOnly)
			if !dates[key] {
				r.MissingImportDates = append(r.MissingImportDates, key)
			}
		}
		if len(r.MissingImportDates) > 0 {
			r.Issues = append(r.Issues, Issue{"missing_import_dates", fmt.Sprintf("%d expected dates have no successful scoreboard import", len(r.MissingImportDates))})
		}
	}
	r.Ready = len(r.Issues) == 0
	return r
}
