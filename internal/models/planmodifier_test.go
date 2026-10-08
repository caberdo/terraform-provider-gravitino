package models

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type compoundIDModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Name     types.String `tfsdk:"name"`
	ID       types.String `tfsdk:"id"`
}

// TestCompoundIDPlanModifier asserts the compound id is kept while its
// components are unchanged, marked unknown when a component changes, and marked
// unknown on create (null state).
func TestCompoundIDPlanModifier(t *testing.T) {
	ctx := context.Background()

	s := schema.Schema{Attributes: map[string]schema.Attribute{
		"metalake": schema.StringAttribute{Optional: true},
		"name":     schema.StringAttribute{Optional: true},
		"id":       schema.StringAttribute{Optional: true, Computed: true},
	}}

	modifier := CompoundID("metalake", "name")

	cases := []struct {
		name        string
		state       compoundIDModel
		plan        compoundIDModel
		wantUnknown bool
	}{
		{
			name:        "unchanged keeps the id",
			state:       compoundIDModel{Metalake: types.StringValue("ml"), Name: types.StringValue("a"), ID: types.StringValue("ml.a")},
			plan:        compoundIDModel{Metalake: types.StringValue("ml"), Name: types.StringValue("a"), ID: types.StringValue("ml.a")},
			wantUnknown: false,
		},
		{
			name:        "renamed marks the id unknown",
			state:       compoundIDModel{Metalake: types.StringValue("ml"), Name: types.StringValue("a"), ID: types.StringValue("ml.a")},
			plan:        compoundIDModel{Metalake: types.StringValue("ml"), Name: types.StringValue("b"), ID: types.StringValue("ml.b")},
			wantUnknown: true,
		},
		{
			name:        "create marks the id unknown",
			state:       compoundIDModel{Metalake: types.StringNull(), Name: types.StringNull(), ID: types.StringNull()},
			plan:        compoundIDModel{Metalake: types.StringValue("ml"), Name: types.StringValue("a"), ID: types.StringUnknown()},
			wantUnknown: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := tfsdk.State{Schema: s}
			if diags := state.Set(ctx, &tc.state); diags.HasError() {
				t.Fatalf("failed to set state: %v", diags)
			}
			plan := tfsdk.Plan{Schema: s}
			if diags := plan.Set(ctx, &tc.plan); diags.HasError() {
				t.Fatalf("failed to set plan: %v", diags)
			}

			req := planmodifier.StringRequest{
				State:      state,
				Plan:       plan,
				Config:     tfsdk.Config{Schema: s},
				StateValue: tc.state.ID,
				PlanValue:  tc.plan.ID,
			}
			resp := &planmodifier.StringResponse{PlanValue: tc.plan.ID}

			modifier.PlanModifyString(ctx, req, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
			if resp.PlanValue.IsUnknown() != tc.wantUnknown {
				t.Fatalf("id unknown = %v, want %v", resp.PlanValue.IsUnknown(), tc.wantUnknown)
			}
		})
	}
}
