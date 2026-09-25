package models

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TablePropertiesFromModel returns the properties of a Terraform map value.
// A null or unknown map yields nil.
func TablePropertiesFromModel(ctx context.Context, value types.Map) (map[string]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}

	properties := make(map[string]string)
	diags.Append(value.ElementsAs(ctx, &properties, false)...)
	if diags.HasError() {
		return nil, diags
	}
	return properties, diags
}

// TableColumnsFromModel converts the modelled columns into tables.yaml#/Column,
// parsing the Gravitino data type of every column.
func TableColumnsFromModel(columns []ColumnTFSDK) ([]Column, diag.Diagnostics) {
	var diags diag.Diagnostics

	result := make([]Column, 0, len(columns))
	for _, column := range columns {
		dataType, err := ParseDataType(column.Type.ValueString())
		if err != nil {
			diags.AddError(
				"Invalid column type",
				fmt.Sprintf("Column %q: %s.", column.Name.ValueString(), err.Error()),
			)
			continue
		}

		result = append(result, Column{
			Name:          column.Name.ValueString(),
			Type:          dataType,
			Comment:       column.Comment.ValueString(),
			Nullable:      column.Nullable.ValueBool(),
			AutoIncrement: column.AutoIncrement.ValueBool(),
			DefaultValue:  ColumnDefaultValue(column, dataType),
		})
	}

	return result, diags
}

// ColumnDefaultValue builds the default value literal of a column. The data
// type of the literal is the column type, which is the shape Gravitino stores
// and reports for column defaults.
func ColumnDefaultValue(column ColumnTFSDK, dataType DataType) *Literal {
	if column.DefaultValue.IsNull() || column.DefaultValue.IsUnknown() {
		return nil
	}

	literal := NewLiteral(dataType, column.DefaultValue.ValueString())
	return &literal
}

// TableColumnsToModel converts the columns reported by Gravitino into the
// Terraform model.
func TableColumnsToModel(ctx context.Context, columns []Column, diags *diag.Diagnostics) []ColumnTFSDK {
	result := make([]ColumnTFSDK, 0, len(columns))
	for _, column := range columns {
		mapped := ColumnTFSDK{
			Name:          types.StringValue(column.Name),
			Type:          types.StringValue(column.Type.String()),
			Nullable:      types.BoolValue(column.Nullable),
			AutoIncrement: types.BoolValue(column.AutoIncrement),
			DefaultValue:  DefaultValueToModel(column.DefaultValue),
		}

		if column.Comment == "" {
			mapped.Comment = types.StringNull()
		} else {
			mapped.Comment = types.StringValue(column.Comment)
		}

		result = append(result, mapped)
	}
	return result
}

// DefaultValueToModel maps the literal Gravitino reports for a column default.
// Gravitino reports {"type":"literal","dataType":"null","value":"NULL"} for a
// column without a default value; that is not a default the configuration can
// express, so it maps to no default value.
func DefaultValueToModel(defaultValue *Literal) types.String {
	if defaultValue == nil || defaultValue.DataType == nil {
		return types.StringNull()
	}
	if strings.EqualFold(defaultValue.DataType.Type, "null") {
		return types.StringNull()
	}
	return types.StringValue(defaultValue.Value)
}

// TableSortOrdersFromModel converts the modelled sort orders. The spec defaults
// the null ordering to nulls_first for an ascending and to nulls_last for a
// descending sort order, so that default is sent explicitly: the value reported
// by Gravitino then matches the configuration.
func TableSortOrdersFromModel(ctx context.Context, orders []SortOrderTFSDK) ([]SortOrder, diag.Diagnostics) {
	var diags diag.Diagnostics

	result := make([]SortOrder, 0, len(orders))
	for _, order := range orders {
		fieldName, fieldDiags := listOfStrings(ctx, order.FieldName)
		diags.Append(fieldDiags...)
		if diags.HasError() {
			return nil, diags
		}

		direction := order.Direction.ValueString()
		if direction == "" {
			direction = "asc"
		}

		nullOrdering := order.NullOrdering.ValueString()
		if nullOrdering == "" {
			nullOrdering = DefaultNullOrdering(direction)
		}

		result = append(result, SortOrder{
			SortTerm:     NewFieldExpression(fieldName),
			Direction:    direction,
			NullOrdering: nullOrdering,
		})
	}

	return result, diags
}

func DefaultNullOrdering(direction string) string {
	if direction == "desc" {
		return "nulls_last"
	}
	return "nulls_first"
}

// TableSortOrdersToModel converts the sort orders reported by Gravitino.
func TableSortOrdersToModel(ctx context.Context, orders []SortOrder, diags *diag.Diagnostics) []SortOrderTFSDK {
	result := make([]SortOrderTFSDK, 0, len(orders))
	for _, order := range orders {
		fieldName, fieldDiags := stringList(order.SortTerm.FieldName)
		diags.Append(fieldDiags...)

		mapped := SortOrderTFSDK{
			FieldName:    fieldName,
			Direction:    types.StringValue(order.Direction),
			NullOrdering: types.StringNull(),
		}
		if order.NullOrdering != "" {
			mapped.NullOrdering = types.StringValue(order.NullOrdering)
		}

		if diags.HasError() {
			return result
		}
		result = append(result, mapped)
	}
	return result
}

// TableDistributionFromModel converts the modelled distribution.
func TableDistributionFromModel(ctx context.Context, distribution *DistributionTFSDK) (*Distribution, diag.Diagnostics) {
	var diags diag.Diagnostics
	if distribution == nil || distribution.Number.IsNull() || distribution.Number.IsUnknown() {
		return nil, diags
	}

	args, argDiags := listOfStrings(ctx, distribution.FuncArgs)
	diags.Append(argDiags...)
	if diags.HasError() {
		return nil, diags
	}

	result := &Distribution{
		Strategy: distribution.Strategy.ValueString(),
		Number:   int32(distribution.Number.ValueInt64()),
		FuncArgs: make([]Expression, 0, len(args)),
	}
	for _, arg := range args {
		result.FuncArgs = append(result.FuncArgs, NewFieldExpression(pathSegments(arg)))
	}

	return result, diags
}

// TableDistributionToModel converts the distribution reported by Gravitino. A
// distribution with the strategy "none" is how the JDBC catalogs report a table
// without a distribution, so it maps to no distribution block.
func TableDistributionToModel(ctx context.Context, distribution *Distribution, diags *diag.Diagnostics) *DistributionTFSDK {
	if distribution == nil || DistributionIsAbsent(distribution) {
		return nil
	}

	mapped := &DistributionTFSDK{
		Strategy: types.StringValue(distribution.Strategy),
		Number:   types.Int64Value(int64(distribution.Number)),
		FuncArgs: types.ListNull(types.StringType),
	}

	paths := fieldPaths(distribution.FuncArgs)
	if len(paths) > 0 {
		list, listDiags := stringList(paths)
		diags.Append(listDiags...)
		if diags.HasError() {
			return nil
		}
		mapped.FuncArgs = list
	}

	return mapped
}

func DistributionIsAbsent(distribution *Distribution) bool {
	if strings.EqualFold(distribution.Strategy, "none") {
		return true
	}
	return distribution.Strategy == "" && distribution.Number == 0 && len(distribution.FuncArgs) == 0
}

// TablePartitioningFromModel converts the modelled partitioning strategies.
func TablePartitioningFromModel(ctx context.Context, parts []PartitioningTFSDK) ([]Partitioning, diag.Diagnostics) {
	var diags diag.Diagnostics

	result := make([]Partitioning, 0, len(parts))
	for _, part := range parts {
		fieldName, fieldDiags := listOfStrings(ctx, part.FieldName)
		diags.Append(fieldDiags...)

		fieldNames, fieldNamesDiags := listOfLists(ctx, part.FieldNames)
		diags.Append(fieldNamesDiags...)

		args, argDiags := listOfStrings(ctx, part.FuncArgs)
		diags.Append(argDiags...)
		if diags.HasError() {
			return nil, diags
		}

		mapped := Partitioning{
			Strategy:   part.Strategy.ValueString(),
			FieldName:  fieldName,
			FieldNames: fieldNames,
		}

		if !part.NumBuckets.IsNull() && !part.NumBuckets.IsUnknown() {
			numBuckets := int(part.NumBuckets.ValueInt64())
			mapped.NumBuckets = &numBuckets
		}
		if !part.Width.IsNull() && !part.Width.IsUnknown() {
			width := int(part.Width.ValueInt64())
			mapped.Width = &width
		}
		if !part.FuncName.IsNull() && !part.FuncName.IsUnknown() {
			mapped.FuncName = part.FuncName.ValueString()
		}
		for _, arg := range args {
			mapped.FuncArgs = append(mapped.FuncArgs, NewFieldExpression(pathSegments(arg)))
		}

		result = append(result, mapped)
	}

	return result, diags
}

// TablePartitioningToModel converts the partitioning strategies reported by
// Gravitino.
func TablePartitioningToModel(ctx context.Context, parts []Partitioning, diags *diag.Diagnostics) []PartitioningTFSDK {
	result := make([]PartitioningTFSDK, 0, len(parts))
	for _, part := range parts {
		mapped := PartitioningTFSDK{
			Strategy:   types.StringValue(part.Strategy),
			FieldName:  types.ListNull(types.StringType),
			FieldNames: types.ListNull(types.ListType{ElemType: types.StringType}),
			NumBuckets: types.Int64Null(),
			Width:      types.Int64Null(),
			FuncName:   types.StringNull(),
			FuncArgs:   types.ListNull(types.StringType),
		}

		if len(part.FieldName) > 0 {
			list, listDiags := stringList(part.FieldName)
			diags.Append(listDiags...)
			mapped.FieldName = list
		}
		if len(part.FieldNames) > 0 {
			list, listDiags := nestedStringList(part.FieldNames)
			diags.Append(listDiags...)
			mapped.FieldNames = list
		}
		if part.NumBuckets != nil {
			mapped.NumBuckets = types.Int64Value(int64(*part.NumBuckets))
		}
		if part.Width != nil {
			mapped.Width = types.Int64Value(int64(*part.Width))
		}
		if part.FuncName != "" {
			mapped.FuncName = types.StringValue(part.FuncName)
		}

		paths := fieldPaths(part.FuncArgs)
		if len(paths) > 0 {
			list, listDiags := stringList(paths)
			diags.Append(listDiags...)
			mapped.FuncArgs = list
		}

		if diags.HasError() {
			return result
		}
		result = append(result, mapped)
	}
	return result
}

// TableIndexesFromModel converts the modelled indexes.
func TableIndexesFromModel(ctx context.Context, indexes []IndexTFSDK) ([]Index, diag.Diagnostics) {
	var diags diag.Diagnostics

	result := make([]Index, 0, len(indexes))
	for _, index := range indexes {
		fieldNames, fieldDiags := listOfLists(ctx, index.FieldNames)
		diags.Append(fieldDiags...)
		if diags.HasError() {
			return nil, diags
		}

		result = append(result, Index{
			IndexType:  index.IndexType.ValueString(),
			Name:       index.Name.ValueString(),
			FieldNames: fieldNames,
		})
	}

	return result, diags
}

// TableIndexesToModel converts the indexes reported by Gravitino. The catalogs
// report the index type in upper case (PRIMARY_KEY) while indexes.yaml defines
// it in lower case, so the type is lower cased to match the configuration.
func TableIndexesToModel(ctx context.Context, indexes []Index, diags *diag.Diagnostics) []IndexTFSDK {
	result := make([]IndexTFSDK, 0, len(indexes))
	for _, index := range indexes {
		fieldNames, fieldDiags := nestedStringList(index.FieldNames)
		diags.Append(fieldDiags...)

		mapped := IndexTFSDK{
			IndexType:  types.StringValue(strings.ToLower(index.IndexType)),
			FieldNames: fieldNames,
		}
		if index.Name == "" {
			mapped.Name = types.StringNull()
		} else {
			mapped.Name = types.StringValue(index.Name)
		}

		if diags.HasError() {
			return result
		}
		result = append(result, mapped)
	}
	return result
}

// listOfStrings reads a Terraform list of strings. A null list yields nil.
func listOfStrings(ctx context.Context, value types.List) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}

	var result []string
	diags.Append(value.ElementsAs(ctx, &result, false)...)
	if diags.HasError() {
		return nil, diags
	}
	return result, diags
}

// listOfLists reads a Terraform list of lists of strings.
func listOfLists(ctx context.Context, value types.List) ([][]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}

	result := make([][]string, 0, len(value.Elements()))
	for _, element := range value.Elements() {
		inner, ok := element.(types.List)
		if !ok {
			diags.AddError("Invalid field names", fmt.Sprintf("Expected a list of field path segments, got %T.", element))
			return nil, diags
		}

		var segments []string
		diags.Append(inner.ElementsAs(ctx, &segments, false)...)
		if diags.HasError() {
			return nil, diags
		}
		result = append(result, segments)
	}

	return result, diags
}

// stringList builds a Terraform list of strings.
func stringList(values []string) (types.List, diag.Diagnostics) {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}
	return types.ListValue(types.StringType, elements)
}

// nestedStringList builds a Terraform list of lists of strings.
func nestedStringList(values [][]string) (types.List, diag.Diagnostics) {
	elements := make([]attr.Value, 0, len(values))
	for _, inner := range values {
		list, diags := stringList(inner)
		if diags.HasError() {
			return types.ListNull(types.ListType{ElemType: types.StringType}), diags
		}
		elements = append(elements, list)
	}
	return types.ListValue(types.ListType{ElemType: types.StringType}, elements)
}

// pathSegments splits a dotted field path into its segments.
func pathSegments(path string) []string {
	return strings.Split(path, ".")
}

// fieldPaths renders the field arguments of a FunctionArg list as dotted paths.
// Only field arguments can be expressed in Terraform; other arguments are
// skipped.
func fieldPaths(args []Expression) []string {
	paths := make([]string, 0, len(args))
	for _, arg := range args {
		if arg.Type != "" && arg.Type != "field" {
			continue
		}
		paths = append(paths, strings.Join(arg.FieldName, "."))
	}
	return paths
}
