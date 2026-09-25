package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client/auth"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

const (
	defaultTimeout   = 30 * time.Second
	contentType      = "application/vnd.gravitino.v1+json"
	plainContentType = "application/json"
	// maxErrorBodyBytes bounds how much of a non-JSON error body is echoed back
	// to the user, so a misconfigured URI pointing at a huge page cannot flood
	// the Terraform output.
	maxErrorBodyBytes = 2 * 1024
)

type Client struct {
	baseURL      string
	httpClient   *http.Client
	authProvider auth.AuthProvider
}

func New(uri string, authProvider auth.AuthProvider) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(uri, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid URI: %w", err)
	}

	c := &Client{
		baseURL: u.String(),
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		authProvider: authProvider,
	}

	if tp, ok := authProvider.(auth.TransportProvider); ok {
		c.httpClient.Transport = tp.WrapTransport(http.DefaultTransport)
	}

	return c, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api"+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", contentType)
	if body != nil {
		req.Header.Set("Content-Type", plainContentType)
	}

	if c.authProvider != nil {
		key, value, err := c.authProvider.Header(ctx)
		if err != nil {
			return nil, fmt.Errorf("auth header failed: %w", err)
		}
		if key != "" && value != "" {
			req.Header.Set(key, value)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, result interface{}) error {
	resp, err := c.doRequest(ctx, method, path, body)
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
		return fmt.Errorf("failed to decode response: %w", err)
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
	if err := json.Unmarshal(raw, &errResp); err != nil {
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
	return c.do(ctx, http.MethodGet, path, nil, result)
}

func (c *Client) Post(ctx context.Context, path string, body, result interface{}) error {
	return c.do(ctx, http.MethodPost, path, body, result)
}

func (c *Client) Put(ctx context.Context, path string, body, result interface{}) error {
	return c.do(ctx, http.MethodPut, path, body, result)
}

func (c *Client) Delete(ctx context.Context, path string, result interface{}) error {
	return c.do(ctx, http.MethodDelete, path, nil, result)
}

func (c *Client) Patch(ctx context.Context, path string, body, result interface{}) error {
	return c.do(ctx, http.MethodPatch, path, body, result)
}

func (c *Client) BaseURL() string {
	return c.baseURL
}
