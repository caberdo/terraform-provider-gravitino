package statistics

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &StatisticsDataSource{}
var _ datasource.DataSourceWithConfigure = &StatisticsDataSource{}

type StatisticsDataSource struct {
	client *client.Client
}

func New() datasource.DataSource {
	return &StatisticsDataSource{}
}

func (d *StatisticsDataSource) SetClient(c *client.Client) {
	d.client = c
}

type StatisticsDataSourceModel struct {
	Metalake     types.String `tfsdk:"metalake"`
	ResourceType types.String `tfsdk:"resource_type"`
	Resource     types.String `tfsdk:"resource"`
	Statistics   types.List   `tfsdk:"statistics"`
}

type statisticItemModel struct {
	Name       types.String `tfsdk:"name"`
	Type       types.String `tfsdk:"type"`
	Value      types.String `tfsdk:"value"`
	Properties types.Map    `tfsdk:"properties"`
}

var StatisticItemAttrTypes = map[string]attr.Type{
	"name":       types.StringType,
	"type":       types.StringType,
	"value":      types.StringType,
	"properties": types.MapType{ElemType: types.StringType},
}

func (d *StatisticsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *StatisticsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_statistics"
}

func (d *StatisticsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"resource_type": schema.StringAttribute{
				Required:    true,
				Description: "The metadata object type (METALAKE, CATALOG, SCHEMA, TABLE, VIEW, COLUMN, FILESET, TOPIC, MODEL, FUNCTION, ROLE). VIEW and FUNCTION require Gravitino 1.3.1 or newer.",
				Validators: []validator.String{
					stringvalidator.OneOf(models.StatisticsObjectTypes...),
				},
			},
			"resource": schema.StringAttribute{
				Required:    true,
				Description: "The resource name.",
			},
			"statistics": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The statistics for the resource.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "The statistic name.",
						},
						"type": schema.StringAttribute{
							Computed:    true,
							Description: "The statistic type.",
						},
						"value": schema.StringAttribute{
							Computed:    true,
							Description: "The statistic value.",
						},
						"properties": schema.MapAttribute{
							Computed:    true,
							ElementType: types.StringType,
							Description: "The statistic properties.",
						},
					},
				},
			},
		},
	}
}

func (d *StatisticsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config StatisticsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resourceType := config.ResourceType.ValueString()
	resource := config.Resource.ValueString()

	if err := d.client.CheckMetadataObjectTypeSupported(ctx, resourceType); err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading statistics", resource, err)...)
		return
	}

	result, err := d.client.ListStatistics(ctx,
		config.Metalake.ValueString(),
		resourceType,
		resource,
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("listing statistics", resource, err)...)
		return
	}

	items := make([]attr.Value, 0, len(result.Statistics))
	for _, stat := range result.Statistics {
		s := stat
		item := statisticToItemModel(ctx, &s, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if item == nil {
			continue
		}
		obj, objDiags := types.ObjectValueFrom(ctx, StatisticItemAttrTypes, item)
		resp.Diagnostics.Append(objDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, obj)
	}

	statsList, listDiags := types.ListValue(types.ObjectType{AttrTypes: StatisticItemAttrTypes}, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Statistics = statsList
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func statisticToItemModel(ctx context.Context, s *models.Statistics, diags *diag.Diagnostics) *statisticItemModel {
	if s == nil {
		return nil
	}

	item := &statisticItemModel{
		Name:  types.StringValue(s.Name),
		Type:  types.StringValue(s.Type),
		Value: types.StringValue(s.Value),
	}

	props, d := types.MapValueFrom(ctx, types.StringType, s.Properties)
	diags.Append(d...)
	if diags.HasError() {
		return nil
	}
	item.Properties = props

	return item
}
