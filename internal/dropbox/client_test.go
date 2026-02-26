package dropbox

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRefreshAccessTokenReturnsAPIErrorFor429(t *testing.T) {
	c := &Client{
		appKey:       "key",
		appSecret:    "secret",
		refreshToken: "refresh",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Retry-After": []string{"3"}},
					Body:       io.NopCloser(strings.NewReader(`{"error":"rate_limited"}`)),
					Request:    req,
				}, nil
			}),
		},
	}

	err := c.refreshAccessToken()
	if err == nil {
		t.Fatal("expected error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Endpoint != "/oauth2/token" {
		t.Fatalf("expected /oauth2/token endpoint, got %q", apiErr.Endpoint)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 status, got %d", apiErr.StatusCode)
	}
	if apiErr.RetryAfter != 3*time.Second {
		t.Fatalf("expected Retry-After 3s, got %s", apiErr.RetryAfter)
	}
	if !apiErr.Retryable() {
		t.Fatal("expected 429 error to be retryable")
	}
}
