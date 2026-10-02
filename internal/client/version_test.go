package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

// TestCheckIndexesSupported covers the version gate for the Gravitino 1.3.1
// IndexSpec additions: the server version is queried only when an index uses a
// data-skipping type or carries custom properties.
func TestCheckIndexesSupported(t *testing.T) {
	tests := []struct {
		name        string
		server      string
		indexes     []models.Index
		wantErr     string
		wantVersion bool
	}{
		{
			name:        "1.3.0 with a data-skipping type",
			server:      "1.3.0",
			indexes:     []models.Index{{IndexType: "data_skipping_minmax", FieldNames: [][]string{{"id"}}}},
			wantErr:     `index type(s) data_skipping_minmax require Gravitino >= 1.3.1, but the server reports version "1.3.0"`,
			wantVersion: true,
		},
		{
			name:        "1.3.0 with index properties",
			server:      "1.3.0",
			indexes:     []models.Index{{IndexType: "unique_key", Properties: map[string]string{"granularity": "3"}}},
			wantErr:     `the index properties require Gravitino >= 1.3.1, but the server reports version "1.3.0"`,
			wantVersion: true,
		},
		{
			name:   "1.3.0 with both",
			server: "1.3.0",
			indexes: []models.Index{
				{IndexType: "unique_key"},
				{IndexType: "data_skipping_set", Properties: map[string]string{"granularity": "1"}},
			},
			wantErr:     `index type(s) data_skipping_set and the index properties require Gravitino >= 1.3.1`,
			wantVersion: true,
		},
		{
			name:        "1.3.1 with both",
			server:      "1.3.1",
			indexes:     []models.Index{{IndexType: "data_skipping_bloom_filter", Properties: map[string]string{"granularity": "1"}}},
			wantVersion: true,
		},
		{
			name:        "1.3.0 with 1.3.0 indexes only",
			server:      "1.3.0",
			indexes:     []models.Index{{IndexType: "primary_key", FieldNames: [][]string{{"id"}}}},
			wantVersion: false,
		},
		{
			name:        "no indexes",
			server:      "1.3.0",
			wantVersion: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			versionCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/version" {
					t.Errorf("unexpected path %q", r.URL.Path)
				}
				versionCalls++
				w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
				fmt.Fprintf(w, `{"code":0,"version":{"version":%q,"compileDate":"2026-01-01","gitCommit":"abc"}}`, tt.server)
			}))
			defer server.Close()

			c, err := New(server.URL, nil)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			err = c.CheckIndexesSupported(context.Background(), tt.indexes)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("CheckIndexesSupported() error = %v, want none", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("CheckIndexesSupported() = nil, want %q", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("CheckIndexesSupported() error = %q, want it to contain %q", err, tt.wantErr)
			}

			if gotVersion := versionCalls > 0; gotVersion != tt.wantVersion {
				t.Errorf("GET /api/version called = %v, want %v", gotVersion, tt.wantVersion)
			}
		})
	}
}

// TestGetVersion
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
