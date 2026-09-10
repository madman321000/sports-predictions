package espn

import (
	"context"
	"net/http"
	"testing"
)

func TestFetchNBATeams(t *testing.T) {
	body := fixture(t)
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/site/v2/sports/basketball/nba/teams" || r.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected URL: %s", r.URL)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing user agent")
		}
		respond(w, body)
	})
	teams, err := c.FetchNBATeams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 2 || teams[0].ExternalID != "1" || teams[0].Name != "Atlanta Hawks" || teams[1].Abbreviation != "BOS" {
		t.Fatalf("unexpected teams: %+v", teams)
	}
}

func TestDecodeTeamsRejectsInvalidData(t *testing.T) {
	for _, body := range []string{
		`not json`, `{}`, `{"sports":[{"leagues":[{"abbreviation":"NBA","teams":[{"team":{"id":"1"}}]}]}]}`,
		`{"sports":[{"leagues":[{"abbreviation":"NBA","teams":[{"team":{"id":"1","displayName":"A","abbreviation":"A"}},{"team":{"id":"1","displayName":"B","abbreviation":"B"}}]}]}]}`,
	} {
		if _, err := decodeTeams([]byte(body)); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
