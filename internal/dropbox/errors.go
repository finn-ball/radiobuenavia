package dropbox

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// APIError carries Dropbox HTTP metadata so callers can retry intelligently.
type APIError struct {
	Endpoint   string
	Method     string
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Body == "" {
		return fmt.Sprintf("dropbox request failed: %s %s returned HTTP %d", e.Method, e.Endpoint, e.StatusCode)
	}
	return fmt.Sprintf("dropbox request failed: %s %s returned HTTP %d: %s", e.Method, e.Endpoint, e.StatusCode, e.Body)
}

func (e *APIError) Retryable() bool {
	if e == nil {
		return false
	}
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= http.StatusInternalServerError
}

func (e *APIError) RetryDelay() (time.Duration, bool) {
	if e == nil || e.RetryAfter <= 0 {
		return 0, false
	}
	return e.RetryAfter, true
}

func newAPIError(endpoint string, resp *http.Response, body []byte) *APIError {
	method := http.MethodPost
	if resp != nil && resp.Request != nil && resp.Request.Method != "" {
		method = resp.Request.Method
	}
	status := 0
	retryAfter := time.Duration(0)
	if resp != nil {
		status = resp.StatusCode
		retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
	}
	return &APIError{
		Endpoint:   endpoint,
		Method:     method,
		StatusCode: status,
		Body:       strings.TrimSpace(string(body)),
		RetryAfter: retryAfter,
	}
}

func parseRetryAfter(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(raw); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}
