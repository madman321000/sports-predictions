package dataset

import (
	"encoding/json"
	"testing"
	"time"
)

func uncertainPlayer() Player {
	no := false
	return Player{GameID: "g1", PlayerID: "p1", Category: "general", Starter: &no, Stats: `{"fouls":"0","blocks":"0","points":"0","steals":"0","assists":"0","minutes":"--","rebounds":"0","plusMinus":"0","turnovers":"0","defensiveRebounds":"0","offensiveRebounds":"0","fieldGoalsMade-fieldGoalsAttempted":"0-0","freeThrowsMade-freeThrowsAttempted":"0-0","threePointFieldGoalsMade-threePointFieldGoalsAttempted":"0-0"}`}
}

func TestUncertainParticipation(t *testing.T) {
	s, d := fixture()
	d.Players = []Player{uncertainPlayer()}
	r := Audit(s, d)
	if !r.Ready || len(r.Warnings) != 1 || r.Warnings[0].Code != "uncertain_participation" {
		t.Fatalf("%+v", r)
	}
	for _, name := range []string{"starter", "unknown starter", "points", "missing other stat", "missing key", "malformed zero", "unknown key", "NFL", "other category"} {
		t.Run(name, func(t *testing.T) {
			s, d := fixture()
			p := uncertainPlayer()
			var stats map[string]string
			if err := json.Unmarshal([]byte(p.Stats), &stats); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "starter":
				yes := true
				p.Starter = &yes
			case "unknown starter":
				p.Starter = nil
			case "points":
				stats["points"] = "1"
			case "missing other stat":
				stats["rebounds"] = "--"
			case "malformed zero":
				stats["points"] = "0-0"
			case "missing key":
				delete(stats, "blocks")
			case "unknown key":
				stats["newStat"] = "0"
			case "NFL":
				s.League = "NFL"
			case "other category":
				p.Category = "passing"
			}
			body, err := json.Marshal(stats)
			if err != nil {
				t.Fatal(err)
			}
			p.Stats = string(body)
			d.Players = []Player{p}
			if Audit(s, d).Ready || Participation(s.League, p) != "reported" {
				t.Fatal("uncertain exception masked possible participation or malformed stats")
			}
		})
	}
}

func TestVerifiedPostponements(t *testing.T) {
	pairs := map[int]map[string]string{2025: {"401705090": "401748704", "401705098": "401748705", "401705103": "401754705", "401705104": "401748706", "401705183": "401754706"}, 2026: {"401810384": "401850920", "401810499": "401857824", "401810506": "401858693", "401810507": "401858694"}}
	for season, pairs := range pairs {
		for oldID, finalID := range pairs {
			s, d := fixture()
			s.Season = season
			final := d.Games[0]
			final.ID = finalID
			old := final
			old.ID = oldID
			old.Status = "postponed"
			old.StartsAt = final.StartsAt.Add(-24 * time.Hour)
			old.PlayersComplete = false
			d.Games = []Game{old, final}
			r := Audit(s, d)
			if !r.Ready || r.FinalGames != 1 || len(r.Warnings) != 1 || r.Warnings[0].Detail != oldID+" -> "+finalID {
				t.Fatalf("%+v", r)
			}
			for _, name := range []string{"missing", "wrong team", "not final", "no players", "no score", "earlier", "other season", "other league", "postseason", "unknown old", "scheduled"} {
				t.Run(oldID+"/"+name, func(t *testing.T) {
					scope := s
					data := d
					data.Games = append([]Game(nil), d.Games...)
					switch name {
					case "missing":
						data.Games = data.Games[:1]
					case "wrong team":
						data.Games[1].Home = "other"
					case "not final":
						data.Games[1].Status = "scheduled"
					case "no players":
						data.Games[1].PlayersComplete = false
					case "no score":
						data.Games[1].HomeScore = nil
					case "earlier":
						data.Games[1].StartsAt = old.StartsAt.Add(-time.Hour)
					case "other season":
						scope.Season = 4051 - season
					case "other league":
						scope.League = "NFL"
					case "postseason":
						scope.SeasonType = 3
					case "unknown old":
						data.Games[0].ID = "unverified"
					case "scheduled":
						data.Games[0].Status = "scheduled"
					}
					if Audit(scope, data).Ready {
						t.Fatal("unverified replacement accepted")
					}
				})
			}
		}
	}

}

func TestLakersSpursReplacementIsNotJanuaryRematch(t *testing.T) {
	s, d := fixture()
	s.Season = 2025
	old := d.Games[0]
	old.ID = "401705103"
	old.Status = "postponed"
	old.StartsAt = time.Date(2025, 1, 12, 3, 30, 0, 0, time.UTC)
	rematch := d.Games[0]
	rematch.ID = "401705118"
	rematch.StartsAt = time.Date(2025, 1, 14, 3, 30, 0, 0, time.UTC)
	d.Games = []Game{old, rematch}
	if Audit(s, d).Ready {
		t.Fatal("unrelated January rematch resolved postponement")
	}
	actual := rematch
	actual.ID = "401754705"
	actual.StartsAt = time.Date(2025, 3, 18, 2, 30, 0, 0, time.UTC)
	d.Games = append(d.Games, actual)
	report := Audit(s, d)
	if !report.Ready || len(report.Warnings) != 1 || report.Warnings[0].Detail != "401705103 -> 401754705" {
		t.Fatalf("%+v", report)
	}
}
