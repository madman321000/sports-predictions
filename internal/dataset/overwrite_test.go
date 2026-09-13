package dataset

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
)

func TestOverwriteExport(t *testing.T) {
	s, d := fixture()
	path := filepath.Join(t.TempDir(), "export")
	if _, err := Export(path, s, d, ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	d.Players = []Player{uncertainPlayer()}
	if _, err := Export(path, s, d, ExportOptions{Overwrite: true}); err != nil {
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
	if rows[0][10] != "participation_status" || rows[1][10] != "uncertain" || rows[1][9] != d.Players[0].Stats || rows[1][7] != "false" {
		t.Fatalf("raw data or uncertainty lost: %v", rows)
	}
	before, err := os.ReadFile(filepath.Join(path, "players.csv"))
	if err != nil {
		t.Fatal(err)
	}
	d.Games[0].PlayersComplete = false
	if _, err := Export(path, s, d, ExportOptions{Overwrite: true}); err == nil {
		t.Fatal("failed quality accepted")
	}
	after, err := os.ReadFile(filepath.Join(path, "players.csv"))
	if err != nil || string(before) != string(after) {
		t.Fatal("failed refresh damaged old export")
	}
}

func TestOverwriteRefusesUnmanagedDestinations(t *testing.T) {
	for _, name := range []string{"extra file", "symlink directory", "symlink file", "wrong season", "bad report", "locked", "file"} {
		t.Run(name, func(t *testing.T) {
			s, d := fixture()
			root := t.TempDir()
			path := filepath.Join(root, "export")
			if _, err := Export(path, s, d, ExportOptions{}); err != nil {
				t.Fatal(err)
			}
			put := func(path, body string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "extra file":
				put(filepath.Join(path, "personal.txt"), "keep")
			case "symlink directory":
				link := filepath.Join(root, "link")
				if err := os.Symlink(path, link); err != nil {
					t.Fatal(err)
				}
				path = link
			case "symlink file":
				target := filepath.Join(root, "personal.txt")
				put(target, "keep")
				if err := os.Remove(filepath.Join(path, "games.csv")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(path, "games.csv")); err != nil {
					t.Fatal(err)
				}
			case "wrong season":
				s.Season = 2025
			case "bad report":
				put(filepath.Join(path, "report.json"), "invalid")
			case "locked":
				put(path+".lock", "")
			case "file":
				path = filepath.Join(root, "file")
				put(path, "keep")
			}
			if _, err := Export(path, s, d, ExportOptions{Overwrite: true}); err == nil {
				t.Fatal("unsafe overwrite accepted")
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatal("destination lost")
			}
		})
	}
}
