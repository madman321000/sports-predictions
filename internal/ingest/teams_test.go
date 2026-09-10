package ingest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/madman321000/sports-predictions/internal/ingest"
	"github.com/madman321000/sports-predictions/internal/team"
)

type source struct {
	called bool
	err    error
}

func (s *source) NBA(context.Context) ([]team.Team, error) {
	s.called = true
	return []team.Team{{ExternalID: "1", Name: "Hawks", Abbreviation: "ATL"}}, s.err
}

type store struct {
	lookupErr, writeErr error
	saved               []team.Team
}

func (s *store) LeagueID(context.Context, string) (int64, error) { return 1, s.lookupErr }
func (s *store) UpsertTeams(_ context.Context, _ int64, _ string, teams []team.Team) error {
	s.saved = teams
	return s.writeErr
}

func TestNBATeams(t *testing.T) {
	failure := errors.New("failure")
	for _, stage := range []string{"success", "lookup", "fetch", "write"} {
		t.Run(stage, func(t *testing.T) {
			src := &source{}
			db := &store{}
			switch stage {
			case "lookup":
				db.lookupErr = failure
			case "fetch":
				src.err = failure
			case "write":
				db.writeErr = failure
			}
			count, err := ingest.NBATeams(context.Background(), src, db)
			if stage == "success" {
				if err != nil || count != 1 || len(db.saved) != 1 {
					t.Fatalf("count=%d, error=%v", count, err)
				}
				return
			}
			if !errors.Is(err, failure) || count != 0 {
				t.Fatalf("count=%d, error=%v", count, err)
			}
			if stage == "lookup" && src.called {
				t.Fatal("called provider before prerequisites passed")
			}
			if (stage == "lookup" || stage == "fetch") && len(db.saved) != 0 {
				t.Fatal("wrote after failed prerequisite")
			}
		})
	}
}
