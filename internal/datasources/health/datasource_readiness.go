package health

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = (*ReadinessDataSource)(nil)
var _ datasource.DataSourceWithConfigure = &ReadinessDataSource{}

type ReadinessDataSource struct {
	client *client.Client
}

func NewReadinessDataSource() datasource.DataSource {
	return &ReadinessDataSource{}
}

func (d *ReadinessDataSource) SetClient(c *client.Client) {
	d.client = c
}

type ReadinessDataSourceModel struct {
	Status types.String       `tfsdk:"status"`
	Checks []HealthCheckModel `tfsdk:"checks"`
}

func (d *ReadinessDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ReadinessDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_readiness"
}

func (d *ReadinessDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "The readiness status.",
			},
		},
		Blocks: map[string]schema.Block{
			"checks": schema.ListNestedBlock{
				Description: "Readiness check details.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "Name of the readiness check.",
						},
						"status": schema.StringAttribute{
							Computed:    true,
							Description: "Status of the readiness check.",
						},
						"details": schema.MapAttribute{
							Computed:    true,
							ElementType: types.StringType,
							Description: "Additional details for the readiness check.",
						},
					},
				},
			},
		},
	}
}

func (d *ReadinessDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state ReadinessDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.GetReadiness(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read readiness", err.Error())
		return
	}

	state.Status = types.StringValue(result.Status)
	state.Checks = healthChecksToModel(ctx, result.Checks, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
