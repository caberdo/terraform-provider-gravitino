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
	Value      types.String `tfsdk:"value"`
	Reserved   types.Bool   `tfsdk:"reserved"`
	Modifiable types.Bool   `tfsdk:"modifiable"`
	Audit      types.Object `tfsdk:"audit"`
}

var StatisticItemAttrTypes = map[string]attr.Type{
	"name":       types.StringType,
	"value":      types.StringType,
	"reserved":   types.BoolType,
	"modifiable": types.BoolType,
	"audit":      types.ObjectType{AttrTypes: models.AuditAttrTypes},
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
				Description: "The metadata object type (METALAKE, CATALOG, SCHEMA, TABLE, VIEW, COLUMN, FILESET, TOPIC, MODEL, FUNCTION, ROLE). Gravitino 1.3.x only implements statistics for TABLE; every other type answers 400. VIEW and FUNCTION require Gravitino 1.3.1 or newer.",
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
						"value": schema.StringAttribute{
							Computed: true,
							Description: "The statistic value as a string. Gravitino's value is untyped JSON " +
								"(statistics.yaml declares `additionalProperties: {}`); a string value contributes its text, " +
								"a number, boolean, array or object its JSON encoding. Null when the statistic has no value.",
						},
						"reserved": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether the statistic is reserved (supplied by the underlying catalog).",
						},
						"modifiable": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether the statistic can be written through Gravitino. Only modifiable statistics can be managed by gravitino_statistics.",
						},
						"audit": schema.SingleNestedAttribute{
							Computed:    true,
							Description: "Audit information for the statistic.",
							Attributes: map[string]schema.Attribute{
								"creator": schema.StringAttribute{
									Computed:    true,
									Description: "The principal that created the statistic.",
								},
								"create_time": schema.StringAttribute{
									Computed:    true,
									Description: "The creation time in RFC 3339 format.",
								},
								"last_modifier": schema.StringAttribute{
									Computed:    true,
									Description: "The principal that last modified the statistic.",
								},
								"last_modified_time": schema.StringAttribute{
									Computed:    true,
									Description: "The last modification time in RFC 3339 format.",
								},
							},
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

func statisticToItemModel(ctx context.Context, s *models.Statistic, diags *diag.Diagnostics) *statisticItemModel {
	if s == nil {
		return nil
	}

	item := &statisticItemModel{
		Name:       types.StringValue(s.Name),
		Reserved:   types.BoolValue(s.Reserved),
		Modifiable: types.BoolValue(s.Modifiable),
	}

	if value, ok := s.ValueString(); ok {
		item.Value = types.StringValue(value)
	} else {
		item.Value = types.StringNull()
	}

	audit, auditDiags := models.AuditToObjectValue(ctx, s.Audit)
	diags.Append(auditDiags...)
	if diags.HasError() {
		return nil
	}
	item.Audit = audit

	return item
}
