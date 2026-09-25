package models

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// CompoundIDPlanModifier keeps the computed compound id of a resource in state
// while all of its components are unchanged, and marks the id unknown when a
// component changes so the id can be recomputed during apply.
//
// It replaces stringplanmodifier.UseStateForUnknown for compound ids: because
// Gravitino supports renaming in place, a UseStateForUnknown modifier would
// keep the old id in the plan while apply writes the new one, which Terraform
// reports as "Provider produced inconsistent result after apply".
type CompoundIDPlanModifier struct {
	// Components are the top level attributes the id is built from.
	Components []string
}

// CompoundID returns a plan modifier for a compound id built from the given
// top level attributes.
func CompoundID(components ...string) planmodifier.String {
	return CompoundIDPlanModifier{Components: components}
}

// Description describes the plan modifier.
func (m CompoundIDPlanModifier) Description(_ context.Context) string {
	return "Keeps the compound id while its components are unchanged and marks it unknown when a component changes."
}

// MarkdownDescription describes the plan modifier in Markdown.
func (m CompoundIDPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

// PlanModifyString implements planmodifier.String.
func (m CompoundIDPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() {
		resp.PlanValue = types.StringUnknown()
		return
	}

	for _, component := range m.Components {
		var stateValue, planValue types.String
		if req.State.GetAttribute(ctx, path.Root(component), &stateValue).HasError() ||
			req.Plan.GetAttribute(ctx, path.Root(component), &planValue).HasError() {
			resp.PlanValue = types.StringUnknown()
			return
		}
		if !stateValue.Equal(planValue) {
			resp.PlanValue = types.StringUnknown()
			return
		}
	}

	resp.PlanValue = req.StateValue
}
