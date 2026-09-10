package espn

import "fmt"

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
