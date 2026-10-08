package authentication

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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
	Name         types.String `tfsdk:"name"`
	ServiceAdmin types.Bool   `tfsdk:"service_admin"`
}

func (d *PrincipalDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *PrincipalDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_principal"
}

func (d *PrincipalDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Gets the server-resolved principal of the authenticated user (GET /api/authn/me) and, " +
			"on Gravitino 1.3.1 and later, whether that credential is a Gravitino service administrator. " +
			"The endpoint returns no roles.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "The server-resolved principal name of the authenticated user.",
			},
			"service_admin": schema.BoolAttribute{
				Computed: true,
				Description: "Whether the authenticated user is a Gravitino service administrator. " +
					"Always `false` on Gravitino versions before 1.3.1, which do not return `serviceAdmin`.",
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

	tflog.Debug(ctx, "Reading authenticated principal")

	result, err := d.client.GetAuthenticatedPrincipal(ctx)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading authenticated principal", "/authn/me", err)...)
		return
	}

	setPrincipalState(ctx, result, &config)

	tflog.Debug(ctx, "Read authenticated principal", map[string]interface{}{
		"principal":     config.Name.ValueString(),
		"service_admin": config.ServiceAdmin.ValueBool(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

// setPrincipalState maps the /authn/me response onto the data source model.
// `principal` is a plain string in the API response, so the mapped `name` is
// always a known value (possibly the empty string when the server omits it).
// `serviceAdmin` is absent on servers before 1.3.1, so the mapped
// `service_admin` is then a known `false` rather than an unknown value.
func setPrincipalState(ctx context.Context, result *models.AuthMeResponse, model *PrincipalDataSourceModel) {
	model.Name = types.StringValue(result.Principal)
	model.ServiceAdmin = types.BoolValue(result.ServiceAdmin)
	tflog.Debug(ctx, "Mapped authenticated principal", map[string]interface{}{
		"principal":     result.Principal,
		"service_admin": result.ServiceAdmin,
		"code":          result.Code,
	})
}
