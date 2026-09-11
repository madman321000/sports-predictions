package player

import (
	"math"
	"strconv"
	"strings"
)

// AdditiveStats exposes only quantities that can be summed across games.
// Provider ratios, percentages, ratings and longest plays remain in raw Stats.
// Missing values are omitted, never treated as zero.
func AdditiveStats(line Line) map[string]float64 {
	result := map[string]float64{}
	if line.DidNotPlay {
		return result
	}
	allowed := map[string]bool{}
	for _, key := range strings.Fields("points rebounds assists turnovers steals blocks offensiveRebounds defensiveRebounds fouls plusMinus passingYards passingTouchdowns interceptions rushingAttempts rushingYards rushingTouchdowns receptions receivingYards receivingTouchdowns receivingTargets fumbles fumblesLost fumblesRecovered totalTackles soloTackles sacks tacklesForLoss passesDefended QBHits defensiveTouchdowns interceptionYards interceptionTouchdowns kickReturns kickReturnYards kickReturnTouchdowns puntReturns puntReturnYards puntReturnTouchdowns totalKickingPoints punts puntYards touchbacks puntsInside20") {
		allowed[key] = true
	}
	pairs := map[string][2]string{
		"fieldGoalsMade-fieldGoalsAttempted":                     {"fieldGoalsMade", "fieldGoalsAttempted"},
		"threePointFieldGoalsMade-threePointFieldGoalsAttempted": {"threePointFieldGoalsMade", "threePointFieldGoalsAttempted"},
		"freeThrowsMade-freeThrowsAttempted":                     {"freeThrowsMade", "freeThrowsAttempted"},
		"completions/passingAttempts":                            {"completions", "passingAttempts"},
		"sacks-sackYardsLost":                                    {"sacksTaken", "sackYardsLost"},
		"fieldGoalsMade/fieldGoalAttempts":                       {"fieldGoalsMade", "fieldGoalAttempts"},
		"extraPointsMade/extraPointAttempts":                     {"extraPointsMade", "extraPointAttempts"},
	}
	for key, value := range line.Stats {
		if names, ok := pairs[key]; ok {
			separator := "-"
			if strings.Contains(key, "/") {
				separator = "/"
			}
			parts := strings.Split(value, separator)
			if len(parts) == 2 {
				a, e1 := strconv.ParseFloat(parts[0], 64)
				b, e2 := strconv.ParseFloat(parts[1], 64)
				if e1 == nil && e2 == nil && a >= 0 && b >= 0 && !math.IsInf(a, 0) && !math.IsInf(b, 0) {
					result[names[0]] = a
					result[names[1]] = b
				}
			}
			continue
		}
		if allowed[key] {
			if v, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) {
				result[key] = v
			}
		}
	}
	return result
}
