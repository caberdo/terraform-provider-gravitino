package health

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = (*LivenessDataSource)(nil)
var _ datasource.DataSourceWithConfigure = &LivenessDataSource{}

type LivenessDataSource struct {
	client *client.Client
}

func NewLivenessDataSource() datasource.DataSource {
	return &LivenessDataSource{}
}

func (d *LivenessDataSource) SetClient(c *client.Client) {
	d.client = c
}

type LivenessDataSourceModel struct {
	Status types.String       `tfsdk:"status"`
	Checks []HealthCheckModel `tfsdk:"checks"`
}

func (d *LivenessDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *LivenessDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_liveness"
}

func (d *LivenessDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "The liveness status.",
			},
		},
		Blocks: map[string]schema.Block{
			"checks": schema.ListNestedBlock{
				Description: "Liveness check details.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "Name of the liveness check.",
						},
						"status": schema.StringAttribute{
							Computed:    true,
							Description: "Status of the liveness check.",
						},
						"details": schema.MapAttribute{
							Computed:    true,
							ElementType: types.StringType,
							Description: "Additional details for the liveness check.",
						},
					},
				},
			},
		},
	}
}

func (d *LivenessDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state LivenessDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.GetLiveness(ctx)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading liveness", "liveness", err)...)
		return
	}

	state.Status = types.StringValue(result.Status)
	state.Checks = healthChecksToModel(ctx, result.Checks, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
