package espn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/teams.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	c, err := NewClient(MinInterval)
	if err != nil {
		t.Fatal(err)
	}
	c.baseURL = s.URL
	c.interval = time.Millisecond
	return c
}
func respond(w http.ResponseWriter, body []byte) { _, _ = w.Write(body) }

func TestNBARejectsRedirect(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Redirect(w, r, "/other", http.StatusFound)
	})
	if _, err := c.FetchNBATeams(context.Background()); err == nil {
		t.Fatal("expected redirect error")
	}
	if calls.Load() != 1 {
		t.Fatal("followed unpaced redirect")
	}
}
