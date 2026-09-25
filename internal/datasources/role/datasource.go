package role

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RolesDataSource{}
var _ datasource.DataSourceWithConfigure = &RolesDataSource{}

type RolesDataSource struct {
	client *client.Client
}

func New() datasource.DataSource {
	return &RolesDataSource{}
}

func (d *RolesDataSource) SetClient(c *client.Client) {
	d.client = c
}

type RolesDataSourceModel struct {
	Metalake     types.String `tfsdk:"metalake"`
	ResourceType types.String `tfsdk:"resource_type"`
	Resource     types.String `tfsdk:"resource"`
	Names        types.List   `tfsdk:"names"`
}

func (d *RolesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RolesDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_roles"
}

func (d *RolesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"resource_type": schema.StringAttribute{
				Required:    true,
				Description: "The type of the metadata object that owns the roles: METALAKE, CATALOG, SCHEMA, TABLE, FILESET, TOPIC, ROLE, MODEL, FUNCTION, TAG, POLICY or JOB_TEMPLATE.",
				Validators: []validator.String{
					stringvalidator.OneOf(models.AllObjectTypes...),
				},
			},
			"resource": schema.StringAttribute{
				Required: true,
				Description: "The full name of the metadata object, relative to the metalake (without the metalake prefix): " +
					"the metalake name itself for a METALAKE, 'my_catalog' for a CATALOG, 'my_catalog.my_schema' for a " +
					"SCHEMA and 'my_catalog.my_schema.my_table' for a TABLE. Gravitino rejects a metalake prefix on " +
					"anything but a METALAKE with HTTP 400 IllegalNamespaceException.",
			},
			"names": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "The names of the roles attached to the metadata object (GET /metalakes/{metalake}/objects/{metadataObjectType}/{metadataObjectFullName}/roles returns a plain name list).",
			},
		},
	}
}

func (d *RolesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config RolesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := config.Metalake.ValueString()
	resource := config.Resource.ValueString()

	result, err := d.client.ListRoles(ctx, metalake, config.ResourceType.ValueString(), resource)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("listing roles for", resource, err)...)
		return
	}

	items := make([]attr.Value, 0, len(result.Names))
	for _, name := range result.Names {
		items = append(items, types.StringValue(name))
	}

	names, listDiags := types.ListValue(types.StringType, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Names = names
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
