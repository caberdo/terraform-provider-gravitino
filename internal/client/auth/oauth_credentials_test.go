package auth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client/auth"
)

func TestOAuthCredentialsProvider_Header(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Fatalf("expected client_credentials, got %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("client_id") != "my-client" {
			t.Fatalf("expected my-client, got %s", r.Form.Get("client_id"))
		}
		if r.Form.Get("client_secret") != "my-secret" {
			t.Fatalf("expected my-secret, got %s", r.Form.Get("client_secret"))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-token-123",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	p := auth.NewOAuthCredentialsProvider("my-client", "my-secret", tokenServer.URL, "/token", "")

	key, value, err := p.Header(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if key != "Authorization" {
		t.Fatalf("expected Authorization, got %s", key)
	}
	if value != "Bearer test-token-123" {
		t.Fatalf("expected 'Bearer test-token-123', got %s", value)
	}
}

func TestOAuthCredentialsProvider_CachesToken(t *testing.T) {
	requestCount := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": fmt.Sprintf("token-%d", requestCount),
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	p := auth.NewOAuthCredentialsProvider("c", "s", tokenServer.URL, "/token", "")

	// First call should get a token
	v1, _, _ := p.Header(context.Background())

	// Second call should use cache (requestCount stays 1)
	v2, _, _ := p.Header(context.Background())

	if v1 != v2 {
		t.Fatalf("expected cached token, got %s then %s", v1, v2)
	}
	if requestCount != 1 {
		t.Fatalf("expected 1 token request, got %d", requestCount)
	}
}

func TestOAuthCredentialsProvider_RefreshesToken(t *testing.T) {
	requestCount := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": fmt.Sprintf("token-%d", requestCount),
			"expires_in":   1, // 1 second — refresh at 0.9s
		})
	}))
	defer tokenServer.Close()

	p := auth.NewOAuthCredentialsProvider("c", "s", tokenServer.URL, "/token", "")

	p.Header(context.Background())

	// Sleep past the 90% refresh margin (1s * 0.9 = 0.9s)
	time.Sleep(950 * time.Millisecond)

	p.Header(context.Background())

	if requestCount != 2 {
		t.Fatalf("expected 2 token requests (expired), got %d", requestCount)
	}
}

func TestOAuthCredentialsProvider_SingleFlightRefresh(t *testing.T) {
	var mu sync.Mutex
	requestCount := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "shared-token",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	p := auth.NewOAuthCredentialsProvider("c", "s", tokenServer.URL, "/token", "")

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if _, _, err := p.Header(context.Background()); err != nil {
				t.Errorf("Header() error = %v", err)
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if requestCount != 1 {
		t.Fatalf("token requests = %d, want 1 (single-flight refresh)", requestCount)
	}
}

func TestOAuthCredentialsProvider_TokenEndpointError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer tokenServer.Close()

	p := auth.NewOAuthCredentialsProvider("bad", "creds", tokenServer.URL, "/token", "")
	_, _, err := p.Header(context.Background())
	if err == nil {
		t.Fatal("expected error from bad credentials")
	}
}
