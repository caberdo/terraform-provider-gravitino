package role

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RolesListDataSource{}
var _ datasource.DataSourceWithConfigure = &RolesListDataSource{}

type RolesListDataSource struct {
	client *client.Client
}

func NewRolesListDataSource() datasource.DataSource {
	return &RolesListDataSource{}
}

func (d *RolesListDataSource) SetClient(c *client.Client) {
	d.client = c
}

type RolesListDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Names    types.List   `tfsdk:"names"`
}

func (d *RolesListDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RolesListDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_roles_list"
}

func (d *RolesListDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
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

func (d *RolesListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config RolesListDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.ListAllRoles(ctx, config.Metalake.ValueString())
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
