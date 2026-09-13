package dataset

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func fixture() (Scope, Snapshot) {
	zero := 0
	return Scope{League: "NBA", Season: 2026, SeasonType: 2, From: "2026-01-01", To: "2026-01-02"}, Snapshot{Games: []Game{{ID: "g1", StartsAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), Home: "1", Away: "2", Status: "final", HomeScore: &zero, AwayScore: &zero, PlayersComplete: true}}, Players: []Player{{GameID: "g1", PlayerID: "p1", Name: "Name, quoted \"player\"", Team: "1", Category: "general", Stats: `{"points":"0"}`}}, ImportDates: []string{"2026-01-01", "2026-01-02"}}
}
func TestAudit(t *testing.T) {
	s, d := fixture()
	r := Audit(s, d)
	if !r.Ready || r.FinalGames != 1 || r.CompletePlayerGames != 1 {
		t.Fatalf("%+v", r)
	}
	cases := map[string]struct {
		mutate func(*Snapshot)
		code   string
	}{
		"no games":        {func(d *Snapshot) { d.Games = nil }, "no_games"},
		"missing date":    {func(d *Snapshot) { d.ImportDates = d.ImportDates[:1] }, "missing_import_dates"},
		"missing players": {func(d *Snapshot) { d.Games[0].PlayersComplete = false }, "missing_player_import"},
		"missing score":   {func(d *Snapshot) { d.Games[0].HomeScore = nil }, "missing_score"},
		"scheduled":       {func(d *Snapshot) { d.Games[0].Status = "scheduled" }, "non_final_game"},
		"invalid JSON":    {func(d *Snapshot) { d.Players[0].Stats = "{" }, "invalid_player_stats"},
		"missing metric":  {func(d *Snapshot) { d.Players[0].Stats = `{"points":"--"}` }, "missing_player_stat_value"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s, d := fixture()
			c.mutate(&d)
			r := Audit(s, d)
			found := false
			for _, i := range r.Issues {
				found = found || i.Code == c.code
			}
			if r.Ready || !found {
				t.Fatalf("%+v", r)
			}
		})
	}
	s.From = ""
	s.To = ""
	if Audit(s, d).Ready {
		t.Fatal("unchecked date coverage passed")
	}
	s, d = fixture()
	d.Players[0].DidNotPlay = true
	d.Players[0].Stats = "{}"
	if !Audit(s, d).Ready {
		t.Fatal("DNP requires stats")
	}
}
func TestExport(t *testing.T) {
	s, d := fixture()
	path := filepath.Join(t.TempDir(), "export")
	if _, err := Export(path, s, d, ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(path, "players.csv"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(f).ReadAll()
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1][2] != d.Players[0].Name || rows[1][8] != "" || rows[1][9] != d.Players[0].Stats {
		t.Fatalf("round trip %v", rows)
	}
	before, err := os.ReadFile(filepath.Join(path, "games.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Export(path, s, d, ExportOptions{}); err == nil {
		t.Fatal("overwrote existing directory")
	}
	after, err := os.ReadFile(filepath.Join(path, "games.csv"))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("existing export changed")
	}
	d.Games[0].PlayersComplete = false
	incomplete := filepath.Join(t.TempDir(), "partial")
	if _, err := Export(incomplete, s, d, ExportOptions{}); err == nil {
		t.Fatal("incomplete export accepted")
	}
	if _, err := os.Stat(incomplete); !os.IsNotExist(err) {
		t.Fatal("failed export left output")
	}
	r, err := Export(incomplete, s, d, ExportOptions{AllowIncomplete: true})
	if err != nil || r.Ready {
		t.Fatalf("explicit incomplete %+v %v", r, err)
	}
}
func TestScopeValidation(t *testing.T) {
	for _, s := range []Scope{{League: "MLB", Season: 2026, SeasonType: 2}, {League: "NBA", SeasonType: 2}, {League: "NBA", Season: 2026, SeasonType: 1}, {League: "NBA", Season: 2026, SeasonType: 2, From: "2026-02-30", To: "2026-03-01"}, {League: "NBA", Season: 2026, SeasonType: 2, From: "2026-02-01"}} {
		if s.Validate() == nil {
			t.Fatalf("accepted %+v", s)
		}
	}
}

func TestUnavailableAdjustedQBRIsWarning(t *testing.T) {
	s, d := fixture()
	s.League = "NFL"
	d.Players[0].Category = "passing"
	d.Players[0].Stats = `{"adjQBR":"--","passingYards":"0","completions/passingAttempts":"0/0"}`
	r := Audit(s, d)
	if !r.Ready || len(r.Warnings) != 1 || len(r.Issues) != 0 {
		t.Fatalf("%+v", r)
	}
	out := filepath.Join(t.TempDir(), "export")
	if _, err := Export(out, s, d, ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(out, "players.csv"))
	if err != nil || len(content) == 0 {
		t.Fatalf("CSV %v", err)
	}
	d.Players[0].Stats = `{"adjQBR":"--","passingYards":"--"}`
	r = Audit(s, d)
	if r.Ready || len(r.Warnings) != 1 || len(r.Issues) != 1 || r.Issues[0].Detail != "g1/p1/passing/passingYards" {
		t.Fatalf("masked core metric: %+v", r)
	}
	s.League = "NBA"
	d.Players[0].Stats = `{"adjQBR":"--"}`
	if Audit(s, d).Ready {
		t.Fatal("exception leaked to NBA")
	}
}
