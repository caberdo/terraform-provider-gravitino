package table

import (
	"context"
	"reflect"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/tfutil"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ModifyPlan canonicalises the planned column types and comments, and forces a
// replacement for the changes Gravitino cannot apply in place.
func (r *tableResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var tfPlan, tfState models.TableResourceTFModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &tfPlan)...)
	hasState := !req.State.Raw.IsNull()
	if hasState {
		resp.Diagnostics.Append(req.State.Get(ctx, &tfState)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	plan := models.TableResourceModelFromTF(ctx, tfPlan, &resp.Diagnostics)
	state := models.TableResourceModelFromTF(ctx, tfState, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	canonicalisePlan(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	tfPlan.Comment = plan.Comment
	// Only write the columns back when they are known: an unknown list is
	// decoded as empty above and must stay unknown in the plan.
	if !tfPlan.Columns.IsNull() && !tfPlan.Columns.IsUnknown() {
		tfPlan.Columns = models.TableColumnsToList(ctx, plan.Columns, &resp.Diagnostics)
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &tfPlan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !hasState {
		return
	}

	if (!tfPlan.SortOrders.IsUnknown() && sortOrdersChanged(plan.SortOrders, state.SortOrders)) ||
		(!tfPlan.Distribution.IsUnknown() && distributionChanged(plan.Distribution, state.Distribution)) ||
		(!tfPlan.Partitioning.IsUnknown() && partitioningChanged(plan.Partitioning, state.Partitioning)) ||
		(!tfPlan.Indexes.IsUnknown() && indexesChanged(plan.Indexes, state.Indexes)) {
		for _, name := range []string{"sort_order", "distribution", "partitioning", "index"} {
			resp.RequiresReplace = append(resp.RequiresReplace, path.Root(name))
		}
	}

	if !tfPlan.Columns.IsUnknown() && columnsReplaceRequired(plan.Columns, state.Columns) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("column"))
	}
}

// canonicalisePlan makes the plan deterministic before it is compared with the
// state: column types are rewritten to their canonical form and empty comments
// become unset, matching the values Gravitino reports back.

// canonicalisePlan makes the plan deterministic before it is compared with the
// state: column types are rewritten to their canonical form and empty comments
// become unset, matching the values Gravitino reports back.
func canonicalisePlan(ctx context.Context, plan *models.TableResourceModel, diags *diag.Diagnostics) {
	if !plan.Comment.IsNull() && !plan.Comment.IsUnknown() && plan.Comment.ValueString() == "" {
		plan.Comment = types.StringNull()
	}

	for i := range plan.Columns {
		column := &plan.Columns[i]

		if !column.Comment.IsNull() && !column.Comment.IsUnknown() && column.Comment.ValueString() == "" {
			column.Comment = types.StringNull()
		}

		if column.Type.IsNull() || column.Type.IsUnknown() {
			continue
		}

		dataType, err := models.ParseDataType(column.Type.ValueString())
		if err != nil {
			diags.AddAttributeError(
				path.Root("column").AtListIndex(i).AtName("type"),
				"Invalid column type",
				err.Error(),
			)
			continue
		}

		if canonical := dataType.String(); canonical != column.Type.ValueString() {
			column.Type = types.StringValue(canonical)
		}
	}
}

// Create creates the table.

// columnsReplaceRequired reports whether the column change needs a new table:
// the set of column names changed, or a default value was removed. Gravitino
// cannot tell a renamed column from a deleted and added one, and it cannot
// clear a column default value.
func columnsReplaceRequired(plan, state []models.ColumnTFSDK) bool {
	planned := make(map[string]models.ColumnTFSDK, len(plan))
	for _, column := range plan {
		if column.Name.IsUnknown() {
			return false
		}
		planned[column.Name.ValueString()] = column
	}

	if len(planned) != len(state) {
		return true
	}

	for _, column := range state {
		desired, exists := planned[column.Name.ValueString()]
		if !exists {
			return true
		}

		hasDefault := !column.DefaultValue.IsNull() && !column.DefaultValue.IsUnknown()
		keepsDefault := !desired.DefaultValue.IsNull() && !desired.DefaultValue.IsUnknown()
		if hasDefault && !keepsDefault {
			return true
		}
	}

	return false
}

// immutableBlockChange reports the first block Gravitino cannot update in place.

// immutableBlockChange reports the first block Gravitino cannot update in place.
func immutableBlockChange(plan, state models.TableResourceModel) string {
	if sortOrdersChanged(plan.SortOrders, state.SortOrders) {
		return "sort_order"
	}
	if distributionChanged(plan.Distribution, state.Distribution) {
		return "distribution"
	}
	if partitioningChanged(plan.Partitioning, state.Partitioning) {
		return "partitioning"
	}
	if indexesChanged(plan.Indexes, state.Indexes) {
		return "index"
	}
	return ""
}

// The changed helpers compare the blocks through the API model, so that a
// cosmetic difference in the Terraform representation is not treated as a
// change. Unknown planned values are not compared: tfutil.FullyKnown is the
// single predicate that decides whether a block is still waiting for apply, and
// the Update method resolves it before writing state.

func sortOrdersChanged(plan, state []models.SortOrderTFSDK) bool {
	if !tfutil.FullyKnown(plan) {
		return false
	}

	planned, planDiags := models.TableSortOrdersFromModel(context.Background(), plan)
	current, stateDiags := models.TableSortOrdersFromModel(context.Background(), state)
	if planDiags.HasError() || stateDiags.HasError() {
		return false
	}
	return !reflect.DeepEqual(planned, current)
}

func distributionChanged(plan, state *models.DistributionTFSDK) bool {
	if !tfutil.FullyKnown(plan) {
		return false
	}

	planned, planDiags := models.TableDistributionFromModel(context.Background(), plan)
	current, stateDiags := models.TableDistributionFromModel(context.Background(), state)
	if planDiags.HasError() || stateDiags.HasError() {
		return false
	}
	if planned == nil || current == nil {
		return planned != current
	}
	return !reflect.DeepEqual(*planned, *current)
}

func partitioningChanged(plan, state []models.PartitioningTFSDK) bool {
	if !tfutil.FullyKnown(plan) {
		return false
	}

	planned, planDiags := models.TablePartitioningFromModel(context.Background(), plan)
	current, stateDiags := models.TablePartitioningFromModel(context.Background(), state)
	if planDiags.HasError() || stateDiags.HasError() {
		return false
	}
	return !reflect.DeepEqual(planned, current)
}

func indexesChanged(plan, state []models.IndexTFSDK) bool {
	// The name is skipped: it is unknown whenever the configuration omits it,
	// because a catalog assigns it in that case, so treating it as a pending
	// value would hide every other change to the index.
	if !tfutil.FullyKnown(plan, "name") {
		return false
	}

	planned, planDiags := models.TableIndexesFromModel(context.Background(), plan)
	current, stateDiags := models.TableIndexesFromModel(context.Background(), state)
	if planDiags.HasError() || stateDiags.HasError() {
		return false
	}

	// A catalog assigns the name of an index whose configuration omits it, so
	// the planned name is unknown then. Comparing it would report a change that
	// is not there, while skipping the whole index would hide a real change to
	// the index type, the field names or the properties.
	for i := range planned {
		if i < len(plan) && i < len(current) && plan[i].Name.IsUnknown() {
			planned[i].Name = current[i].Name
		}
	}

	return !reflect.DeepEqual(planned, current)
}
