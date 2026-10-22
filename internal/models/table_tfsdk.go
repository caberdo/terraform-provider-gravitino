package models

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// TableResourceModel is the Terraform model of the gravitino_table resource.
type TableResourceModel struct {
	Metalake     types.String        `tfsdk:"metalake"`
	Catalog      types.String        `tfsdk:"catalog"`
	Schema       types.String        `tfsdk:"schema"`
	Name         types.String        `tfsdk:"name"`
	Comment      types.String        `tfsdk:"comment"`
	Properties   types.Map           `tfsdk:"properties"`
	ID           types.String        `tfsdk:"id"`
	Audit        types.Object        `tfsdk:"audit"`
	Columns      []ColumnTFSDK       `tfsdk:"column"`
	SortOrders   []SortOrderTFSDK    `tfsdk:"sort_order"`
	Distribution *DistributionTFSDK  `tfsdk:"distribution"`
	Partitioning []PartitioningTFSDK `tfsdk:"partitioning"`
	Indexes      []IndexTFSDK        `tfsdk:"index"`
}

// AuditTFSDK is the Terraform model of an audit object.
type AuditTFSDK struct {
	Creator          types.String `tfsdk:"creator"`
	CreateTime       types.String `tfsdk:"create_time"`
	LastModifier     types.String `tfsdk:"last_modifier"`
	LastModifiedTime types.String `tfsdk:"last_modified_time"`
}

// ColumnTFSDK is the Terraform model of a table column. Type holds the
// Gravitino data type: a primitive name such as "varchar(255)", or a JSON
// object for struct, list, map, union, unparsed and external types. DefaultValue
// holds the value of the column default value literal; the data type of that
// literal is the column type, so it is not modelled separately.
type ColumnTFSDK struct {
	Name          types.String `tfsdk:"name"`
	Type          types.String `tfsdk:"type"`
	Comment       types.String `tfsdk:"comment"`
	Nullable      types.Bool   `tfsdk:"nullable"`
	AutoIncrement types.Bool   `tfsdk:"auto_increment"`
	DefaultValue  types.String `tfsdk:"default_value"`
}

// SortOrderTFSDK is the Terraform model of a sort order. FieldName holds the
// path of the sort term field.
type SortOrderTFSDK struct {
	FieldName    types.List   `tfsdk:"field_name"`
	Direction    types.String `tfsdk:"direction"`
	NullOrdering types.String `tfsdk:"null_ordering"`
}

// DistributionTFSDK is the Terraform model of a distribution. FuncArgs holds
// the dotted field paths of the distribution arguments.
type DistributionTFSDK struct {
	Strategy types.String `tfsdk:"strategy"`
	Number   types.Int64  `tfsdk:"number"`
	FuncArgs types.List   `tfsdk:"func_args"`
}

// PartitioningTFSDK is the Terraform model of a partitioning strategy.
// FuncArgs holds the dotted field paths of the function arguments.
type PartitioningTFSDK struct {
	Strategy   types.String `tfsdk:"strategy"`
	FieldName  types.List   `tfsdk:"field_name"`
	FieldNames types.List   `tfsdk:"field_names"`
	NumBuckets types.Int64  `tfsdk:"num_buckets"`
	Width      types.Int64  `tfsdk:"width"`
	FuncName   types.String `tfsdk:"func_name"`
	FuncArgs   types.List   `tfsdk:"func_args"`
}

// IndexTFSDK is the Terraform model of an index. FieldNames holds one path per
// indexed field. Properties holds the extra index parameters Gravitino v1.3.1
// accepts.
type IndexTFSDK struct {
	IndexType  types.String `tfsdk:"index_type"`
	Name       types.String `tfsdk:"name"`
	FieldNames types.List   `tfsdk:"field_names"`
	Properties types.Map    `tfsdk:"properties"`
}

// TableDataSourceModel is the Terraform model of the gravitino_table data source.
type TableDataSourceModel struct {
	Metalake     types.String        `tfsdk:"metalake"`
	Catalog      types.String        `tfsdk:"catalog"`
	Schema       types.String        `tfsdk:"schema"`
	Name         types.String        `tfsdk:"name"`
	Comment      types.String        `tfsdk:"comment"`
	Properties   types.Map           `tfsdk:"properties"`
	Audit        types.Object        `tfsdk:"audit"`
	Columns      []ColumnTFSDK       `tfsdk:"column"`
	SortOrders   []SortOrderTFSDK    `tfsdk:"sort_order"`
	Distribution *DistributionTFSDK  `tfsdk:"distribution"`
	Partitioning []PartitioningTFSDK `tfsdk:"partitioning"`
	Indexes      []IndexTFSDK        `tfsdk:"index"`
}

// TableResourceTFModel is the Plugin Framework representation of the
// gravitino_table resource. The collection attributes are modelled with the
// framework types, because only those can hold the unknown values Terraform
// produces during planning (a Go slice cannot).
type TableResourceTFModel struct {
	Metalake     types.String `tfsdk:"metalake"`
	Catalog      types.String `tfsdk:"catalog"`
	Schema       types.String `tfsdk:"schema"`
	Name         types.String `tfsdk:"name"`
	Comment      types.String `tfsdk:"comment"`
	Properties   types.Map    `tfsdk:"properties"`
	ID           types.String `tfsdk:"id"`
	Audit        types.Object `tfsdk:"audit"`
	Columns      types.List   `tfsdk:"column"`
	SortOrders   types.List   `tfsdk:"sort_order"`
	Distribution types.Object `tfsdk:"distribution"`
	Partitioning types.List   `tfsdk:"partitioning"`
	Indexes      types.List   `tfsdk:"index"`
}

// TableDataSourceTFModel is the Plugin Framework representation of the
// gravitino_table data source.
type TableDataSourceTFModel struct {
	Metalake     types.String `tfsdk:"metalake"`
	Catalog      types.String `tfsdk:"catalog"`
	Schema       types.String `tfsdk:"schema"`
	Name         types.String `tfsdk:"name"`
	Comment      types.String `tfsdk:"comment"`
	Properties   types.Map    `tfsdk:"properties"`
	Audit        types.Object `tfsdk:"audit"`
	Columns      types.List   `tfsdk:"column"`
	SortOrders   types.List   `tfsdk:"sort_order"`
	Distribution types.Object `tfsdk:"distribution"`
	Partitioning types.List   `tfsdk:"partitioning"`
	Indexes      types.List   `tfsdk:"index"`
}

// ColumnAttrTypes returns the attribute types of a column object.
func ColumnAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":           types.StringType,
		"type":           types.StringType,
		"comment":        types.StringType,
		"nullable":       types.BoolType,
		"auto_increment": types.BoolType,
		"default_value":  types.StringType,
	}
}

// SortOrderAttrTypes returns the attribute types of a sort order object.
func SortOrderAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"field_name":    types.ListType{ElemType: types.StringType},
		"direction":     types.StringType,
		"null_ordering": types.StringType,
	}
}

// DistributionAttrTypes returns the attribute types of a distribution object.
func DistributionAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"strategy":  types.StringType,
		"number":    types.Int64Type,
		"func_args": types.ListType{ElemType: types.StringType},
	}
}

// PartitioningAttrTypes returns the attribute types of a partitioning object.
func PartitioningAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"strategy":    types.StringType,
		"field_name":  types.ListType{ElemType: types.StringType},
		"field_names": types.ListType{ElemType: types.ListType{ElemType: types.StringType}},
		"num_buckets": types.Int64Type,
		"width":       types.Int64Type,
		"func_name":   types.StringType,
		"func_args":   types.ListType{ElemType: types.StringType},
	}
}

// IndexAttrTypes returns the attribute types of an index object.
func IndexAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"index_type":  types.StringType,
		"name":        types.StringType,
		"field_names": types.ListType{ElemType: types.ListType{ElemType: types.StringType}},
		"properties":  types.MapType{ElemType: types.StringType},
	}
}

func columnObjectType() types.ObjectType    { return types.ObjectType{AttrTypes: ColumnAttrTypes()} }
func sortOrderObjectType() types.ObjectType { return types.ObjectType{AttrTypes: SortOrderAttrTypes()} }
func partitioningObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: PartitioningAttrTypes()}
}
func indexObjectType() types.ObjectType { return types.ObjectType{AttrTypes: IndexAttrTypes()} }

// TableResourceModelFromTF converts the framework representation of the
// resource into the slice-based model the CRUD code works with. A null or
// unknown collection yields no elements, so callers must check the framework
// value when an unknown value must not be treated as empty.
func TableResourceModelFromTF(ctx context.Context, in TableResourceTFModel, diags *diag.Diagnostics) TableResourceModel {
	return TableResourceModel{
		Metalake:     in.Metalake,
		Catalog:      in.Catalog,
		Schema:       in.Schema,
		Name:         in.Name,
		Comment:      in.Comment,
		Properties:   in.Properties,
		ID:           in.ID,
		Audit:        in.Audit,
		Columns:      TableColumnsFromList(ctx, in.Columns, diags),
		SortOrders:   TableSortOrdersFromList(ctx, in.SortOrders, diags),
		Distribution: TableDistributionFromObject(ctx, in.Distribution, diags),
		Partitioning: TablePartitioningFromList(ctx, in.Partitioning, diags),
		Indexes:      TableIndexesFromList(ctx, in.Indexes, diags),
	}
}

// TableResourceModelToTF converts the slice-based model into the framework
// representation used to read and write plan and state values.
func TableResourceModelToTF(ctx context.Context, in TableResourceModel, diags *diag.Diagnostics) TableResourceTFModel {
	return TableResourceTFModel{
		Metalake:     in.Metalake,
		Catalog:      in.Catalog,
		Schema:       in.Schema,
		Name:         in.Name,
		Comment:      in.Comment,
		Properties:   in.Properties,
		ID:           in.ID,
		Audit:        in.Audit,
		Columns:      TableColumnsToList(ctx, in.Columns, diags),
		SortOrders:   TableSortOrdersToList(ctx, in.SortOrders, diags),
		Distribution: TableDistributionToObject(ctx, in.Distribution, diags),
		Partitioning: TablePartitioningToList(ctx, in.Partitioning, diags),
		Indexes:      TableIndexesToList(ctx, in.Indexes, diags),
	}
}

// TableDataSourceModelToTF converts the slice-based data source model into the
// framework representation used to read and write the data source state.
func TableDataSourceModelToTF(ctx context.Context, in TableDataSourceModel, diags *diag.Diagnostics) TableDataSourceTFModel {
	return TableDataSourceTFModel{
		Metalake:     in.Metalake,
		Catalog:      in.Catalog,
		Schema:       in.Schema,
		Name:         in.Name,
		Comment:      in.Comment,
		Properties:   in.Properties,
		Audit:        in.Audit,
		Columns:      TableColumnsToList(ctx, in.Columns, diags),
		SortOrders:   TableSortOrdersToList(ctx, in.SortOrders, diags),
		Distribution: TableDistributionToObject(ctx, in.Distribution, diags),
		Partitioning: TablePartitioningToList(ctx, in.Partitioning, diags),
		Indexes:      TableIndexesToList(ctx, in.Indexes, diags),
	}
}

// TableColumnsFromList decodes a column list. Null and unknown lists yield no
// columns.
func TableColumnsFromList(ctx context.Context, value types.List, diags *diag.Diagnostics) []ColumnTFSDK {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var columns []ColumnTFSDK
	diags.Append(value.ElementsAs(ctx, &columns, false)...)
	if diags.HasError() {
		return nil
	}
	return columns
}

// TableSortOrdersFromList decodes a sort order list.
func TableSortOrdersFromList(ctx context.Context, value types.List, diags *diag.Diagnostics) []SortOrderTFSDK {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var orders []SortOrderTFSDK
	diags.Append(value.ElementsAs(ctx, &orders, false)...)
	if diags.HasError() {
		return nil
	}
	return orders
}

// TablePartitioningFromList decodes a partitioning list.
func TablePartitioningFromList(ctx context.Context, value types.List, diags *diag.Diagnostics) []PartitioningTFSDK {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var parts []PartitioningTFSDK
	diags.Append(value.ElementsAs(ctx, &parts, false)...)
	if diags.HasError() {
		return nil
	}
	return parts
}

// TableIndexesFromList decodes an index list.
func TableIndexesFromList(ctx context.Context, value types.List, diags *diag.Diagnostics) []IndexTFSDK {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var indexes []IndexTFSDK
	diags.Append(value.ElementsAs(ctx, &indexes, false)...)
	if diags.HasError() {
		return nil
	}
	return indexes
}

// TableDistributionFromObject decodes a distribution object. A null or unknown
// object yields no distribution.
func TableDistributionFromObject(ctx context.Context, value types.Object, diags *diag.Diagnostics) *DistributionTFSDK {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var distribution DistributionTFSDK
	diags.Append(value.As(ctx, &distribution, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	return &distribution
}

// TableColumnsToList encodes columns as a framework list.
func TableColumnsToList(ctx context.Context, columns []ColumnTFSDK, diags *diag.Diagnostics) types.List {
	elementType := columnObjectType()
	if columns == nil {
		return types.ListNull(elementType)
	}
	items := make([]attr.Value, 0, len(columns))
	for _, column := range columns {
		object, objectDiags := types.ObjectValueFrom(ctx, ColumnAttrTypes(), column)
		diags.Append(objectDiags...)
		if diags.HasError() {
			return types.ListNull(elementType)
		}
		items = append(items, object)
	}
	list, listDiags := types.ListValue(elementType, items)
	diags.Append(listDiags...)
	return list
}

// TableSortOrdersToList encodes sort orders as a framework list.
func TableSortOrdersToList(ctx context.Context, orders []SortOrderTFSDK, diags *diag.Diagnostics) types.List {
	elementType := sortOrderObjectType()
	if orders == nil {
		return types.ListNull(elementType)
	}
	items := make([]attr.Value, 0, len(orders))
	for _, order := range orders {
		object, objectDiags := types.ObjectValueFrom(ctx, SortOrderAttrTypes(), order)
		diags.Append(objectDiags...)
		if diags.HasError() {
			return types.ListNull(elementType)
		}
		items = append(items, object)
	}
	list, listDiags := types.ListValue(elementType, items)
	diags.Append(listDiags...)
	return list
}

// TablePartitioningToList encodes partitioning strategies as a framework list.
func TablePartitioningToList(ctx context.Context, parts []PartitioningTFSDK, diags *diag.Diagnostics) types.List {
	elementType := partitioningObjectType()
	if parts == nil {
		return types.ListNull(elementType)
	}
	items := make([]attr.Value, 0, len(parts))
	for _, part := range parts {
		object, objectDiags := types.ObjectValueFrom(ctx, PartitioningAttrTypes(), part)
		diags.Append(objectDiags...)
		if diags.HasError() {
			return types.ListNull(elementType)
		}
		items = append(items, object)
	}
	list, listDiags := types.ListValue(elementType, items)
	diags.Append(listDiags...)
	return list
}

// TableIndexesToList encodes indexes as a framework list.
func TableIndexesToList(ctx context.Context, indexes []IndexTFSDK, diags *diag.Diagnostics) types.List {
	elementType := indexObjectType()
	if indexes == nil {
		return types.ListNull(elementType)
	}
	items := make([]attr.Value, 0, len(indexes))
	for _, index := range indexes {
		object, objectDiags := types.ObjectValueFrom(ctx, IndexAttrTypes(), index)
		diags.Append(objectDiags...)
		if diags.HasError() {
			return types.ListNull(elementType)
		}
		items = append(items, object)
	}
	list, listDiags := types.ListValue(elementType, items)
	diags.Append(listDiags...)
	return list
}

// TableDistributionToObject encodes a distribution, or a null object when there
// is none.
func TableDistributionToObject(ctx context.Context, distribution *DistributionTFSDK, diags *diag.Diagnostics) types.Object {
	if distribution == nil {
		return types.ObjectNull(DistributionAttrTypes())
	}
	object, objectDiags := types.ObjectValueFrom(ctx, DistributionAttrTypes(), *distribution)
	diags.Append(objectDiags...)
	return object
}
