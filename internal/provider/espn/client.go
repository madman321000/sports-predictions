package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/madman321000/sports-predictions/internal/team"
)

const MinInterval = 5 * time.Second
const maxBody = 2 << 20

// Client serializes requests and retries through one gate. Share one client per
// process; separate processes do not share this limiter.
type Client struct {
	http     *http.Client
	baseURL  string
	interval time.Duration
	gate     chan struct{}
	next     time.Time
	blocked  error
}

func NewClient(interval time.Duration) (*Client, error) {
	if interval < MinInterval || interval > time.Hour {
		return nil, fmt.Errorf("ESPN request interval must be between %s and 1h", MinInterval)
	}
	return &Client{
		http:     &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		baseURL:  "https://site.api.espn.com",
		interval: interval,
		gate:     make(chan struct{}, 1),
	}, nil
}

// HTTPError deliberately excludes response bodies, which may contain HTML or
// other provider content unrelated to the failed operation.
type HTTPError struct {
	StatusCode int
	RetryAfter string
}

func (e *HTTPError) Error() string {
	if e.RetryAfter != "" {
		return fmt.Sprintf("ESPN HTTP %d (Retry-After: %s)", e.StatusCode, e.RetryAfter)
	}
	return fmt.Sprintf("ESPN HTTP %d", e.StatusCode)
}

func wait(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// NBA fetches the team list once; no per-team requests or automatic polling.
func (c *Client) NBA(ctx context.Context) ([]team.Team, error) {
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.gate }()
	if c.blocked != nil {
		return nil, c.blocked
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := wait(ctx, time.Until(c.next)); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/apis/site/v2/sports/basketball/nba/teams?limit=100", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "sports-predictions/0.1 (learning project)")
		resp, err := c.http.Do(req)
		c.next = time.Now().Add(c.interval)
		if err != nil {
			return nil, fmt.Errorf("fetch ESPN NBA teams: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
		closeErr := resp.Body.Close()
		// Pace from completion, not from request start; slow requests cannot overlap.
		c.next = time.Now().Add(c.interval)
		if resp.StatusCode != http.StatusOK {
			statusErr := &HTTPError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
			if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
				c.blocked = statusErr
				return nil, statusErr
			}
			var serverDelay time.Duration
			if raw := resp.Header.Get("Retry-After"); raw != "" {
				var err error
				serverDelay, err = retryDelay(raw, time.Now())
				if err != nil || serverDelay > time.Minute {
					c.blocked = statusErr
					return nil, statusErr
				}
				if serverDelay > c.interval {
					c.next = time.Now().Add(serverDelay)
				}
			}
			if (resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504) && attempt < 2 {
				delay := max(c.interval*time.Duration(1<<attempt), serverDelay)
				c.next = time.Now().Add(delay)
				continue
			}
			return nil, statusErr
		}
		if readErr != nil {
			return nil, fmt.Errorf("read ESPN teams: %w", readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close ESPN response: %w", closeErr)
		}
		if len(body) > maxBody {
			return nil, fmt.Errorf("ESPN response exceeds %d bytes", maxBody)
		}
		return decodeTeams(body)
	}
	return nil, fmt.Errorf("ESPN retry budget exhausted")
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

func decodeTeams(body []byte) ([]team.Team, error) {
	var response struct {
		Sports []struct {
			Leagues []struct {
				Abbreviation string `json:"abbreviation"`
				Teams        []struct {
					Team struct {
						ID           string `json:"id"`
						Name         string `json:"displayName"`
						Abbreviation string `json:"abbreviation"`
					} `json:"team"`
				} `json:"teams"`
			} `json:"leagues"`
		} `json:"sports"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode ESPN teams: %w", err)
	}
	var result []team.Team
	seen := make(map[string]bool)
	for _, sport := range response.Sports {
		for _, league := range sport.Leagues {
			if league.Abbreviation != "NBA" {
				continue
			}
			for _, entry := range league.Teams {
				t := entry.Team
				if strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.Abbreviation) == "" {
					return nil, fmt.Errorf("ESPN team is missing required fields")
				}
				if seen[t.ID] {
					return nil, fmt.Errorf("duplicate ESPN team ID %q", t.ID)
				}
				seen[t.ID] = true
				result = append(result, team.Team{ExternalID: t.ID, Name: t.Name, Abbreviation: t.Abbreviation})
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("ESPN returned no NBA teams")
	}
	return result, nil
}
