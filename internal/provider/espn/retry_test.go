package espn

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestNBAStopsOnAccessRestrictions(t *testing.T) {
	for _, status := range []int{403, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(status)
			})
			for i := 0; i < 2; i++ {
				_, err := c.FetchNBATeams(context.Background())
				var got *HTTPError
				if !errors.As(err, &got) || got.StatusCode != status || got.RetryAfter != "120" {
					t.Fatalf("error = %v", err)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("requests = %d, want 1", calls.Load())
			}
		})
	}
}

func TestNBARetriesAreBounded(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		status, failures, wantCalls int
		wantError                   bool
	}{
		{"recovers", 503, 2, 3, false}, {"exhausted", 502, 5, 3, true}, {"bad request", 400, 5, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			body := fixture(t)
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if int(calls.Add(1)) <= tc.failures {
					w.WriteHeader(tc.status)
					return
				}
				respond(w, body)
			})
			_, err := c.FetchNBATeams(context.Background())
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v", err)
			}
			if int(calls.Load()) != tc.wantCalls {
				t.Fatalf("requests = %d, want %d", calls.Load(), tc.wantCalls)
			}
		})
	}
}

func TestNBARetryAfterAndCancellation(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(503)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.FetchNBATeams(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("retried before server delay: %d", calls.Load())
	}
}

func TestRetryDelay(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		raw  string
		want time.Duration
		bad  bool
	}{
		{"10", 10 * time.Second, false}, {now.Add(20 * time.Second).Format(http.TimeFormat), 20 * time.Second, false},
		{now.Add(-time.Second).Format(http.TimeFormat), 0, false}, {"-1", 0, true}, {"9999999999999999999999", 0, true}, {"bad", 0, true},
	} {
		got, err := retryDelay(tc.raw, now)
		if (err != nil) != tc.bad || (!tc.bad && got != tc.want) {
			t.Errorf("%q: got %s, %v", tc.raw, got, err)
		}
	}
	if _, err := NewClient(Options{BaseURL: "https://example.com", RequestInterval: time.Second, HTTPTimeout: time.Second}); err == nil {
		t.Fatal("accepted unsafe interval")
	}
}

func TestNBALongOrInvalidRetryAfterStopsClient(t *testing.T) {
	for _, header := range []string{"120", "invalid"} {
		t.Run(header, func(t *testing.T) {
			var calls atomic.Int32
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", header)
				w.WriteHeader(503)
			})
			for i := 0; i < 2; i++ {
				if _, err := c.FetchNBATeams(context.Background()); err == nil {
					t.Fatal("expected server error")
				}
			}
			if calls.Load() != 1 {
				t.Fatal("retried despite server delay")
			}
		})
	}
}
