package metalake

import (
	"context"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMetalakeToState_NullPropertiesWithServerProperties(t *testing.T) {
	state := MetalakeResourceModel{
		Name:       types.StringValue("ml"),
		Properties: types.MapNull(types.StringType),
	}

	metalakeToState(&models.Metalake{
		Name:       "ml",
		Properties: map[string]string{"env": "dev"},
	}, &state, nil)

	if state.Properties.IsNull() || state.Properties.IsUnknown() {
		t.Fatalf("expected properties to be set, got null/unknown")
	}
	props := make(map[string]string)
	if d := state.Properties.ElementsAs(context.Background(), &props, false); d.HasError() {
		t.Fatalf("failed to read properties: %v", d)
	}
	if props["env"] != "dev" {
		t.Fatalf("expected env=dev, got %#v", props)
	}
}
