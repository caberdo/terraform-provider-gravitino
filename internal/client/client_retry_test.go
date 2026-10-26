package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newReliabilityClient(t *testing.T, uri string, opts ...Option) *Client {
	t.Helper()
	c, err := New(uri, nil, opts...)
	if err != nil {
		t.Fatalf("New(%q) failed: %v", uri, err)
	}
	return c
}

// recordSleeps replaces the retry sleeper with a recorder so a test asserts the
// policy without spending the backoff in wall-clock time.
func recordSleeps(c *Client) *[]time.Duration {
	delays := &[]time.Duration{}
	c.retry.sleep = func(_ context.Context, delay time.Duration) error {
		*delays = append(*delays, delay)
		return nil
	}
	return delays
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

func TestGetRetriesTransientStatuses(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, `{"identifiers":[]}`)
	}))
	t.Cleanup(server.Close)

	c := newReliabilityClient(t, server.URL)
	delays := recordSleeps(c)

	var result map[string]interface{}
	if err := c.Get(context.Background(), "/metalakes/metalake/catalogs", &result); err != nil {
		t.Fatalf("Get() = %v, want success after two 503s", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
	if len(*delays) != 2 {
		t.Errorf("retries = %d, want 2", len(*delays))
	}
}

func TestGetGivesUpAfterBoundedRetries(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	c := newReliabilityClient(t, server.URL)
	delays := recordSleeps(c)

	var result map[string]interface{}
	err := c.Get(context.Background(), "/metalakes", &result)

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Get() = %v, want a 503 HTTPError", err)
	}
	if got := attempts.Load(); got != int32(c.retry.maxRetries+1) {
		t.Errorf("attempts = %d, want %d", got, c.retry.maxRetries+1)
	}
	if len(*delays) != c.retry.maxRetries {
		t.Errorf("retries = %d, want %d", len(*delays), c.retry.maxRetries)
	}
}

func TestGetDoesNotRetryClientError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)

	c := newReliabilityClient(t, server.URL)
	delays := recordSleeps(c)

	var result map[string]interface{}
	err := c.Get(context.Background(), "/metalakes", &result)

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("Get() = %v, want a 400 HTTPError", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (a 400 is not transient)", got)
	}
	if len(*delays) != 0 {
		t.Errorf("retries = %d, want 0", len(*delays))
	}
}

func TestPostIsNeverRetried(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	c := newReliabilityClient(t, server.URL)
	delays := recordSleeps(c)

	var result map[string]interface{}
	err := c.Post(context.Background(), "/metalakes", map[string]string{"name": "lake"}, &result)

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Post() = %v, want a 503 HTTPError", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (a create must not be retried)", got)
	}
	if len(*delays) != 0 {
		t.Errorf("retries = %d, want 0", len(*delays))
	}
}

func TestPostIdempotentRetriesTransientStatus(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, `{"code":0}`)
	}))
	t.Cleanup(server.Close)

	c := newReliabilityClient(t, server.URL)
	delays := recordSleeps(c)

	var result map[string]interface{}
	if err := c.PostIdempotent(context.Background(), "/metalakes/m/catalogs/testConnection", map[string]string{"name": "c"}, &result); err != nil {
		t.Fatalf("PostIdempotent() = %v, want success", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
	if len(*delays) != 1 {
		t.Errorf("retries = %d, want 1", len(*delays))
	}
}

func TestGetHonoursRetryAfter(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeJSON(w, `{}`)
	}))
	t.Cleanup(server.Close)

	c := newReliabilityClient(t, server.URL)
	delays := recordSleeps(c)

	var result map[string]interface{}
	if err := c.Get(context.Background(), "/metalakes", &result); err != nil {
		t.Fatalf("Get() = %v, want success", err)
	}
	if len(*delays) != 1 {
		t.Fatalf("retries = %d, want 1", len(*delays))
	}
	if (*delays)[0] != time.Second {
		t.Errorf("delay = %s, want the 1s Retry-After value", (*delays)[0])
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		value string
		want  time.Duration
		ok    bool
	}{
		{name: "seconds", value: "7", want: 7 * time.Second, ok: true},
		{name: "zero", value: "0", want: 0, ok: true},
		{name: "negative", value: "-1", ok: false},
		{name: "http date", value: now.Add(3 * time.Second).Format(http.TimeFormat), want: 3 * time.Second, ok: true},
		{name: "date in the past", value: now.Add(-time.Minute).Format(http.TimeFormat), want: 0, ok: true},
		{name: "garbage", value: "soon", ok: false},
		{name: "empty", value: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRetryAfter(tt.value, now)
			if ok != tt.ok || got != tt.want {
				t.Errorf("parseRetryAfter(%q) = (%s, %t), want (%s, %t)", tt.value, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestBackoffIsBoundedAndJittered(t *testing.T) {
	policy := defaultRetryPolicy()

	attempts := []struct {
		attempt int
		min     time.Duration
		max     time.Duration
	}{
		{attempt: 0, min: policy.baseDelay / 2, max: policy.baseDelay},
		{attempt: 1, min: policy.baseDelay, max: 2 * policy.baseDelay},
		{attempt: 2, min: 2 * policy.baseDelay, max: 4 * policy.baseDelay},
		{attempt: 10, min: policy.maxDelay / 2, max: policy.maxDelay},
	}
	for _, tt := range attempts {
		for range 100 {
			got := policy.backoff(tt.attempt, 0, false)
			if got < tt.min || got > tt.max {
				t.Fatalf("backoff(attempt=%d) = %s, want within [%s, %s]", tt.attempt, got, tt.min, tt.max)
			}
		}
	}

	if got := policy.backoff(0, 10*time.Second, true); got != policy.maxDelay {
		t.Errorf("backoff with a long Retry-After = %s, want the %s cap", got, policy.maxDelay)
	}
}

func TestGetRetriesConnectionReset(t *testing.T) {
	c := newReliabilityClient(t, "http://gravitino.example")
	delays := recordSleeps(c)

	var attempts atomic.Int32
	c.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			return nil, &url.Error{Op: req.Method, URL: req.URL.String(), Err: syscall.ECONNRESET}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    req,
		}, nil
	})

	var result map[string]interface{}
	if err := c.Get(context.Background(), "/metalakes", &result); err != nil {
		t.Fatalf("Get() = %v, want a retry after the reset connection", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
	if len(*delays) != 1 {
		t.Errorf("retries = %d, want 1", len(*delays))
	}
}

func TestPostDoesNotRetryConnectionReset(t *testing.T) {
	c := newReliabilityClient(t, "http://gravitino.example")
	delays := recordSleeps(c)

	var attempts atomic.Int32
	c.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return nil, &url.Error{Op: req.Method, URL: req.URL.String(), Err: syscall.ECONNRESET}
	})

	var result map[string]interface{}
	if err := c.Post(context.Background(), "/metalakes", map[string]string{"name": "lake"}, &result); err == nil {
		t.Fatal("Post() = nil, want the connection error")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (a create must not be retried)", got)
	}
	if len(*delays) != 0 {
		t.Errorf("retries = %d, want 0", len(*delays))
	}
}

func TestTimeoutSurfacesAsClearError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		writeJSON(w, `{}`)
	}))
	t.Cleanup(server.Close)

	c := newReliabilityClient(t, server.URL, WithTimeout(20*time.Millisecond))
	recordSleeps(c)

	var result map[string]interface{}
	err := c.Get(context.Background(), "/metalakes", &result)
	if err == nil {
		t.Fatal("Get() = nil, want a timeout")
	}
	if !strings.Contains(err.Error(), "timed out after 20ms") {
		t.Fatalf("error = %q, want it to name the configured 20ms timeout", err)
	}
}

func TestCallerDeadlineIsNotRetried(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		time.Sleep(200 * time.Millisecond)
		writeJSON(w, `{}`)
	}))
	t.Cleanup(server.Close)

	c := newReliabilityClient(t, server.URL)
	delays := recordSleeps(c)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	var result map[string]interface{}
	err := c.Get(ctx, "/metalakes", &result)
	if err == nil {
		t.Fatal("Get() = nil, want the caller deadline error")
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("error = %q, want it to report the cancelled caller context", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
	if len(*delays) != 0 {
		t.Errorf("retries = %d, want 0", len(*delays))
	}
}

func TestUserAgent(t *testing.T) {
	var got atomic.Value
	got.Store("")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("User-Agent"))
		writeJSON(w, `{}`)
	}))
	t.Cleanup(server.Close)

	tests := []struct {
		name string
		opts []Option
		want string
	}{
		{name: "version", opts: []Option{WithUserAgent("1.2.3")}, want: "terraform-provider-gravitino/1.2.3"},
		{name: "empty version falls back to dev", opts: []Option{WithUserAgent("")}, want: "terraform-provider-gravitino/dev"},
		{name: "unset keeps the dev marker", want: "terraform-provider-gravitino/dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newReliabilityClient(t, server.URL, tt.opts...)
			var result map[string]interface{}
			if err := c.Get(context.Background(), "/metalakes", &result); err != nil {
				t.Fatalf("Get() = %v", err)
			}
			if ua := got.Load().(string); ua != tt.want {
				t.Errorf("User-Agent = %q, want %q", ua, tt.want)
			}
		})
	}
}
