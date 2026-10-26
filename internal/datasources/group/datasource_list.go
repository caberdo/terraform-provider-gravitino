package group

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/providerbase"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &GroupsDataSource{}
var _ datasource.DataSourceWithConfigure = &GroupsDataSource{}

type GroupsDataSource struct {
	providerbase.DataSource
}

func NewListDataSource() datasource.DataSource {
	return &GroupsDataSource{}
}

type GroupsDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Names    types.List   `tfsdk:"names"`
}

func (d *GroupsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_groups"
}

func (d *GroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"names": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "The list of group names.",
			},
		},
	}
}

func (d *GroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config GroupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.Client.ListGroups(ctx, config.Metalake.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Metalake not found",
				fmt.Sprintf("No metalake %q exists; cannot list its groups.", config.Metalake.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("listing groups", config.Metalake.ValueString(), err)...)
		return
	}

	names, diags := types.ListValueFrom(ctx, types.StringType, result.Names)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Names = names
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
