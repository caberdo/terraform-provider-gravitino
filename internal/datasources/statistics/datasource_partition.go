package statistics

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &PartitionStatisticsDataSource{}
var _ datasource.DataSourceWithConfigure = &PartitionStatisticsDataSource{}

type PartitionStatisticsDataSource struct {
	client *client.Client
}

func NewPartition() datasource.DataSource {
	return &PartitionStatisticsDataSource{}
}

func (d *PartitionStatisticsDataSource) SetClient(c *client.Client) {
	d.client = c
}

type PartitionStatisticsDataSourceModel struct {
	Metalake            types.String `tfsdk:"metalake"`
	Catalog             types.String `tfsdk:"catalog"`
	Schema              types.String `tfsdk:"schema"`
	Table               types.String `tfsdk:"table"`
	PartitionStatistics types.List   `tfsdk:"partition_statistics"`
}

type partitionStatisticItemModel struct {
	PartitionName types.String `tfsdk:"partition_name"`
	Statistics    types.List   `tfsdk:"statistics"`
}

var PartitionStatisticItemAttrTypes = map[string]attr.Type{
	"partition_name": types.StringType,
	"statistics":     types.ListType{ElemType: types.ObjectType{AttrTypes: StatisticItemAttrTypes}},
}

func (d *PartitionStatisticsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PartitionStatisticsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_partition_statistics"
}

func (d *PartitionStatisticsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"catalog": schema.StringAttribute{
				Required:    true,
				Description: "The catalog name.",
			},
			"schema": schema.StringAttribute{
				Required:    true,
				Description: "The schema name.",
			},
			"table": schema.StringAttribute{
				Required:    true,
				Description: "The table name.",
			},
			"partition_statistics": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The partition statistics.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"partition_name": schema.StringAttribute{
							Computed:    true,
							Description: "The partition name.",
						},
						"statistics": schema.ListNestedAttribute{
							Computed:    true,
							Description: "The statistics for the partition.",
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
				},
			},
		},
	}
}

func (d *PartitionStatisticsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PartitionStatisticsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.ListPartitionStatistics(ctx,
		config.Metalake.ValueString(),
		"TABLE",
		fmt.Sprintf("%s.%s.%s", config.Catalog.ValueString(), config.Schema.ValueString(), config.Table.ValueString()),
	)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list partition statistics", err.Error())
		return
	}

	items := make([]attr.Value, 0, len(result.Statistics))
	for _, ps := range result.Statistics {
		p := ps
		item := partitionStatisticToItemModel(ctx, &p)
		if item == nil {
			continue
		}
		obj, objDiags := types.ObjectValueFrom(ctx, PartitionStatisticItemAttrTypes, item)
		resp.Diagnostics.Append(objDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, obj)
	}

	psList, listDiags := types.ListValue(types.ObjectType{AttrTypes: PartitionStatisticItemAttrTypes}, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.PartitionStatistics = psList
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func partitionStatisticToItemModel(ctx context.Context, ps *models.PartitionStatistics) *partitionStatisticItemModel {
	if ps == nil {
		return nil
	}

	item := &partitionStatisticItemModel{
		PartitionName: types.StringValue(ps.PartitionName),
	}

	statsItems := make([]attr.Value, 0, len(ps.Statistics))
	for _, s := range ps.Statistics {
		st := s
		statItem := statisticToItemModel(ctx, &st)
		if statItem == nil {
			continue
		}
		obj, objDiags := types.ObjectValueFrom(ctx, StatisticItemAttrTypes, statItem)
		if objDiags.HasError() {
			return nil
		}
		statsItems = append(statsItems, obj)
	}

	if len(statsItems) > 0 {
		statsList, d := types.ListValue(types.ObjectType{AttrTypes: StatisticItemAttrTypes}, statsItems)
		if d.HasError() {
			return nil
		}
		item.Statistics = statsList
	} else {
		item.Statistics = types.ListNull(types.ObjectType{AttrTypes: StatisticItemAttrTypes})
	}

	return item
}
