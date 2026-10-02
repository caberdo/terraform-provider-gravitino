package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"version": map[string]string{"version": "1.3.0", "compileDate": "28/06/2026", "gitCommit": "abc"},
		})
	}))
	defer server.Close()

	c, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := c.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}
	if got.Code != 0 {
		t.Fatalf("Code = %d, want 0", got.Code)
	}
	if got.Version.Version != "1.3.0" {
		t.Fatalf("Version.Version = %q, want 1.3.0", got.Version.Version)
	}
	if got.Version.CompileDate != "28/06/2026" {
		t.Fatalf("Version.CompileDate = %q", got.Version.CompileDate)
	}
}
