package models

import "github.com/hashicorp/terraform-plugin-framework/types"

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

type AuditTFSDK struct {
	Creator          types.String `tfsdk:"creator"`
	CreateTime       types.String `tfsdk:"create_time"`
	LastModifier     types.String `tfsdk:"last_modifier"`
	LastModifiedTime types.String `tfsdk:"last_modified_time"`
}

type ColumnTFSDK struct {
	Name          types.String `tfsdk:"name"`
	Type          types.String `tfsdk:"type"`
	Length        types.Int64  `tfsdk:"length"`
	Precision     types.Int64  `tfsdk:"precision"`
	Scale         types.Int64  `tfsdk:"scale"`
	Comment       types.String `tfsdk:"comment"`
	Nullable      types.Bool   `tfsdk:"nullable"`
	AutoIncrement types.Bool   `tfsdk:"auto_increment"`
	DefaultValue  types.String `tfsdk:"default_value"`
}

type SortOrderTFSDK struct {
	FieldName    types.List   `tfsdk:"field_name"`
	Direction    types.String `tfsdk:"direction"`
	NullOrdering types.String `tfsdk:"null_ordering"`
}

type DistributionTFSDK struct {
	Strategy types.String `tfsdk:"strategy"`
	Number   types.Int64  `tfsdk:"number"`
	FuncArgs types.List   `tfsdk:"func_args"`
}

type PartitioningTFSDK struct {
	Strategy   types.String `tfsdk:"strategy"`
	FieldName  types.List   `tfsdk:"field_name"`
	FieldNames types.List   `tfsdk:"field_names"`
	NumBuckets types.Int64  `tfsdk:"num_buckets"`
	Width      types.Int64  `tfsdk:"width"`
	FuncName   types.String `tfsdk:"func_name"`
	FuncArgs   types.List   `tfsdk:"func_args"`
}

type IndexTFSDK struct {
	IndexType  types.String `tfsdk:"index_type"`
	Name       types.String `tfsdk:"name"`
	FieldNames types.List   `tfsdk:"field_names"`
}

type TableDataSourceModel struct {
	Metalake     types.String        `tfsdk:"metalake"`
	Catalog      types.String        `tfsdk:"catalog"`
	Schema       types.String        `tfsdk:"schema"`
	Name         types.String        `tfsdk:"name"`
	Comment      types.String        `tfsdk:"comment"`
	Properties   types.Map           `tfsdk:"properties"`
	Audit        *AuditTFSDK         `tfsdk:"audit"`
	Columns      []ColumnTFSDK       `tfsdk:"column"`
	SortOrders   []SortOrderTFSDK    `tfsdk:"sort_order"`
	Distribution *DistributionTFSDK  `tfsdk:"distribution"`
	Partitioning []PartitioningTFSDK `tfsdk:"partitioning"`
	Indexes      []IndexTFSDK        `tfsdk:"index"`
}
