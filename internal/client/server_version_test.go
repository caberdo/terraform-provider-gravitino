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

// TestDetectServerVersionRejectsUnparsableVersion asserts a server that reports
// something that is not a version leaves the client without a version, so the
// gates stay permissive instead of assuming an ancient server.
func TestDetectServerVersionRejectsUnparsableVersion(t *testing.T) {
	server := versionServer(t, "not-a-version")

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

// TestCheckMetadataObjectTypeSupportedReusesDetectedVersion asserts the 1.3.1
// object type check reuses the version detected at Configure instead of probing
// GET /api/version again, and keeps failing closed for VIEW and FUNCTION.
func TestCheckMetadataObjectTypeSupportedReusesDetectedVersion(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"version": map[string]string{"version": "1.3.0"},
		})
	}))
	t.Cleanup(server.Close)

	c, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := c.CheckMetadataObjectTypeSupported(context.Background(), "TABLE"); err != nil {
		t.Fatalf("CheckMetadataObjectTypeSupported(TABLE) = %v, want no error", err)
	}
	if requests != 0 {
		t.Errorf("an object type that exists since 1.3.0 must not probe the version, got %d requests", requests)
	}

	if err := c.CheckMetadataObjectTypeSupported(context.Background(), "VIEW"); err == nil {
		t.Fatal("CheckMetadataObjectTypeSupported(VIEW) = nil error, want an error on 1.3.0")
	}
	if requests != 1 {
		t.Errorf("requests = %d, want one probe when no version was detected", requests)
	}
	if got := c.ServerVersion(); got != "1.3.0" {
		t.Errorf("ServerVersion() = %q, want the probed version", got)
	}

	if err := c.CheckMetadataObjectTypeSupported(context.Background(), "FUNCTION"); err == nil {
		t.Fatal("CheckMetadataObjectTypeSupported(FUNCTION) = nil error, want an error on 1.3.0")
	}
	if requests != 1 {
		t.Errorf("a detected version must be reused, got %d requests", requests)
	}
}
