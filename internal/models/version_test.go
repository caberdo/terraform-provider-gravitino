package models

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		version string
		major   int
		minor   int
		patch   int
		ok      bool
	}{
		{version: "1.3.1", major: 1, minor: 3, patch: 1, ok: true},
		{version: "1.3.0", major: 1, minor: 3, patch: 0, ok: true},
		{version: "1.3.0-incubating", major: 1, minor: 3, patch: 0, ok: true},
		{version: "1.4.0-SNAPSHOT", major: 1, minor: 4, patch: 0, ok: true},
		{version: "1.4.0+7f3c1a", major: 1, minor: 4, patch: 0, ok: true},
		{version: "2.0.10", major: 2, minor: 0, patch: 10, ok: true},
		{version: "", ok: false},
		{version: "1.3", ok: false},
		{version: "1.3.1.2", ok: false},
		{version: "v1.3.1", ok: false},
		{version: "dev", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			major, minor, patch, ok := ParseVersion(tt.version)
			if ok != tt.ok {
				t.Fatalf("ParseVersion(%q) ok = %t, want %t", tt.version, ok, tt.ok)
			}
			if !tt.ok {
				return
			}
			if major != tt.major || minor != tt.minor || patch != tt.patch {
				t.Fatalf("ParseVersion(%q) = %d.%d.%d, want %d.%d.%d", tt.version, major, minor, patch, tt.major, tt.minor, tt.patch)
			}
		})
	}
}
