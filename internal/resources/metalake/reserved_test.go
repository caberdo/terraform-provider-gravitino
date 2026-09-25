package metalake

import (
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

// TestPropertyUpdates_SkipsReservedProperties: 'in-use' is managed by Gravitino
// through PATCH /metalakes/{name} and must never be sent as a regular
// setProperty/removeProperty update.
func TestPropertyUpdates_SkipsReservedProperties(t *testing.T) {
	r := &MetalakeResource{}

	updates := r.propertyUpdates(
		map[string]string{"in-use": "true", "env": "dev", "gone": "x"},
		map[string]string{"in-use": "false", "env": "prod"},
	)

	var sawSet, sawRemove bool
	for _, u := range updates {
		switch v := u.(type) {
		case models.SetMetalakePropertyRequest:
			if v.Property == "in-use" {
				t.Fatalf("setProperty must not touch the reserved property: %#v", v)
			}
			if v.Property != "env" || v.Value != "prod" {
				t.Fatalf("unexpected setProperty update: %#v", v)
			}
			sawSet = true
		case models.RemoveMetalakePropertyRequest:
			if v.Property == "in-use" {
				t.Fatalf("removeProperty must not touch the reserved property: %#v", v)
			}
			if v.Property != "gone" {
				t.Fatalf("unexpected removeProperty update: %#v", v)
			}
			sawRemove = true
		default:
			t.Fatalf("unexpected update type %T", u)
		}
	}

	if !sawSet || !sawRemove {
		t.Fatalf("expected both a setProperty and a removeProperty update, got %#v", updates)
	}
}

// TestFilterReservedProperties keeps the request-side filter honest: reserved
// keys never leave the provider towards the API.
func TestFilterReservedProperties(t *testing.T) {
	got := filterReservedProperties(map[string]string{"in-use": "true", "env": "dev"})
	if len(got) != 1 || got["env"] != "dev" {
		t.Fatalf("expected only env=dev, got %#v", got)
	}
}
