package espn

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
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

func TestNBA(t *testing.T) {
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
	teams, err := c.NBA(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 2 || teams[0].ExternalID != "1" || teams[0].Name != "Atlanta Hawks" || teams[1].Abbreviation != "BOS" {
		t.Fatalf("unexpected teams: %+v", teams)
	}
}

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
				_, err := c.NBA(context.Background())
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
			_, err := c.NBA(context.Background())
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
	_, err := c.NBA(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("retried before server delay: %d", calls.Load())
	}
}

func TestNBAPacesConcurrentCallers(t *testing.T) {
	var mu sync.Mutex
	var times []time.Time
	body := fixture(t)
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		respond(w, body)
	})
	c.interval = 20 * time.Millisecond
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Go(func() {
			if _, err := c.NBA(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(times) != 3 {
		t.Fatalf("requests = %d", len(times))
	}
	for i := 1; i < len(times); i++ {
		if times[i].Sub(times[i-1]) < c.interval {
			t.Fatal("requests were not paced")
		}
	}
}

func TestNBACancelWhileQueued(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected request") })
	c.gate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.NBA(ctx)
	<-c.gate
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestNBARejectsRedirect(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Redirect(w, r, "/other", http.StatusFound)
	})
	if _, err := c.NBA(context.Background()); err == nil {
		t.Fatal("expected redirect error")
	}
	if calls.Load() != 1 {
		t.Fatal("followed unpaced redirect")
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
	if _, err := NewClient(time.Second); err == nil {
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
				if _, err := c.NBA(context.Background()); err == nil {
					t.Fatal("expected server error")
				}
			}
			if calls.Load() != 1 {
				t.Fatal("retried despite server delay")
			}
		})
	}
}
