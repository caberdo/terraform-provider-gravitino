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

func TestParseServerVersion(t *testing.T) {
	tests := []struct {
		in      string
		wantOK  bool
		wantMaj int
		wantMin int
		wantPat int
	}{
		{in: "1.3.0", wantOK: true, wantMaj: 1, wantMin: 3, wantPat: 0},
		{in: "1.3.1", wantOK: true, wantMaj: 1, wantMin: 3, wantPat: 1},
		{in: "v1.3.1", wantOK: true, wantMaj: 1, wantMin: 3, wantPat: 1},
		{in: "1.3.1-SNAPSHOT", wantOK: true, wantMaj: 1, wantMin: 3, wantPat: 1},
		{in: "1.3", wantOK: true, wantMaj: 1, wantMin: 3, wantPat: 0},
		{in: "2", wantOK: true, wantMaj: 2, wantMin: 0, wantPat: 0},
		{in: "", wantOK: false},
		{in: "unknown", wantOK: false},
		{in: "1.3.1.4", wantOK: false},
		{in: "1.x.0", wantOK: false},
	}
	for _, tt := range tests {
		got, ok := parseServerVersion(tt.in)
		if ok != tt.wantOK {
			t.Errorf("parseServerVersion(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if got.major != tt.wantMaj || got.minor != tt.wantMin || got.patch != tt.wantPat {
			t.Errorf("parseServerVersion(%q) = %d.%d.%d, want %d.%d.%d",
				tt.in, got.major, got.minor, got.patch, tt.wantMaj, tt.wantMin, tt.wantPat)
		}
		if got.raw != tt.in {
			t.Errorf("parseServerVersion(%q) raw = %q", tt.in, got.raw)
		}
	}
}

func TestClientServerVersion(t *testing.T) {
	c, err := New("http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Unknown until detected: gating callers must see "" and false.
	if got := c.ServerVersion(); got != "" {
		t.Fatalf("ServerVersion() = %q before detection, want \"\"", got)
	}
	if c.AtLeast(1, 3, 1) {
		t.Fatal("AtLeast() = true before detection, want false")
	}

	c.SetServerVersion("1.3.0")
	if got := c.ServerVersion(); got != "1.3.0" {
		t.Fatalf("ServerVersion() = %q, want 1.3.0", got)
	}
	if c.AtLeast(1, 3, 1) {
		t.Fatal("AtLeast(1,3,1) = true for a 1.3.0 server, want false")
	}
	if !c.AtLeast(1, 3, 0) {
		t.Fatal("AtLeast(1,3,0) = false for a 1.3.0 server, want true")
	}

	c.SetServerVersion("1.3.1-SNAPSHOT")
	if !c.AtLeast(1, 3, 1) {
		t.Fatal("AtLeast(1,3,1) = false for a 1.3.1-SNAPSHOT server, want true")
	}

	// An unparseable version must not overwrite a known one with garbage.
	c.SetServerVersion("not-a-version")
	if got := c.ServerVersion(); got != "1.3.1-SNAPSHOT" {
		t.Fatalf("ServerVersion() = %q after unparseable input, want the previous value", got)
	}

	c.SetServerVersion("2.0.0")
	if !c.AtLeast(1, 3, 1) {
		t.Fatal("AtLeast(1,3,1) = false for a 2.0.0 server, want true")
	}
}
