package metalake

import (
	"context"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMetalakeToState_FiltersReservedProperties(t *testing.T) {
	state := MetalakeResourceModel{
		Name:       types.StringValue("ml"),
		Properties: types.MapNull(types.StringType),
	}

	metalakeToState(&models.Metalake{
		Name:       "ml",
		Properties: map[string]string{"in-use": "true", "env": "dev"},
	}, &state, nil)

	props := make(map[string]string)
	if d := state.Properties.ElementsAs(context.Background(), &props, false); d.HasError() {
		t.Fatalf("failed to read properties: %v", d)
	}
	if _, ok := props["in-use"]; ok {
		t.Fatalf("reserved property 'in-use' must not appear in state, got %#v", props)
	}
	if props["env"] != "dev" {
		t.Fatalf("expected env=dev preserved, got %#v", props)
	}
}

func TestMetalakeResource_UpdateSkipsReservedProperties(t *testing.T) {
	r := &MetalakeResource{}

	var updates []interface{}
	oldProps := map[string]string{"in-use": "true", "env": "dev"}
	newProps := map[string]string{"env": "dev"}

	updates = r.propertyUpdates(oldProps, newProps)

	for _, u := range updates {
		m, ok := u.(map[string]interface{})
		if !ok {
			continue
		}
		if m["property"] == "in-use" {
			t.Fatalf("update must not touch reserved property 'in-use', got %#v", updates)
		}
	}
}
