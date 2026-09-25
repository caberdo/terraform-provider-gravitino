package function

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// commentPlanModifier implements the semantics of the function comment:
//
//   - an empty string means "no comment" (the API stores no comment for an empty
//     value) and is planned as null, so `comment = ""` cannot end up as
//     "provider produced inconsistent result after apply";
//   - a comment is updated in place with the API's updateComment request;
//   - clearing a comment replaces the function, because the API's alter endpoint
//     rejects it: "New comment cannot be null or empty" even though
//     UpdateFunctionCommentRequest documents newComment as nullable.
type commentPlanModifier struct{}

func commentModifier() planmodifier.String {
	return commentPlanModifier{}
}

func (m commentPlanModifier) Description(_ context.Context) string {
	return "Normalises an empty comment to null and replaces the function when the comment is cleared."
}

func (m commentPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m commentPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	planned := req.PlanValue
	if planned.IsUnknown() {
		return
	}

	if !planned.IsNull() && planned.ValueString() == "" {
		resp.PlanValue = types.StringNull()
		planned = resp.PlanValue
	}

	if !planned.IsNull() {
		return
	}

	if req.StateValue.IsNull() {
		return
	}

	resp.RequiresReplace = true
	resp.PlanValue = types.StringNull()
	resp.Diagnostics.AddAttributeWarning(req.Path,
		"Clearing a function comment replaces the function",
		fmt.Sprintf("Gravitino cannot clear the comment of a function in place (the alter endpoint rejects a null comment), "+
			"so %s is replaced to drop the comment.", req.Path))
}
