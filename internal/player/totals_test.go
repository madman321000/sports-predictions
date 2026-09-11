package player

import (
	"reflect"
	"testing"
)

func TestAdditiveStats(t *testing.T) {
	line := Line{Stats: map[string]string{"points": "0", "plusMinus": "-14", "fieldGoalsMade-fieldGoalsAttempted": "3-8", "completions/passingAttempts": "17/32", "sacks-sackYardsLost": "1-8", "yardsPerPassAttempt": "5.2", "QBRating": "99.3", "longReception": "50", "assists": "--", "rebounds": "NaN"}}
	want := map[string]float64{"points": 0, "plusMinus": -14, "fieldGoalsMade": 3, "fieldGoalsAttempted": 8, "completions": 17, "passingAttempts": 32, "sacksTaken": 1, "sackYardsLost": 8}
	if got := AdditiveStats(line); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	line.DidNotPlay = true
	if got := AdditiveStats(line); len(got) != 0 {
		t.Fatalf("DNP contributes totals: %v", got)
	}
}
