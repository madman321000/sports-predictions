package espn

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// shouldRetry updates the shared cooldown and access restriction state.
// The caller must hold the client's request gate.
func (c *Client) shouldRetry(resp *http.Response, attempt int) (bool, error) {
	statusErr := &HTTPError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		c.blocked = statusErr
		return false, statusErr
	}
	var serverDelay time.Duration
	if raw := resp.Header.Get("Retry-After"); raw != "" {
		var err error
		serverDelay, err = retryDelay(raw, time.Now())
		if err != nil || serverDelay > time.Minute {
			c.blocked = statusErr
			return false, statusErr
		}
		if serverDelay > c.interval {
			c.next = time.Now().Add(serverDelay)
		}
	}
	if (resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504) && attempt < 2 {
		delay := max(c.interval*time.Duration(1<<attempt), serverDelay)
		c.next = time.Now().Add(delay)
		return true, nil
	}
	return false, statusErr
}

func retryDelay(raw string, now time.Time) (time.Duration, error) {
	if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if seconds < 0 || seconds > 86400 {
			return 0, fmt.Errorf("unsupported Retry-After")
		}
		return time.Duration(seconds) * time.Second, nil
	}
	date, err := http.ParseTime(raw)
	if err != nil {
		return 0, err
	}
	return max(time.Duration(0), date.Sub(now)), nil
}
