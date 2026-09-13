package main

import (
	"io"
	"testing"
)

func TestOptions(t *testing.T) {
	o, err := parseOptions([]string{"-action", "export", "-league", "NFL", "-season", "2025", "-out", "exports/nfl", "-allow-incomplete", "-overwrite"}, io.Discard)
	if err != nil || o.scope.League != "NFL" || !o.allow || !o.overwrite {
		t.Fatalf("%+v %v", o, err)
	}
	for _, args := range [][]string{{"-season", "2026", "-overwrite"}, {}, {"-season", "2026", "-action", "bad"}, {"-season", "2026", "-action", "export"}, {"-season", "2026", "-out", "x"}, {"-season", "2026", "-allow-incomplete"}, {"-season", "2026", "extra"}} {
		if _, err := parseOptions(args, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
