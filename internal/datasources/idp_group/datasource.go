package idp_group

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &IdpGroupDataSource{}
var _ datasource.DataSourceWithConfigure = &IdpGroupDataSource{}

// IdpGroupDataSourceDescription is the schema description shown in the generated docs.
// The built-in IDP REST API is not part of a default Gravitino 1.3.0 server:
// it needs `gravitino.authenticators = basic` (without `simple`),
// `gravitino.server.rest.extensionPackages = org.apache.gravitino.idp.web.rest.feature`
// and a service admin from `gravitino.authorization.serviceAdmins`. Without
// that configuration every IDP endpoint answers HTTP 404.
const IdpGroupDataSourceDescription = "Gets a built-in IDP group; the built-in IDP REST API needs " +
	"`gravitino.authenticators = basic` (without `simple`) and " +
	"`gravitino.server.rest.extensionPackages = org.apache.gravitino.idp.web.rest.feature`, " +
	"and calls must come from a `gravitino.authorization.serviceAdmins` service admin, " +
	"otherwise the endpoint answers HTTP 404."

type IdpGroupDataSource struct {
	client *client.Client
}

func NewDataSource() datasource.DataSource {
	return &IdpGroupDataSource{}
}

func (d *IdpGroupDataSource) SetClient(c *client.Client) {
	d.client = c
}

type IdpGroupDataSourceModel struct {
	Name  types.String `tfsdk:"name"`
	Users types.Set    `tfsdk:"users"`
}

func (d *IdpGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid provider data",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue.", req.ProviderData),
		)
		return
	}
	d.client = c
}

func (d *IdpGroupDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_idp_group"
}

func (d *IdpGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: IdpGroupDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The group name.",
			},
			"users": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "The usernames of members in the group.",
			},
		},
	}
}

func (d *IdpGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config IdpGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.GetIdpGroup(ctx, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading IDP group", config.Name.ValueString(), err)...)
		return
	}

	config.Name = types.StringValue(result.Group.Name)
	users, diags := stringSliceToList(ctx, result.Group.Users)
	resp.Diagnostics.Append(diags...)
	config.Users = users

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func stringSliceToList(ctx context.Context, items []string) (types.Set, diag.Diagnostics) {
	vals := make([]attr.Value, 0, len(items))
	for _, s := range items {
		vals = append(vals, types.StringValue(s))
	}
	return types.SetValue(types.StringType, vals)
}
