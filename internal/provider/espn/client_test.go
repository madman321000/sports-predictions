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
	c, err := NewClient(Options{BaseURL: s.URL, RequestInterval: MinInterval, HTTPTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
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

func TestNewClientValidatesOptions(t *testing.T) {
	for _, base := range []string{"", ":bad", "file:///tmp/endpoint", "https://user:secret@example.com", "https://example.com/path", "https://example.com?key=secret", "https://example.com#fragment"} {
		if _, err := NewClient(Options{BaseURL: base, RequestInterval: MinInterval, HTTPTimeout: time.Second}); err == nil {
			t.Errorf("accepted invalid origin %q", base)
		}
	}
	if _, err := NewClient(Options{BaseURL: "https://example.com", RequestInterval: MinInterval}); err == nil {
		t.Fatal("accepted missing timeout")
	}
}
