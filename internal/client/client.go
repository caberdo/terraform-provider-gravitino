package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client/auth"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	// DefaultTimeout is the request timeout when the provider does not
	// configure one. It is exported so the provider schema's default and the
	// docs name the same number.
	DefaultTimeout   = 30 * time.Second
	contentType      = "application/vnd.gravitino.v1+json"
	plainContentType = "application/json"
	// maxErrorBodyBytes bounds how much of a non-JSON error body is echoed back
	// to the user, so a misconfigured URI pointing at a huge page cannot flood
	// the Terraform output.
	maxErrorBodyBytes = 2 * 1024

	// defaultUserAgent identifies provider traffic when the caller does not
	// supply a version (tests, the live acceptance pre-check).
	defaultUserAgent = "terraform-provider-gravitino/dev"

	// Retry policy: at most three retries after the first attempt, an
	// exponential backoff that starts at 100 ms, and a hard cap of 2 s so a
	// rate-limiting or restarting server cannot stretch an apply indefinitely.
	// `Retry-After` is honoured up to the same cap.
	defaultMaxRetries     = 3
	defaultRetryBaseDelay = 100 * time.Millisecond
	defaultRetryMaxDelay  = 2 * time.Second
)

type Client struct {
	baseURL      string
	httpClient   *http.Client
	authProvider auth.AuthProvider

	timeout   time.Duration
	userAgent string
	retry     retryPolicy

	// version is the server version detected by DetectServerVersion; empty when
	// the probe failed, in which case the gates treat the server as current.
	versionMu sync.RWMutex
	version   string
}

// Option configures a Client. Both options are optional: an unset timeout falls
// back to DefaultTimeout and an unset version to a "dev" User-Agent.
type Option func(*Client)

// WithTimeout sets the ceiling on a single HTTP attempt. A non-positive value
// is ignored, so a zero from an absent provider attribute is not an error.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.timeout = timeout
		}
	}
}

// WithUserAgent sets the User-Agent to
// "terraform-provider-gravitino/<version>". An empty version yields the
// "dev" marker rather than a trailing slash.
func WithUserAgent(version string) Option {
	return func(c *Client) {
		version = strings.TrimSpace(version)
		if version == "" {
			version = "dev"
		}
		c.userAgent = "terraform-provider-gravitino/" + version
	}
}

// retryPolicy is the bounded retry applied to requests that are safe to
// repeat. `sleep` is a field so tests can observe the delay without waiting.
type retryPolicy struct {
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	sleep      func(context.Context, time.Duration) error
}

func defaultRetryPolicy() retryPolicy {
	return retryPolicy{
		maxRetries: defaultMaxRetries,
		baseDelay:  defaultRetryBaseDelay,
		maxDelay:   defaultRetryMaxDelay,
		sleep:      sleepContext,
	}
}

func New(uri string, authProvider auth.AuthProvider, opts ...Option) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(uri, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid URI: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid URI %q: scheme must be http or https", uri)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid URI %q: host must not be empty", uri)
	}

	c := &Client{
		baseURL:      u.String(),
		authProvider: authProvider,
		timeout:      DefaultTimeout,
		userAgent:    defaultUserAgent,
		retry:        defaultRetryPolicy(),
	}
	for _, opt := range opts {
		opt(c)
	}

	c.httpClient = &http.Client{Timeout: c.timeout}
	if tp, ok := authProvider.(auth.TransportProvider); ok {
		c.httpClient.Transport = tp.WrapTransport(http.DefaultTransport)
	}

	return c, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, retryable bool) (*http.Response, error) {
	var payload []byte
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal %s %s request body: %w", method, path, err)
		}
		payload = data
	}

	for attempt := 0; ; attempt++ {
		var bodyReader io.Reader
		if payload != nil {
			// A fresh reader per attempt: a retried request must not reuse a
			// body the previous attempt already consumed.
			bodyReader = bytes.NewReader(payload)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api"+path, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("failed to create %s %s request: %w", method, path, err)
		}

		req.Header.Set("Accept", contentType)
		req.Header.Set("User-Agent", c.userAgent)
		if body != nil {
			req.Header.Set("Content-Type", plainContentType)
		}

		if c.authProvider != nil {
			key, value, err := c.authProvider.Header(ctx)
			if err != nil {
				return nil, fmt.Errorf("auth header for %s %s failed: %w", method, path, err)
			}
			if key != "" && value != "" {
				req.Header.Set(key, value)
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// A cancelled or expired caller context is final: retrying would
			// outlive the deadline the user gave Terraform.
			if ctx.Err() != nil {
				return nil, c.requestError(ctx, method, path, err)
			}
			if retryable && attempt < c.retry.maxRetries && isConnectionReset(err) {
				delay := c.retry.backoff(attempt, 0, false)
				tflog.Warn(ctx, "Retrying Gravitino API request after a connection reset", map[string]interface{}{
					"method":  method,
					"path":    path,
					"attempt": attempt + 1,
					"delay":   delay.String(),
					"error":   err.Error(),
				})
				if err := c.retry.sleep(ctx, delay); err != nil {
					return nil, c.requestError(ctx, method, path, err)
				}
				continue
			}
			return nil, c.requestError(ctx, method, path, err)
		}

		if !retryable || attempt >= c.retry.maxRetries || !retryableStatus(resp.StatusCode) {
			return resp, nil
		}

		retryAfter, hasRetryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		drainAndClose(resp.Body)
		delay := c.retry.backoff(attempt, retryAfter, hasRetryAfter)
		tflog.Warn(ctx, "Retrying Gravitino API request after a transient response", map[string]interface{}{
			"method":  method,
			"path":    path,
			"attempt": attempt + 1,
			"status":  resp.StatusCode,
			"delay":   delay.String(),
		})
		if err := c.retry.sleep(ctx, delay); err != nil {
			return nil, c.requestError(ctx, method, path, err)
		}
	}
}

// requestError keeps the configured timeout visible when the transport gave up
// on the request, instead of leaving the user with an opaque "context deadline
// exceeded" from the HTTP client. A caller that cancelled Terraform's own
// context is reported as such, not as the provider's per-attempt timeout.
func (c *Client) requestError(ctx context.Context, method, path string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s %s aborted: %w", method, path, ctxErr)
	}
	if isTimeout(err) {
		return fmt.Errorf("%s %s timed out after %s: %w", method, path, c.timeout, err)
	}
	return fmt.Errorf("%s %s failed: %w", method, path, err)
}

// isTimeout reports whether err is the HTTP client's own deadline: the caller's
// context has not expired, but the configured per-attempt timeout has.
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isConnectionReset(err error) bool {
	return errors.Is(err, syscall.ECONNRESET)
}

func retryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// parseRetryAfter accepts both forms RFC 9110 allows: delta-seconds and an
// HTTP-date.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	if when, err := http.ParseTime(value); err == nil {
		delay := when.Sub(now)
		if delay < 0 {
			delay = 0
		}
		return delay, true
	}
	return 0, false
}

// backoff returns the delay before the next attempt. A server-supplied
// Retry-After wins and is capped like the exponential path; otherwise the
// delay doubles per attempt and carries "equal jitter" (half fixed, half
// random) so concurrent Terraform runs do not retry in lockstep.
func (p retryPolicy) backoff(attempt int, retryAfter time.Duration, hasRetryAfter bool) time.Duration {
	if hasRetryAfter {
		if retryAfter > p.maxDelay {
			return p.maxDelay
		}
		return retryAfter
	}

	delay := p.baseDelay
	for i := 0; i < attempt && delay < p.maxDelay; i++ {
		delay *= 2
	}
	if delay > p.maxDelay || delay <= 0 {
		delay = p.maxDelay
	}

	half := delay / 2
	// #nosec G404 -- jitter only spreads retries; it is not a security primitive.
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
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

// drainAndClose consumes a bounded amount of a response that is about to be
// retried, so the connection can be reused, and closes it.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 4*1024))
	_ = body.Close()
}

func (c *Client) do(ctx context.Context, method, path string, body, result interface{}, retryable bool) error {
	resp, err := c.doRequest(ctx, method, path, body, retryable)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return newHTTPError(resp)
	}

	if result == nil {
		return nil
	}

	// Endpoints such as DELETE return an empty body; that is not an error.
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("failed to decode %s %s response: %w", method, path, err)
	}

	return nil
}

// newHTTPError builds an HTTPError, preserving the real HTTP status code. The
// status is captured before decoding because a Gravitino error payload carries
// its own application code in the `code` field.
func newHTTPError(resp *http.Response) *HTTPError {
	httpErr := &HTTPError{
		StatusCode: resp.StatusCode,
		Status:     http.StatusText(resp.StatusCode),
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	if err != nil || len(raw) == 0 {
		return httpErr
	}

	var errResp models.ErrorResponse
	if err := json.Unmarshal(raw, &errResp); err != nil || (errResp.Type == "" && errResp.Message == "") {
		httpErr.Status = fmt.Sprintf("%s: %s", httpErr.Status, truncateBody(raw))
		return httpErr
	}

	httpErr.Response = &errResp
	return httpErr
}

func truncateBody(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
	return c.do(ctx, http.MethodGet, path, nil, result, true)
}

// Post sends a request that may create or otherwise mutate server state, so it
// is never retried: a 503 that arrives after the server committed the write
// would turn one apply into two objects.
func (c *Client) Post(ctx context.Context, path string, body, result interface{}) error {
	return c.do(ctx, http.MethodPost, path, body, result, false)
}

// PostIdempotent sends a POST that is safe to repeat, such as a connection
// probe that creates nothing. Use it only for endpoints where a second request
// has no additional effect.
func (c *Client) PostIdempotent(ctx context.Context, path string, body, result interface{}) error {
	return c.do(ctx, http.MethodPost, path, body, result, true)
}

func (c *Client) Put(ctx context.Context, path string, body, result interface{}) error {
	return c.do(ctx, http.MethodPut, path, body, result, true)
}

func (c *Client) Delete(ctx context.Context, path string, result interface{}) error {
	return c.do(ctx, http.MethodDelete, path, nil, result, true)
}

func (c *Client) Patch(ctx context.Context, path string, body, result interface{}) error {
	return c.do(ctx, http.MethodPatch, path, body, result, true)
}
