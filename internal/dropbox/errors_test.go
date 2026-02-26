package dropbox

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfterSeconds(t *testing.T) {
	got := parseRetryAfter("5")
	if got != 5*time.Second {
		t.Fatalf("expected 5s, got %s", got)
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	when := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)
	got := parseRetryAfter(when)
	if got <= 0 {
		t.Fatalf("expected positive delay, got %s", got)
	}
	if got > 3*time.Second {
		t.Fatalf("expected delay under 3s, got %s", got)
	}
}

func TestAPIErrorRetryable(t *testing.T) {
	if !(&APIError{StatusCode: http.StatusTooManyRequests}).Retryable() {
		t.Fatal("expected 429 to be retryable")
	}
	if !(&APIError{StatusCode: http.StatusBadGateway}).Retryable() {
		t.Fatal("expected 5xx to be retryable")
	}
	if (&APIError{StatusCode: http.StatusBadRequest}).Retryable() {
		t.Fatal("expected 4xx (non-429) to be non-retryable")
	}
}
