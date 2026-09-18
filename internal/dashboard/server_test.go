package dashboard

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixture = `{"schema_version":1,"kind":"cross_season","league":"NBA","title":"NBA test","test_season":2026,"candidates":{"strength":{"metrics":{"games":1,"accuracy":1,"log_loss":0.2,"brier_score":0.1}}},"predictions":[{"candidate":"strength","game_id":"g1","date":"2026-01-01","home_win":1,"probability":0.8}],"private_path":"secret-path"}`

func TestAPI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nba.json"), []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	reports, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(reports, "https://frontend.example", nil)
	for _, tc := range []struct {
		path string
		code int
	}{{"/api/runs", 200}, {"/api/runs/nba", 200}, {"/api/runs/missing", 404}, {"/api/runs/nba/predictions?limit=1", 200}, {"/api/runs/nba/predictions?offset=99999", 200}, {"/api/runs/nba/predictions?limit=201", 400}, {"/api/runs/nba/predictions?candidate=bad", 400}, {"/api/runs/nba/predictions?offset=-1", 400}, {"/api/unknown", 404}} {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			req.Header.Set("Origin", "https://frontend.example")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.code {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "secret-path") {
				t.Fatal("private metadata leaked")
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "https://frontend.example" {
				t.Fatal("missing CORS")
			}
		})
	}
	req := httptest.NewRequest("POST", "/api/runs/nba", nil)
	req.Header.Set("Origin", "https://untrusted.example")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code < 400 || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("write or cross-origin access allowed")
	}
}
func TestRejectSymlinkAndMalformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("accepted invalid report")
	}
	if err := os.Remove(filepath.Join(dir, "bad.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("accepted symlink")
	}
}
