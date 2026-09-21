package espn

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxBody = 2 << 20

// Client serializes requests and retries through one gate. Share one client per
// process; separate processes do not share this limiter.
type Client struct {
	http           *http.Client
	baseURL        string
	interval       time.Duration
	gate           chan struct{}
	next           time.Time
	blocked        error
	onSkippedEvent func(string, string)
}

// Options are supplied by command configuration, not read from the environment.
type Options struct {
	// OnSkippedEvent reports an excluded event ID and reason; calls may be concurrent.
	OnSkippedEvent  func(string, string)
	BaseURL         string
	RequestInterval time.Duration
	HTTPTimeout     time.Duration
}

func NewClient(options Options) (*Client, error) {
	if options.RequestInterval < MinInterval || options.RequestInterval > time.Hour {
		return nil, fmt.Errorf("ESPN request interval must be between %s and 1h", MinInterval)
	}
	if options.HTTPTimeout <= 0 {
		return nil, fmt.Errorf("ESPN HTTP timeout must be positive")
	}
	base, err := url.Parse(options.BaseURL)
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return nil, fmt.Errorf("ESPN_BASE_URL must be an HTTP(S) origin without credentials, a path, query, or fragment")
	}
	return &Client{
		onSkippedEvent: options.OnSkippedEvent,
		http:           &http.Client{Timeout: options.HTTPTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		baseURL:        strings.TrimRight(options.BaseURL, "/"),
		interval:       options.RequestInterval,
		gate:           make(chan struct{}, 1),
	}, nil
}

// get executes a paced GET with the shared retry policy.
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	response, err := c.getSnapshot(ctx, path)
	if err != nil {
		return nil, err
	}
	return response.body, nil
}

type snapshot struct {
	body       []byte
	observedAt time.Time
}

// Capture observation time while holding the gate, before another request starts.
func (c *Client) getSnapshot(ctx context.Context, path string) (*snapshot, error) {
	return c.getSnapshotWithLimit(ctx, path, maxBody)
}

func (c *Client) getSnapshotWithLimit(ctx context.Context, path string, bodyLimit int64) (*snapshot, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()
	if c.blocked != nil {
		return nil, c.blocked
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := wait(ctx, time.Until(c.next)); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		// Let net/http supply its standard User-Agent.
		resp, err := c.http.Do(req)
		c.next = time.Now().Add(c.interval)
		if err != nil {
			return nil, fmt.Errorf("fetch ESPN response: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
		closeErr := resp.Body.Close()
		// Pace from completion, not from request start; slow requests cannot overlap.
		c.next = time.Now().Add(c.interval)
		if resp.StatusCode != http.StatusOK {
			retry, err := c.shouldRetry(resp, attempt)
			if retry {
				continue
			}
			return nil, err
		}
		if readErr != nil {
			return nil, fmt.Errorf("read ESPN response: %w", readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close ESPN response: %w", closeErr)
		}
		if int64(len(body)) > bodyLimit {
			return nil, fmt.Errorf("ESPN response exceeds %d bytes", bodyLimit)
		}
		return &snapshot{body: body, observedAt: time.Now().UTC()}, nil
	}
	return nil, fmt.Errorf("ESPN retry budget exhausted")
}
