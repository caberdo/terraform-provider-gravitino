package secrets

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &SecretsDataSource{}
var _ datasource.DataSourceWithConfigure = &SecretsDataSource{}

type SecretsDataSource struct {
	client *client.Client
}

func New() datasource.DataSource {
	return &SecretsDataSource{}
}

func (d *SecretsDataSource) SetClient(c *client.Client) {
	d.client = c
}

type SecretsDataSourceModel struct {
	Metalake     types.String `tfsdk:"metalake"`
	ResourceType types.String `tfsdk:"resource_type"`
	Resource     types.String `tfsdk:"resource"`
	Secrets      types.Map    `tfsdk:"secrets"`
}

func (d *SecretsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SecretsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_secrets"
}

func (d *SecretsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Gets the resolved secrets for a metadata object. Requires Gravitino 1.4 or newer: the " +
			"secrets API does not exist in 1.3.x, where this data source fails with a 404.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"resource_type": schema.StringAttribute{
				Required:    true,
				Description: "The metadata object type (CATALOG, SCHEMA, FILESET).",
				Validators: []validator.String{
					stringvalidator.OneOf(models.ObjectTypeCatalog, models.ObjectTypeSchema, models.ObjectTypeFileset),
				},
			},
			"resource": schema.StringAttribute{
				Required:    true,
				Description: "The full name of the metadata object.",
			},
			"secrets": schema.MapAttribute{
				Computed:    true,
				Sensitive:   true,
				ElementType: types.StringType,
				Description: "The resolved plaintext secrets for the metadata object.",
			},
		},
	}
}

func (d *SecretsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SecretsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.GetSecrets(ctx,
		config.Metalake.ValueString(),
		config.ResourceType.ValueString(),
		config.Resource.ValueString(),
	)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError("Secrets not found", "The specified metadata object does not exist.")
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading secrets", config.Resource.ValueString(), err)...)
		return
	}

	secrets, diags := types.MapValueFrom(ctx, types.StringType, result.Secrets)
	resp.Diagnostics.Append(diags...)
	config.Secrets = secrets

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
