package table

import (
	"context"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMapTableResponseToState_NullPropertiesWithServerProperties(t *testing.T) {
	state := models.TableResourceModel{
		Name:       types.StringValue("tbl"),
		Properties: types.MapNull(types.StringType),
	}

	diags := mapTableResponseToState(context.Background(), &models.TableResponse{
		Table: models.Table{
			Name:       "tbl",
			Properties: map[string]string{"env": "dev"},
		},
	}, &state)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
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

func TestMapTableResponseToState_EmptyConfigProperties(t *testing.T) {
	props, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]string{})
	state := models.TableResourceModel{
		Name:       types.StringValue("tbl"),
		Properties: props,
	}

	diags := mapTableResponseToState(context.Background(), &models.TableResponse{
		Table: models.Table{
			Name: "tbl",
		},
	}, &state)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.Properties.IsNull() {
		t.Fatalf("expected empty map to be preserved, got null")
	}
}

func TestMapTableResponseToState_FiltersReservedProperties(t *testing.T) {
	state := models.TableResourceModel{
		Name:       types.StringValue("tbl"),
		Properties: types.MapNull(types.StringType),
	}

	diags := mapTableResponseToState(context.Background(), &models.TableResponse{
		Table: models.Table{
			Name:       "tbl",
			Properties: map[string]string{"in-use": "true", "env": "dev"},
		},
	}, &state)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
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
