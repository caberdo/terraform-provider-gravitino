package owner

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

var _ datasource.DataSource = &OwnerDataSource{}
var _ datasource.DataSourceWithConfigure = &OwnerDataSource{}

type OwnerDataSource struct {
	client *client.Client
}

func NewOwnerDataSource() datasource.DataSource {
	return &OwnerDataSource{}
}

func (d *OwnerDataSource) SetClient(c *client.Client) {
	d.client = c
}

type OwnerDataSourceModel struct {
	Metalake       types.String `tfsdk:"metalake"`
	ObjectType     types.String `tfsdk:"object_type"`
	ObjectFullName types.String `tfsdk:"object_full_name"`
	OwnerName      types.String `tfsdk:"owner_name"`
	OwnerType      types.String `tfsdk:"owner_type"`
}

func (d *OwnerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *OwnerDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_owner"
}

func (d *OwnerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"object_type": schema.StringAttribute{
				Required:    true,
				Description: "The metadata object type. One of: METALAKE, CATALOG, SCHEMA, TABLE, FILESET, TOPIC, ROLE (upper case singular, as required by the API path).",
				Validators: []validator.String{
					stringvalidator.OneOf(models.OwnerObjectTypes...),
				},
			},
			"object_full_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the metadata object, relative to the metalake (the API rejects a metalake prefix with HTTP 400 IllegalNamespaceException): METALAKE = the metalake name, CATALOG = the catalog name, SCHEMA = 'catalog.schema', TABLE = 'catalog.schema.table', and likewise for FILESET, TOPIC and ROLE.",
			},
			"owner_name": schema.StringAttribute{
				Computed:    true,
				Description: "The owner name.",
			},
			"owner_type": schema.StringAttribute{
				Computed:    true,
				Description: "The owner type (USER or GROUP). Gravitino matches this case-insensitively and always answers in lowercase; the provider normalises responses back to upper case.",
			},
		},
	}
}

func (d *OwnerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config OwnerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.GetOwner(ctx,
		config.Metalake.ValueString(),
		config.ObjectType.ValueString(),
		config.ObjectFullName.ValueString(),
	)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Owner not found",
				fmt.Sprintf("No owner is set for %s %q in metalake %q.", config.ObjectType.ValueString(), config.ObjectFullName.ValueString(), config.Metalake.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading owner", config.ObjectFullName.ValueString(), err)...)
		return
	}

	config.OwnerName = types.StringValue(result.Owner.Name)
	config.OwnerType = types.StringValue(models.NormalizeOwnerType(result.Owner.Type))

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
