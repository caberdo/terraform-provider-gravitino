package health

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = (*HealthDataSource)(nil)
var _ datasource.DataSourceWithConfigure = &HealthDataSource{}

type HealthDataSource struct {
	client *client.Client
}

func NewHealthDataSource() datasource.DataSource {
	return &HealthDataSource{}
}

func (d *HealthDataSource) SetClient(c *client.Client) {
	d.client = c
}

type HealthDataSourceModel struct {
	Status types.String       `tfsdk:"status"`
	Checks []HealthCheckModel `tfsdk:"checks"`
}

type HealthCheckModel struct {
	Name    types.String `tfsdk:"name"`
	Status  types.String `tfsdk:"status"`
	Details types.Map    `tfsdk:"details"`
}

func (d *HealthDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *HealthDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_health"
}

func (d *HealthDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "The aggregate health status.",
			},
		},
		Blocks: map[string]schema.Block{
			"checks": schema.ListNestedBlock{
				Description: "Health check details.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "Name of the health check.",
						},
						"status": schema.StringAttribute{
							Computed:    true,
							Description: "Status of the health check.",
						},
						"details": schema.MapAttribute{
							Computed:    true,
							ElementType: types.StringType,
							Description: "Additional details for the health check.",
						},
					},
				},
			},
		},
	}
}

func (d *HealthDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state HealthDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.GetHealth(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read health", err.Error())
		return
	}

	state.Status = types.StringValue(result.Status)
	state.Checks = healthChecksToModel(ctx, result.Checks, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
