package metalake

import (
	"context"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func propsOf(t *testing.T, m types.Map) map[string]string {
	t.Helper()
	out := make(map[string]string)
	if d := m.ElementsAs(context.Background(), &out, false); d.HasError() {
		t.Fatalf("failed to read properties: %v", d)
	}
	return out
}

// TestMetalakeToState_KeepsConfiguredProperties: a configured key keeps the
// server's value while server-only keys are never added. `properties` is
// Optional+Computed, so the applied value must equal the planned value.
func TestMetalakeToState_KeepsConfiguredProperties(t *testing.T) {
	ctx := context.Background()
	diags := &diag.Diagnostics{}
	state := MetalakeResourceModel{
		Name:       types.StringValue("ml"),
		Properties: types.MapValueMust(types.StringType, map[string]attr.Value{"env": types.StringValue("dev")}),
	}

	metalakeToState(ctx, &models.Metalake{
		Name:       "ml",
		Properties: map[string]string{"env": "production", "in-use": "true", "gravitino.identifier": "uid1"},
	}, &state, diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	got := propsOf(t, state.Properties)
	if len(got) != 1 || got["env"] != "production" {
		t.Fatalf("expected exactly the configured key with the server value, got %#v", got)
	}
}

// TestMetalakeToState_NullPlanAdoptsServerProperties: a null plan means "no
// configured properties" — that happens while importing, where the server's
// properties must be reconstructed, minus the keys Gravitino manages itself.
func TestMetalakeToState_NullPlanAdoptsServerProperties(t *testing.T) {
	ctx := context.Background()

	diags := &diag.Diagnostics{}
	state := MetalakeResourceModel{
		Name:       types.StringValue("ml"),
		Properties: types.MapNull(types.StringType),
	}
	metalakeToState(ctx, &models.Metalake{Name: "ml", Properties: map[string]string{"in-use": "true", "gravitino.identifier": "uid1", "env": "dev"}}, &state, diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := propsOf(t, state.Properties); len(got) != 1 || got["env"] != "dev" {
		t.Fatalf("expected only env=dev, got %#v", got)
	}

	// Only server-managed keys: nothing to adopt, stay null.
	diags = &diag.Diagnostics{}
	onlyReserved := MetalakeResourceModel{
		Name:       types.StringValue("ml"),
		Properties: types.MapNull(types.StringType),
	}
	metalakeToState(ctx, &models.Metalake{Name: "ml", Properties: map[string]string{"in-use": "true"}}, &onlyReserved, diags)
	if !onlyReserved.Properties.IsNull() {
		t.Fatalf("expected null properties, got %v", onlyReserved.Properties)
	}
}

// TestMetalakeToState_EmptyPlanStaysEmpty: a configured empty map must stay an
// empty map (not null), otherwise Terraform reports an inconsistent result.
func TestMetalakeToState_EmptyPlanStaysEmpty(t *testing.T) {
	ctx := context.Background()
	diags := &diag.Diagnostics{}
	state := MetalakeResourceModel{
		Name:       types.StringValue("ml"),
		Properties: types.MapValueMust(types.StringType, map[string]attr.Value{}),
	}

	metalakeToState(ctx, &models.Metalake{
		Name:       "ml",
		Properties: map[string]string{"in-use": "true"},
	}, &state, diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.Properties.IsNull() || state.Properties.IsUnknown() {
		t.Fatalf("expected an empty (known) map, got %v", state.Properties)
	}
	if len(propsOf(t, state.Properties)) != 0 {
		t.Fatalf("expected no properties, got %v", state.Properties)
	}
}

// TestMetalakeToState_KeepsKeyTheServerDidNotEcho: a configured key that the
// server does not return must keep its planned value instead of disappearing.
func TestMetalakeToState_KeepsKeyTheServerDidNotEcho(t *testing.T) {
	ctx := context.Background()
	diags := &diag.Diagnostics{}
	state := MetalakeResourceModel{
		Name:       types.StringValue("ml"),
		Properties: types.MapValueMust(types.StringType, map[string]attr.Value{"env": types.StringValue("dev")}),
	}

	metalakeToState(ctx, &models.Metalake{Name: "ml"}, &state, diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got := propsOf(t, state.Properties); got["env"] != "dev" {
		t.Fatalf("expected env=dev to survive, got %#v", got)
	}
}

// TestMetalakeToState_CommentAlwaysKnown: an unconfigured comment (unknown in
// the plan) must become a known value, otherwise Terraform fails with
// "Provider returned invalid result object after apply".
func TestMetalakeToState_CommentAlwaysKnown(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name    string
		server  string
		planned types.String
		want    types.String
	}{
		{"unknown plan and omitted server comment", "", types.StringUnknown(), types.StringNull()},
		{"null plan and omitted server comment", "", types.StringNull(), types.StringNull()},
		{"known plan survives an omitted server comment", "", types.StringValue("keep me"), types.StringValue("keep me")},
		{"server comment wins", "from server", types.StringUnknown(), types.StringValue("from server")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			state := MetalakeResourceModel{
				Name:       types.StringValue("ml"),
				Comment:    tc.planned,
				Properties: types.MapNull(types.StringType),
			}
			metalakeToState(ctx, &models.Metalake{Name: "ml", Comment: tc.server}, &state, diags)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if !state.Comment.Equal(tc.want) {
				t.Fatalf("expected comment %v, got %v", tc.want, state.Comment)
			}
			if state.Comment.IsUnknown() {
				t.Fatal("comment must never be unknown in state")
			}
		})
	}
}
