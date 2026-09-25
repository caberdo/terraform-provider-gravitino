package metalake

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = (*MetalakeDataSource)(nil)

type MetalakeDataSource struct {
	client *client.Client
}

type MetalakeDataSourceModel struct {
	Name       types.String `tfsdk:"name"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
}

func NewMetalakeDataSource() datasource.DataSource {
	return &MetalakeDataSource{}
}

func (d *MetalakeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_metalake"
}

func (d *MetalakeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
			},
			"comment": schema.StringAttribute{
				Computed: true,
			},
			"properties": schema.MapAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: models.AuditAttrTypes,
			},
		},
	}
}

func (d *MetalakeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cli, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = cli
}

func (d *MetalakeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state MetalakeDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.GetMetalake(ctx, state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Metalake not found",
				fmt.Sprintf("No metalake %q exists.", state.Name.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading metalake", state.Name.ValueString(), err)...)
		return
	}

	state.Comment = types.StringValue(result.Metalake.Comment)
	state.Properties = propertiesToMapDS(ctx, result.Metalake.Properties, &resp.Diagnostics)

	auditObj, adiags := models.AuditToObjectValue(ctx, result.Metalake.Audit)
	resp.Diagnostics.Append(adiags...)
	state.Audit = auditObj

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
