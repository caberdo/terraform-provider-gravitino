package partition

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &PartitionDataSource{}
var _ datasource.DataSourceWithConfigure = &PartitionDataSource{}

type PartitionDataSource struct {
	client *client.Client
}

// PartitionDataSourceModel is the Terraform model of the gravitino_partition
// data source.
type PartitionDataSourceModel struct {
	Metalake   types.String `tfsdk:"metalake"`
	Catalog    types.String `tfsdk:"catalog"`
	Schema     types.String `tfsdk:"schema"`
	Table      types.String `tfsdk:"table"`
	Name       types.String `tfsdk:"name"`
	Type       types.String `tfsdk:"type"`
	FieldNames types.List   `tfsdk:"field_names"`
	Values     types.List   `tfsdk:"values"`
	Upper      types.Object `tfsdk:"upper"`
	Lower      types.Object `tfsdk:"lower"`
	Lists      types.List   `tfsdk:"lists"`
	Properties types.Map    `tfsdk:"properties"`
}

// NewPartitionDataSource returns the gravitino_partition data source.
func NewPartitionDataSource() datasource.DataSource {
	return &PartitionDataSource{}
}

func (d *PartitionDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_partition"
}

func (d *PartitionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves a single partition of a Gravitino table by name.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Description: "The metalake the partition belongs to.",
				Required:    true,
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog the partition belongs to.",
				Required:    true,
			},
			"schema": schema.StringAttribute{
				Description: "The schema the partition belongs to.",
				Required:    true,
			},
			"table": schema.StringAttribute{
				Description: "The table the partition belongs to.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the partition.",
				Required:    true,
			},
			"type": schema.StringAttribute{
				Description: "The partition type: identity, range or list.",
				Computed:    true,
			},
			"field_names": schema.ListAttribute{
				Description: "The identity partition fields, one entry per field, each entry holding the path segments " +
					"of the field.",
				Computed:    true,
				ElementType: types.ListType{ElemType: types.StringType},
			},
			"values": schema.ListAttribute{
				Description: "The identity partition values, one literal per entry of field_names.",
				Computed:    true,
				ElementType: types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes},
			},
			"upper": schema.ObjectAttribute{
				Description:    "The exclusive upper bound of a range partition.",
				Computed:       true,
				AttributeTypes: models.PartitionLiteralAttrTypes,
			},
			"lower": schema.ObjectAttribute{
				Description:    "The inclusive lower bound of a range partition.",
				Computed:       true,
				AttributeTypes: models.PartitionLiteralAttrTypes,
			},
			"lists": schema.ListAttribute{
				Description: "The value lists of a list partition, one entry per list.",
				Computed:    true,
				ElementType: types.ListType{ElemType: types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes}},
			},
			"properties": schema.MapAttribute{
				Description: "The properties of the partition as reported by Gravitino.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

// SetClient sets the API client of the data source.
func (d *PartitionDataSource) SetClient(c *client.Client) {
	d.client = c
}

func (d *PartitionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider data", "Expected *client.Client, got unexpected type.")
		return
	}
	d.client = c
}

func (d *PartitionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PartitionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	partitionResp, err := d.client.GetPartition(
		ctx,
		config.Metalake.ValueString(),
		config.Catalog.ValueString(),
		config.Schema.ValueString(),
		config.Table.ValueString(),
		config.Name.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading partition", config.Name.ValueString(), err)...)
		return
	}

	config.Name = types.StringValue(partitionResp.Partition.Name)
	partitionToDataSourceModel(ctx, &partitionResp.Partition, &config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// partitionToDataSourceModel maps a partition of partitions.yaml into the data
// source model.
func partitionToDataSourceModel(ctx context.Context, partition *models.Partition, model *PartitionDataSourceModel, diags *diag.Diagnostics) {
	model.Type = types.StringValue(partition.Type)

	fieldNames, fieldNameDiags := models.FieldNamesToList(ctx, partition.FieldNames)
	diags.Append(fieldNameDiags...)

	values, valueDiags := models.LiteralsToList(ctx, partition.Values)
	diags.Append(valueDiags...)

	lists, listDiags := models.LiteralListsToList(ctx, partition.Lists)
	diags.Append(listDiags...)
	if diags.HasError() {
		return
	}

	model.FieldNames = fieldNames
	model.Values = values
	model.Lists = lists

	model.Upper = types.ObjectNull(models.PartitionLiteralAttrTypes)
	if partition.Upper != nil {
		upper, upperDiags := models.LiteralToObjectValue(ctx, *partition.Upper)
		diags.Append(upperDiags...)
		model.Upper = upper
	}

	model.Lower = types.ObjectNull(models.PartitionLiteralAttrTypes)
	if partition.Lower != nil {
		lower, lowerDiags := models.LiteralToObjectValue(ctx, *partition.Lower)
		diags.Append(lowerDiags...)
		model.Lower = lower
	}

	model.Properties = types.MapNull(types.StringType)
	if len(partition.Properties) > 0 {
		properties, propertyDiags := types.MapValueFrom(ctx, types.StringType, partition.Properties)
		diags.Append(propertyDiags...)
		model.Properties = properties
	}
}
