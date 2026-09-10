package seed_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/madman321000/sports-predictions/internal/league"
	"github.com/madman321000/sports-predictions/internal/seed"
)

type recordingWriter struct {
	values   []league.League
	contexts []context.Context
	failAt   int
	err      error
}

func (w *recordingWriter) Create(ctx context.Context, value *league.League) error {
	w.values = append(w.values, *value)
	w.contexts = append(w.contexts, ctx)
	if len(w.values) == w.failAt {
		return w.err
	}
	return nil
}

func TestSeedLeagues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := &recordingWriter{}
	if err := seed.SeedLeagues(ctx, writer); err != nil {
		t.Fatal(err)
	}
	want := []league.League{
		{Name: "National Football League", Abbreviation: "NFL", Sport: "Football"},
		{Name: "National Basketball Association", Abbreviation: "NBA", Sport: "Basketball"},
	}
	if !reflect.DeepEqual(writer.values, want) {
		t.Fatalf("writes = %#v, want %#v", writer.values, want)
	}
	for _, got := range writer.contexts {
		if got != ctx {
			t.Fatal("writer did not receive caller context")
		}
	}
}

func TestSeedLeaguesReturnsWriterError(t *testing.T) {
	for _, tc := range []struct {
		name       string
		failAt     int
		leagueName string
	}{
		{"first write", 1, "National Football League"},
		{"second write", 2, "National Basketball Association"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := errors.New("write failed")
			writer := &recordingWriter{failAt: tc.failAt, err: want}
			err := seed.SeedLeagues(context.Background(), writer)
			if !errors.Is(err, want) {
				t.Fatalf("error = %v, want wrapped %v", err, want)
			}
			if !strings.Contains(err.Error(), tc.leagueName) {
				t.Fatalf("error does not identify failed league: %v", err)
			}
			if len(writer.values) != tc.failAt {
				t.Fatalf("writes = %d, want %d", len(writer.values), tc.failAt)
			}
		})
	}
}
