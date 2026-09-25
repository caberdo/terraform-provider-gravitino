package models

import "github.com/hashicorp/terraform-plugin-framework/types"

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
// object for struct, list, map, union and unparsed types. DefaultValue holds
// the value of the column default value literal; the data type of that literal
// is the column type, so it is not modelled separately.
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
// indexed field.
type IndexTFSDK struct {
	IndexType  types.String `tfsdk:"index_type"`
	Name       types.String `tfsdk:"name"`
	FieldNames types.List   `tfsdk:"field_names"`
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
