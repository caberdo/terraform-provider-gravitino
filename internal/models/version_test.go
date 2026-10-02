package models

import "testing"

func TestServerVersionAtLeast(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"1.3.1", true},
		{"1.3.0", false},
		{"1.2.9", false},
		{"1.4.0", true},
		{"2.0.0", true},
		{"v1.3.1", true},
		{"1.3.1-incubating", true},
		{"1.4.0-SNAPSHOT", true},
		{"1.3.1+build7", true},
		{" 1.3.1 ", true},
		{"1.3", false},
		{"1", false},
		{"", false},
		{"not-a-version", false},
		{"1.x.1", false},
	}

	for _, tc := range cases {
		if got := ServerVersionAtLeast(tc.version, 1, 3, 1); got != tc.want {
			t.Errorf("ServerVersionAtLeast(%q, 1, 3, 1) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestObjectTypeRequiresGravitino131(t *testing.T) {
	for _, objectType := range []string{"VIEW", "FUNCTION", "view", " function "} {
		if !ObjectTypeRequiresGravitino131(objectType) {
			t.Errorf("ObjectTypeRequiresGravitino131(%q) = false, want true", objectType)
		}
	}
	for _, objectType := range []string{"TABLE", "COLUMN", "MODEL", "ROLE", "METALAKE", "CATALOG", "SCHEMA", "FILESET", "TOPIC", ""} {
		if ObjectTypeRequiresGravitino131(objectType) {
			t.Errorf("ObjectTypeRequiresGravitino131(%q) = true, want false", objectType)
		}
	}
}

// TestStatisticsObjectTypesMatchSpec pins the statistics metadataObjectType
// enum to the Gravitino v1.3.1 openapi.yaml parameter order.
func TestStatisticsObjectTypesMatchSpec(t *testing.T) {
	want := []string{
		"METALAKE", "CATALOG", "SCHEMA", "TABLE", "VIEW", "COLUMN",
		"FILESET", "TOPIC", "MODEL", "FUNCTION", "ROLE",
	}
	if len(StatisticsObjectTypes) != len(want) {
		t.Fatalf("StatisticsObjectTypes has %d values, want %d: %v", len(StatisticsObjectTypes), len(want), StatisticsObjectTypes)
	}
	for i := range want {
		if StatisticsObjectTypes[i] != want[i] {
			t.Errorf("StatisticsObjectTypes[%d] = %q, want %q", i, StatisticsObjectTypes[i], want[i])
		}
	}
}

// TestCredentialObjectTypesMatchSpec pins the credentials metadataObjectType
// enum to the Gravitino v1.3.1 openapi.yaml parameter order.
func TestCredentialObjectTypesMatchSpec(t *testing.T) {
	want := []string{
		"METALAKE", "CATALOG", "SCHEMA", "TABLE", "VIEW", "COLUMN",
		"FILESET", "TOPIC", "MODEL", "FUNCTION", "ROLE",
	}
	if len(CredentialObjectTypes) != len(want) {
		t.Fatalf("CredentialObjectTypes has %d values, want %d: %v", len(CredentialObjectTypes), len(want), CredentialObjectTypes)
	}
	for i := range want {
		if CredentialObjectTypes[i] != want[i] {
			t.Errorf("CredentialObjectTypes[%d] = %q, want %q", i, CredentialObjectTypes[i], want[i])
		}
	}
}
