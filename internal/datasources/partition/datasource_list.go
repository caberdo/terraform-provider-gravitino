package partition

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &PartitionsDataSource{}
var _ datasource.DataSourceWithConfigure = &PartitionsDataSource{}

type PartitionsDataSource struct {
	client *client.Client
}

// PartitionsDataSourceModel is the Terraform model of the gravitino_partitions
// data source.
type PartitionsDataSourceModel struct {
	Metalake   types.String `tfsdk:"metalake"`
	Catalog    types.String `tfsdk:"catalog"`
	Schema     types.String `tfsdk:"schema"`
	Table      types.String `tfsdk:"table"`
	Partitions types.List   `tfsdk:"partitions"`
}

// NewPartitionsDataSource returns the gravitino_partitions data source.
func NewPartitionsDataSource() datasource.DataSource {
	return &PartitionsDataSource{}
}

func (d *PartitionsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_partitions"
}

func (d *PartitionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the partitions of a Gravitino table with their details.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Description: "The metalake the partitions belong to.",
				Required:    true,
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog the partitions belong to.",
				Required:    true,
			},
			"schema": schema.StringAttribute{
				Description: "The schema the partitions belong to.",
				Required:    true,
			},
			"table": schema.StringAttribute{
				Description: "The table the partitions belong to.",
				Required:    true,
			},
			"partitions": schema.ListNestedAttribute{
				Description: "The partitions of the table, as reported by Gravitino.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Description: "The partition type: identity, range or list.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The name of the partition.",
							Computed:    true,
						},
						"field_names": schema.ListAttribute{
							Description: "The identity partition fields, each entry holding the path segments of a field.",
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
				},
			},
		},
	}
}

// SetClient sets the API client of the data source.
func (d *PartitionsDataSource) SetClient(c *client.Client) {
	d.client = c
}

func (d *PartitionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PartitionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PartitionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	partitions, err := d.client.ListPartitions(
		ctx,
		config.Metalake.ValueString(),
		config.Catalog.ValueString(),
		config.Schema.ValueString(),
		config.Table.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("listing partitions", config.Table.ValueString(), err)...)
		return
	}

	items := make([]attr.Value, 0, len(partitions))
	for i := range partitions {
		item, itemDiags := models.PartitionToObjectValue(ctx, &partitions[i])
		resp.Diagnostics.Append(itemDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, item)
	}

	list, listDiags := types.ListValue(
		types.ObjectType{AttrTypes: models.PartitionSpecAttrTypes},
		items,
	)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.Partitions = list

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
