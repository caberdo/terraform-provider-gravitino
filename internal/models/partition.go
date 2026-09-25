package models

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Partition types from partitions.yaml#/PartitionSpec.
const (
	PartitionTypeIdentity = "identity"
	PartitionTypeRange    = "range"
	PartitionTypeList     = "list"
)

// Partition mirrors partitions.yaml#/PartitionSpec. The dialect is selected by
// Type: "identity" uses FieldNames and Values, "range" uses Upper and Lower and
// "list" uses Lists.
type Partition struct {
	Type       string            `json:"type"`
	Name       string            `json:"name,omitempty"`
	FieldNames [][]string        `json:"fieldNames,omitempty"`
	Values     []Literal         `json:"values,omitempty"`
	Upper      *Literal          `json:"upper,omitempty"`
	Lower      *Literal          `json:"lower,omitempty"`
	Lists      [][]Literal       `json:"lists,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// AddPartitionsRequest mirrors partitions.yaml#/AddPartitionsRequest, the body
// of POST /tables/{table}/partitions.
type AddPartitionsRequest struct {
	Partitions []Partition `json:"partitions"`
}

// PartitionResponse mirrors the PartitionResponse schema of partitions.yaml.
type PartitionResponse struct {
	Code      int       `json:"code"`
	Partition Partition `json:"partition"`
}

// PartitionNameListResponse is returned by
// GET /tables/{table}/partitions without the details query parameter.
type PartitionNameListResponse struct {
	Code  int      `json:"code"`
	Names []string `json:"names"`
}

// PartitionListResponse is returned by
// GET /tables/{table}/partitions?details=true and by POST /tables/{table}/partitions.
type PartitionListResponse struct {
	Code       int         `json:"code"`
	Partitions []Partition `json:"partitions"`
}

// PartitionLiteralAttrTypes holds the Terraform attribute types of a literal
// (expression.yaml#/Literal) as exposed in partition values.
var PartitionLiteralAttrTypes = map[string]attr.Type{
	"data_type": types.StringType,
	"value":     types.StringType,
}

// PartitionSpecAttrTypes holds the Terraform attribute types of a partition
// spec, used by the gravitino_partitions data source.
var PartitionSpecAttrTypes = map[string]attr.Type{
	"type":        types.StringType,
	"name":        types.StringType,
	"field_names": types.ListType{ElemType: types.ListType{ElemType: types.StringType}},
	"values":      types.ListType{ElemType: types.ObjectType{AttrTypes: PartitionLiteralAttrTypes}},
	"upper":       types.ObjectType{AttrTypes: PartitionLiteralAttrTypes},
	"lower":       types.ObjectType{AttrTypes: PartitionLiteralAttrTypes},
	"lists":       types.ListType{ElemType: types.ListType{ElemType: types.ObjectType{AttrTypes: PartitionLiteralAttrTypes}}},
	"properties":  types.MapType{ElemType: types.StringType},
}

// Element types of the partition collections. The framework list helpers take
// the type of the elements, not the type of the list.
var (
	partitionFieldNameElementType = types.ListType{ElemType: types.StringType}
	partitionValueElementType     = types.ObjectType{AttrTypes: PartitionLiteralAttrTypes}
	partitionListElementType      = types.ListType{ElemType: types.ObjectType{AttrTypes: PartitionLiteralAttrTypes}}
)

// LiteralToObjectValue converts a literal into the object value used by the
// partition resource and data sources.
func LiteralToObjectValue(ctx context.Context, literal Literal) (types.Object, diag.Diagnostics) {
	dataType := types.StringNull()
	if literal.DataType != nil {
		dataType = types.StringValue(literal.DataType.String())
	}

	return types.ObjectValue(PartitionLiteralAttrTypes, map[string]attr.Value{
		"data_type": dataType,
		"value":     types.StringValue(literal.Value),
	})
}

// LiteralFromObjectValue converts an object value into a literal. A null object
// yields the zero literal.
func LiteralFromObjectValue(ctx context.Context, value types.Object) (Literal, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return Literal{}, diags
	}

	var literalType types.String
	var valueString types.String
	for name, attribute := range value.Attributes() {
		switch name {
		case "data_type":
			if stringValue, ok := attribute.(types.String); ok {
				literalType = stringValue
			}
		case "value":
			if stringValue, ok := attribute.(types.String); ok {
				valueString = stringValue
			}
		}
	}

	literal := Literal{Type: LiteralType, Value: valueString.ValueString()}
	if !literalType.IsNull() && !literalType.IsUnknown() && literalType.ValueString() != "" {
		dataType, err := ParseDataType(literalType.ValueString())
		if err != nil {
			diags.AddError("Invalid partition value type", err.Error())
			return Literal{}, diags
		}
		literal.DataType = &dataType
	}

	return literal, diags
}

// FieldNamesToList converts spec field names into a Terraform list value.
func FieldNamesToList(ctx context.Context, fieldNames [][]string) (types.List, diag.Diagnostics) {
	if len(fieldNames) == 0 {
		return types.ListNull(partitionFieldNameElementType), nil
	}

	outer := make([]attr.Value, 0, len(fieldNames))
	for _, fieldName := range fieldNames {
		inner := make([]attr.Value, 0, len(fieldName))
		for _, segment := range fieldName {
			inner = append(inner, types.StringValue(segment))
		}

		innerList, diags := types.ListValue(types.StringType, inner)
		if diags.HasError() {
			return types.ListNull(partitionFieldNameElementType), diags
		}
		outer = append(outer, innerList)
	}

	return types.ListValue(partitionFieldNameElementType, outer)
}

// FieldNamesFromList converts a Terraform list value into spec field names.
func FieldNamesFromList(ctx context.Context, value types.List) ([][]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}

	fieldNames := make([][]string, 0, len(value.Elements()))
	for _, element := range value.Elements() {
		innerList, ok := element.(types.List)
		if !ok {
			diags.AddError("Invalid field_names element", fmt.Sprintf("Expected a list of field name segments, got %T.", element))
			return nil, diags
		}

		var segments []string
		diags.Append(innerList.ElementsAs(ctx, &segments, false)...)
		if diags.HasError() {
			return nil, diags
		}
		fieldNames = append(fieldNames, segments)
	}

	return fieldNames, diags
}

// LiteralsToList converts literals into a Terraform list of objects.
func LiteralsToList(ctx context.Context, literals []Literal) (types.List, diag.Diagnostics) {
	if len(literals) == 0 {
		return types.ListNull(partitionValueElementType), nil
	}

	elements := make([]attr.Value, 0, len(literals))
	var diags diag.Diagnostics
	for _, literal := range literals {
		object, objectDiags := LiteralToObjectValue(ctx, literal)
		diags.Append(objectDiags...)
		if diags.HasError() {
			return types.ListNull(partitionValueElementType), diags
		}
		elements = append(elements, object)
	}

	list, listDiags := types.ListValue(partitionValueElementType, elements)
	diags.Append(listDiags...)
	return list, diags
}

// LiteralsFromList converts a Terraform list of objects into literals.
func LiteralsFromList(ctx context.Context, value types.List) ([]Literal, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}

	literals := make([]Literal, 0, len(value.Elements()))
	for _, element := range value.Elements() {
		object, ok := element.(types.Object)
		if !ok {
			diags.AddError("Invalid partition value", fmt.Sprintf("Expected an object with data_type and value, got %T.", element))
			return nil, diags
		}

		literal, literalDiags := LiteralFromObjectValue(ctx, object)
		diags.Append(literalDiags...)
		if diags.HasError() {
			return nil, diags
		}
		literals = append(literals, literal)
	}

	return literals, diags
}

// LiteralListsToList converts a list partition's nested literal lists into a
// Terraform list of lists of objects.
func LiteralListsToList(ctx context.Context, lists [][]Literal) (types.List, diag.Diagnostics) {
	if len(lists) == 0 {
		return types.ListNull(partitionListElementType), nil
	}

	outer := make([]attr.Value, 0, len(lists))
	var diags diag.Diagnostics
	for _, literals := range lists {
		inner, innerDiags := LiteralsToList(ctx, literals)
		diags.Append(innerDiags...)
		if diags.HasError() {
			return types.ListNull(partitionListElementType), diags
		}
		outer = append(outer, inner)
	}

	return types.ListValue(partitionListElementType, outer)
}

// LiteralListsFromList converts a Terraform list of lists of objects into a
// list partition's nested literal lists.
func LiteralListsFromList(ctx context.Context, value types.List) ([][]Literal, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}

	lists := make([][]Literal, 0, len(value.Elements()))
	for _, element := range value.Elements() {
		innerList, ok := element.(types.List)
		if !ok {
			diags.AddError("Invalid lists element", fmt.Sprintf("Expected a list of partition values, got %T.", element))
			return nil, diags
		}

		literals, literalDiags := LiteralsFromList(ctx, innerList)
		diags.Append(literalDiags...)
		if diags.HasError() {
			return nil, diags
		}
		lists = append(lists, literals)
	}

	return lists, diags
}

// PartitionToObjectValue converts a partition into the object value used by the
// gravitino_partitions data source.
func PartitionToObjectValue(ctx context.Context, partition *Partition) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	fieldNames, fieldNameDiags := FieldNamesToList(ctx, partition.FieldNames)
	diags.Append(fieldNameDiags...)

	values, valuesDiags := LiteralsToList(ctx, partition.Values)
	diags.Append(valuesDiags...)

	upper := types.ObjectNull(PartitionLiteralAttrTypes)
	if partition.Upper != nil {
		object, upperDiags := LiteralToObjectValue(ctx, *partition.Upper)
		diags.Append(upperDiags...)
		upper = object
	}

	lower := types.ObjectNull(PartitionLiteralAttrTypes)
	if partition.Lower != nil {
		object, lowerDiags := LiteralToObjectValue(ctx, *partition.Lower)
		diags.Append(lowerDiags...)
		lower = object
	}

	lists, listDiags := LiteralListsToList(ctx, partition.Lists)
	diags.Append(listDiags...)
	if diags.HasError() {
		return types.ObjectNull(PartitionSpecAttrTypes), diags
	}

	properties := types.MapNull(types.StringType)
	if len(partition.Properties) > 0 {
		propertiesValue, propertyDiags := types.MapValueFrom(ctx, types.StringType, partition.Properties)
		diags.Append(propertyDiags...)
		properties = propertiesValue
	}

	object, objectDiags := types.ObjectValue(PartitionSpecAttrTypes, map[string]attr.Value{
		"type":        types.StringValue(partition.Type),
		"name":        types.StringValue(partition.Name),
		"field_names": fieldNames,
		"values":      values,
		"upper":       upper,
		"lower":       lower,
		"lists":       lists,
		"properties":  properties,
	})
	diags.Append(objectDiags...)

	return object, diags
}
