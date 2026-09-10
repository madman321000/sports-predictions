package espn

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

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
			if _, err := c.FetchNBATeams(context.Background()); err != nil {
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
	_, err := c.FetchNBATeams(ctx)
	<-c.gate
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}
