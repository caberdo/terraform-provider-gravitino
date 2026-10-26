package table

import (
	"context"
	"reflect"
	"sort"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// buildCreateRequest converts the planned table into a TableCreateRequest.
func (r *tableResource) buildCreateRequest(ctx context.Context, plan *models.TableResourceModel) (*models.TableCreateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	properties, propertyDiags := models.TablePropertiesFromModel(ctx, plan.Properties)
	diags.Append(propertyDiags...)

	columns, columnDiags := models.TableColumnsFromModel(plan.Columns)
	diags.Append(columnDiags...)

	sortOrders, sortOrderDiags := models.TableSortOrdersFromModel(ctx, plan.SortOrders)
	diags.Append(sortOrderDiags...)

	distribution, distributionDiags := models.TableDistributionFromModel(ctx, plan.Distribution)
	diags.Append(distributionDiags...)

	partitioning, partitioningDiags := models.TablePartitioningFromModel(ctx, plan.Partitioning)
	diags.Append(partitioningDiags...)

	indexes, indexDiags := models.TableIndexesFromModel(ctx, plan.Indexes)
	diags.Append(indexDiags...)
	if diags.HasError() {
		return nil, diags
	}

	return &models.TableCreateRequest{
		Name:         plan.Name.ValueString(),
		Comment:      plan.Comment.ValueString(),
		Properties:   filterReservedProperties(properties),
		Columns:      columns,
		SortOrders:   sortOrders,
		Distribution: distribution,
		Partitioning: partitioning,
		Indexes:      indexes,
	}, diags
}

// tableStateFromServer refreshes the model from a table reported by Gravitino.
// preferServerProperties selects whether the properties reported by Gravitino
// or the properties already in the model win for keys that both know.

// tablePropertyUpdates diffs the managed properties into setProperty and
// removeProperty updates. The configuration is the complete set of managed
// properties, so a key that is no longer configured is removed.
func tablePropertyUpdates(ctx context.Context, plan, state types.Map) ([]models.TableUpdateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	planned, planDiags := models.TablePropertiesFromModel(ctx, plan)
	diags.Append(planDiags...)

	current, stateDiags := models.TablePropertiesFromModel(ctx, state)
	diags.Append(stateDiags...)
	if diags.HasError() {
		return nil, diags
	}

	var updates []models.TableUpdateRequest

	for key, value := range planned {
		if reservedProperties[key] {
			continue
		}
		if oldValue, exists := current[key]; !exists || oldValue != value {
			updates = append(updates, models.NewSetTablePropertyRequest(key, value))
		}
	}

	for key := range current {
		if reservedProperties[key] {
			continue
		}
		if _, exists := planned[key]; !exists {
			updates = append(updates, models.NewRemoveTablePropertyRequest(key))
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		if updates[i].Property != updates[j].Property {
			return updates[i].Property < updates[j].Property
		}
		return updates[i].Type < updates[j].Type
	})

	return updates, diags
}

// tableColumnUpdates builds the column updates of an existing column.
//
// Columns are matched by name: a change to the set of names needs a new table
// (see columnsReplaceRequired), so only the updatable properties of a column
// are turned into requests here.

// tableColumnUpdates builds the column updates of an existing column.
//
// Columns are matched by name: a change to the set of names needs a new table
// (see columnsReplaceRequired), so only the updatable properties of a column
// are turned into requests here.
func tableColumnUpdates(plan, state []models.ColumnTFSDK) ([]models.TableUpdateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	planned := make(map[string]models.ColumnTFSDK, len(plan))
	for _, column := range plan {
		planned[column.Name.ValueString()] = column
	}

	var updates []models.TableUpdateRequest

	for _, column := range state {
		name := column.Name.ValueString()
		desired, exists := planned[name]
		if !exists {
			continue
		}

		desiredType, err := models.ParseDataType(desired.Type.ValueString())
		if err != nil {
			diags.AddAttributeError(path.Root("column"), "Invalid column type", err.Error())
			return nil, diags
		}

		currentType, err := models.ParseDataType(column.Type.ValueString())
		if err != nil {
			diags.AddAttributeError(path.Root("column"), "Invalid column type", err.Error())
			return nil, diags
		}

		if desiredType.String() != currentType.String() {
			updates = append(updates, models.NewUpdateTableColumnTypeRequest([]string{name}, desiredType))
		}

		if !desired.Comment.Equal(column.Comment) {
			updates = append(updates, models.NewUpdateTableColumnCommentRequest([]string{name}, desired.Comment.ValueString()))
		}

		if !desired.Nullable.Equal(column.Nullable) {
			updates = append(updates, models.NewUpdateTableColumnNullabilityRequest([]string{name}, desired.Nullable.ValueBool()))
		}

		if !desired.DefaultValue.Equal(column.DefaultValue) {
			updates = append(updates, models.NewUpdateTableColumnDefaultValueRequest([]string{name}, models.ColumnDefaultValue(desired, desiredType)))
		}
	}

	updates = append(updates, tableColumnPositionUpdates(plan, state)...)

	return updates, diags
}

// tableColumnPositionUpdates reorders the columns when their relative order
// changed. Gravitino applies the requests in order, so placing every column
// after its predecessor yields exactly the planned order.

// tableColumnPositionUpdates reorders the columns when their relative order
// changed. Gravitino applies the requests in order, so placing every column
// after its predecessor yields exactly the planned order.
func tableColumnPositionUpdates(plan, state []models.ColumnTFSDK) []models.TableUpdateRequest {
	stateOrder := make([]string, 0, len(state))
	for _, column := range state {
		stateOrder = append(stateOrder, column.Name.ValueString())
	}

	planOrder := make([]string, 0, len(plan))
	for _, column := range plan {
		planOrder = append(planOrder, column.Name.ValueString())
	}

	if reflect.DeepEqual(stateOrder, planOrder) {
		return nil
	}

	updates := make([]models.TableUpdateRequest, 0, len(planOrder))
	for i, name := range planOrder {
		if i == 0 {
			updates = append(updates, models.NewUpdateTableColumnPositionRequest([]string{name}, models.FirstColumnPosition()))
			continue
		}
		updates = append(updates, models.NewUpdateTableColumnPositionRequest([]string{name}, models.AfterColumnPosition(planOrder[i-1])))
	}

	return updates
}

// columnsReplaceRequired reports whether the column change needs a new table:
// the set of column names changed, or a default value was removed. Gravitino
// cannot tell a renamed column from a deleted and added one, and it cannot
// clear a column default value.
