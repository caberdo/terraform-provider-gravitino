package statistics

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
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
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected DataSource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	d.client = c
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
				Description: "The metadata object type (e.g. CATALOG, SCHEMA, TABLE, COLUMN, FILESET, TOPIC, MODEL, ROLE).",
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

	result, err := d.client.ListStatistics(ctx,
		config.Metalake.ValueString(),
		config.ResourceType.ValueString(),
		config.Resource.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list statistics", err.Error())
		return
	}

	items := make([]attr.Value, 0, len(result.Statistics))
	for _, stat := range result.Statistics {
		s := stat
		item := statisticToItemModel(ctx, &s)
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

func statisticToItemModel(ctx context.Context, s *models.Statistics) *statisticItemModel {
	if s == nil {
		return nil
	}

	item := &statisticItemModel{
		Name:  types.StringValue(s.Name),
		Type:  types.StringValue(s.Type),
		Value: types.StringValue(s.Value),
	}

	props, d := types.MapValueFrom(ctx, types.StringType, s.Properties)
	if d.HasError() {
		return nil
	}
	item.Properties = props

	return item
}
