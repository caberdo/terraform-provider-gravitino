package authentication

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &PrincipalDataSource{}
var _ datasource.DataSourceWithConfigure = &PrincipalDataSource{}

type PrincipalDataSource struct {
	client *client.Client
}

func New() datasource.DataSource {
	return &PrincipalDataSource{}
}

func (d *PrincipalDataSource) SetClient(c *client.Client) {
	d.client = c
}

type PrincipalDataSourceModel struct {
	Name  types.String `tfsdk:"name"`
	Roles types.List   `tfsdk:"roles"`
}

func (d *PrincipalDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PrincipalDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_principal"
}

func (d *PrincipalDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "The principal name.",
			},
			"roles": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "The roles assigned to the principal.",
			},
		},
	}
}

func (d *PrincipalDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PrincipalDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.GetAuthenticatedPrincipal()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get authenticated principal", err.Error())
		return
	}

	config.Name = types.StringValue(result.Principal)
	config.Roles = types.ListValueMust(types.StringType, []attr.Value{})
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
