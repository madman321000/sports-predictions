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
	league string
	err    error
}

func (s *source) FetchTeams(_ context.Context, league string) ([]team.Team, error) {
	s.called = true
	s.league = league
	return []team.Team{{ExternalID: "1", Name: "Hawks", Abbreviation: "ATL"}}, s.err
}

type store struct {
	lookupErr, writeErr error
	complete            bool
	checkErr            error
	saved               []team.Team
}

func (s *store) LeagueID(context.Context, string) (int64, error) { return 1, s.lookupErr }
func (s *store) SaveTeamImport(_ context.Context, _ int64, _ string, teams []team.Team) error {
	s.saved = teams
	return s.writeErr
}

func TestIngestNBATeams(t *testing.T) {
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
			count, err := ingest.IngestTeams(context.Background(), "NBA", src, db, false)
			if stage == "success" {
				if err != nil || count.TeamsProcessed != 1 || len(db.saved) != 1 {
					t.Fatalf("result=%+v, error=%v", count, err)
				}
				return
			}
			if !errors.Is(err, failure) || count.TeamsProcessed != 0 {
				t.Fatalf("result=%+v, error=%v", count, err)
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

func TestIngestNFLTeams(t *testing.T) {
	src := &source{}
	db := &store{}
	count, err := ingest.IngestTeams(context.Background(), "NFL", src, db, false)
	if err != nil || count.TeamsProcessed != 1 || len(db.saved) != 1 || src.league != "NFL" {
		t.Fatalf("NFL result=%+v error=%v", count, err)
	}
}

func (s *store) TeamsImported(context.Context, int64, string) (bool, error) {
	return s.complete, s.checkErr
}

func TestTeamsSkipCachedImportsAndForceRefresh(t *testing.T) {
	for _, league := range []string{"NBA", "NFL"} {
		t.Run(league, func(t *testing.T) {
			src := &source{}
			db := &store{complete: true}
			result, err := ingest.IngestTeams(context.Background(), league, src, db, false)
			if err != nil || !result.Skipped || src.called || len(db.saved) != 0 {
				t.Fatalf("cached result=%+v error=%v called=%v", result, err, src.called)
			}
			result, err = ingest.IngestTeams(context.Background(), league, src, db, true)
			if err != nil || result.Skipped || !src.called || result.TeamsProcessed != 1 {
				t.Fatalf("force result=%+v error=%v", result, err)
			}
		})
	}
}

func TestTeamsCacheErrorDoesNotCallProvider(t *testing.T) {
	failure := errors.New("cache read failed")
	src := &source{}
	db := &store{checkErr: failure}
	_, err := ingest.IngestTeams(context.Background(), "NFL", src, db, false)
	if !errors.Is(err, failure) || src.called {
		t.Fatalf("error=%v called=%v", err, src.called)
	}
}
