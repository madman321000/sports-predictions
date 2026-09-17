package dataset

import (
	"encoding/json"
	"strings"
)

// These ESPN IDs were reconciled against NBA rescheduling announcements and
// stored final matchups. Never infer replacement games from matching teams alone.
// Sources:
// https://pr.nba.com/heat-bulls-schedule-adjustments/
// https://www.nba.com/news/warriors-timberwolves-game-postponed
// https://www.nba.com/news/nba-schedule-adjustments-weather-2026
var nbaReplacements = map[int]map[string]string{
	2025: {
		// https://pr.nba.com/nba-game-schedule-adjustments-1-15-25/
		"401705090": "401748704",
		"401705098": "401748705",
		"401705104": "401748706",
		// https://www.nba.com/spurs/news/san-antonio-spurs-announce-schedule-changes
		"401705103": "401754705",
		// https://www.nba.com/pelicans/news/pelicans-tickets-information-nba-schedule-adjustments-smoothie-king-center-milwaukee-bucks-orlando-magic
		"401705183": "401754706",
	},
	2026: {
		"401810384": "401850920",
		"401810499": "401857824",
		"401810506": "401858693",
		"401810507": "401858694",
	},
}

func replacement(s Scope, old Game, games []Game) string {
	if s.League != "NBA" || s.SeasonType != 2 || old.Status != "postponed" {
		return ""
	}
	id := nbaReplacements[s.Season][old.ID]
	if id == "" {
		return ""
	}
	for _, g := range games {
		if g.ID == id && g.Status == "final" && g.Home == old.Home && g.Away == old.Away && g.StartsAt.After(old.StartsAt) && g.HomeScore != nil && g.AwayScore != nil && g.PlayersComplete {
			return id
		}
	}
	return ""
}

// Participation preserves the distinction between an explicit DNP and a row
// whose zero statistics and unavailable minutes do not establish an appearance.
func Participation(league string, p Player) string {
	if p.DidNotPlay {
		return "did_not_play"
	}
	if league != "NBA" || p.Category != "general" || p.Starter == nil || *p.Starter {
		return "reported"
	}
	var stats map[string]string
	if json.Unmarshal([]byte(p.Stats), &stats) != nil || stats["minutes"] != "--" {
		return "reported"
	}
	keys := []string{"fouls", "blocks", "points", "steals", "assists", "rebounds", "plusMinus", "turnovers", "defensiveRebounds", "offensiveRebounds", "fieldGoalsMade-fieldGoalsAttempted", "freeThrowsMade-freeThrowsAttempted", "threePointFieldGoalsMade-threePointFieldGoalsAttempted"}
	if len(stats) != len(keys)+1 {
		return "reported"
	}
	for _, key := range keys {
		value, ok := stats[key]
		expected := "0"
		if strings.Contains(key, "-") {
			expected = "0-0"
		}
		if !ok || value != expected {
			return "reported"
		}
	}
	return "uncertain"
}
