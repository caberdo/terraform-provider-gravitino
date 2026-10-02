package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func versionServer(t *testing.T, version string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"version": map[string]string{"version": version},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestParseServerVersion(t *testing.T) {
	valid := map[string]serverVersion{
		"1.3.0":              {major: 1, minor: 3},
		"1.3.1":              {major: 1, minor: 3, patch: 1},
		"1.3":                {major: 1, minor: 3},
		"1.3.1-SNAPSHOT":     {major: 1, minor: 3, patch: 1},
		" 1.3.0-incubating ": {major: 1, minor: 3},
	}
	for raw, want := range valid {
		got, err := parseServerVersion(raw)
		if err != nil {
			t.Errorf("parseServerVersion(%q) = %v, want no error", raw, err)
			continue
		}
		if *got != want {
			t.Errorf("parseServerVersion(%q) = %#v, want %#v", raw, *got, want)
		}
	}

	for _, raw := range []string{"", "   ", "abc", "1.3.x", "1.2.3.4", "-SNAPSHOT"} {
		if _, err := parseServerVersion(raw); err == nil {
			t.Errorf("parseServerVersion(%q) = nil error, want an error", raw)
		}
	}
}

func TestDetectServerVersion(t *testing.T) {
	server := versionServer(t, "1.3.0")

	c, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if got := c.ServerVersion(); got != "" {
		t.Errorf("ServerVersion() before detection = %q, want empty", got)
	}

	if err := c.DetectServerVersion(context.Background()); err != nil {
		t.Fatalf("DetectServerVersion() error = %v", err)
	}

	if got := c.ServerVersion(); got != "1.3.0" {
		t.Errorf("ServerVersion() = %q, want 1.3.0", got)
	}
	if !c.AtLeast(1, 3, 0) {
		t.Error("AtLeast(1, 3, 0) = false, want true on 1.3.0")
	}
	if c.AtLeast(1, 3, 1) {
		t.Error("AtLeast(1, 3, 1) = true, want false on 1.3.0")
	}
	if c.SupportsExternalType() {
		t.Error("SupportsExternalType() = true, want false on 1.3.0")
	}
}

func TestDetectServerVersionEnablesExternalTypeOn131(t *testing.T) {
	server := versionServer(t, "1.3.1")

	c, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.DetectServerVersion(context.Background()); err != nil {
		t.Fatalf("DetectServerVersion() error = %v", err)
	}

	if !c.SupportsExternalType() {
		t.Error("SupportsExternalType() = false, want true on 1.3.1")
	}
	if c.AtLeast(1, 4, 0) {
		t.Error("AtLeast(1, 4, 0) = true, want false on 1.3.1")
	}
}

// TestVersionGatesArePermissiveWhenDetectionFails asserts a failed probe does
// not reject features: an unknown server version is treated as current.
func TestVersionGatesArePermissiveWhenDetectionFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	c, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := c.DetectServerVersion(context.Background()); err == nil {
		t.Fatal("DetectServerVersion() = nil error, want an error")
	}

	if got := c.ServerVersion(); got != "" {
		t.Errorf("ServerVersion() = %q, want empty", got)
	}
	if !c.SupportsExternalType() {
		t.Error("SupportsExternalType() = false, want true when the version is unknown")
	}
	if !c.AtLeast(99, 0, 0) {
		t.Error("AtLeast(99, 0, 0) = false, want true when the version is unknown")
	}
}
