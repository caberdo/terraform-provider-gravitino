package role

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/providerbase"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &MetalakeRolesDataSource{}
var _ datasource.DataSourceWithConfigure = &MetalakeRolesDataSource{}

type MetalakeRolesDataSource struct {
	providerbase.DataSource
}

func NewMetalakeRolesDataSource() datasource.DataSource {
	return &MetalakeRolesDataSource{}
}

type MetalakeRolesDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Names    types.List   `tfsdk:"names"`
}

func (d *MetalakeRolesDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_metalake_roles"
}

func (d *MetalakeRolesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"names": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "The list of role names in the metalake.",
			},
		},
	}
}

func (d *MetalakeRolesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config MetalakeRolesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.Client.ListAllRoles(ctx, config.Metalake.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("listing roles in", config.Metalake.ValueString(), err)...)
		return
	}

	items := make([]attr.Value, 0, len(result.Names))
	for _, name := range result.Names {
		items = append(items, types.StringValue(name))
	}

	namesList, listDiags := types.ListValue(types.StringType, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Names = namesList
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
