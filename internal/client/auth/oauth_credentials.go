package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

var tokenHTTPClient = &http.Client{Timeout: 30 * time.Second}

type OAuthCredentialsProvider struct {
	clientID     string
	clientSecret string
	serverURI    string
	tokenPath    string
	scope        string
	httpClient   *http.Client

	mu     sync.RWMutex
	token  string
	expiry time.Time

	// refreshMu serialises token refreshes so concurrent Header calls do not all
	// hit the token endpoint at once.
	refreshMu sync.Mutex
}

func NewOAuthCredentialsProvider(clientID, clientSecret, serverURI, tokenPath, scope string) *OAuthCredentialsProvider {
	return &OAuthCredentialsProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		serverURI:    strings.TrimRight(serverURI, "/"),
		tokenPath:    tokenPath,
		scope:        scope,
		httpClient:   tokenHTTPClient,
	}
}

func (p *OAuthCredentialsProvider) Header(ctx context.Context) (string, string, error) {
	if !p.tokenValid() {
		if err := p.refresh(ctx); err != nil {
			return "", "", fmt.Errorf("oauth token refresh failed: %w", err)
		}
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return "Authorization", "Bearer " + p.token, nil
}

func (p *OAuthCredentialsProvider) tokenValid() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.token != "" && time.Now().Before(p.expiry)
}

// refresh obtains a new token, serialising concurrent callers behind refreshMu
// and re-checking the cache once the lock is held so a token fetched by another
// goroutine in the meantime is reused.
func (p *OAuthCredentialsProvider) refresh(ctx context.Context) error {
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()

	if p.tokenValid() {
		return nil
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", p.clientID)
	data.Set("client_secret", p.clientSecret)
	if p.scope != "" {
		data.Set("scope", p.scope)
	}

	tokenURL := p.serverURI + "/" + strings.TrimLeft(p.tokenPath, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := p.httpClient
	if httpClient == nil {
		httpClient = tokenHTTPClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return err
	}
	if tr.AccessToken == "" {
		return fmt.Errorf("token endpoint returned empty access_token")
	}

	duration := time.Duration(tr.ExpiresIn) * time.Second
	if duration <= 0 {
		duration = 3600 * time.Second
	}
	refreshAfter := time.Duration(float64(duration) * 0.9)

	p.mu.Lock()
	p.token = tr.AccessToken
	p.expiry = time.Now().Add(refreshAfter)
	p.mu.Unlock()

	return nil
}
