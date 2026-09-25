package catalog

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &CatalogDataSource{}
var _ datasource.DataSourceWithConfigure = &CatalogDataSource{}

type CatalogDataSource struct {
	client *client.Client
}

func NewGetDataSource() datasource.DataSource {
	return &CatalogDataSource{}
}

func (d *CatalogDataSource) SetClient(c *client.Client) {
	d.client = c
}

type CatalogDataSourceModel struct {
	Metalake   types.String `tfsdk:"metalake"`
	Name       types.String `tfsdk:"name"`
	Type       types.String `tfsdk:"type"`
	Provider   types.String `tfsdk:"catalog_provider"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
}

func (d *CatalogDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CatalogDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_catalog"
}

func (d *CatalogDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The catalog name.",
			},
			"type": schema.StringAttribute{
				Computed:    true,
				Description: "The catalog type.",
			},
			"catalog_provider": schema.StringAttribute{
				Computed:    true,
				Description: "The catalog provider.",
			},
			"comment": schema.StringAttribute{
				Computed:    true,
				Description: "The catalog comment.",
			},
			"properties": schema.MapAttribute{
				Computed:    true,
				Sensitive:   true,
				ElementType: types.StringType,
				Description: "The catalog properties.",
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
				Description:    "Audit information for the catalog.",
			},
		},
	}
}

func (d *CatalogDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CatalogDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.GetCatalog(ctx, config.Metalake.ValueString(), config.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Catalog not found",
				fmt.Sprintf("No catalog %q exists in metalake %q.", config.Name.ValueString(), config.Metalake.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading catalog", config.Name.ValueString(), err)...)
		return
	}

	setDataSourceStateFromCatalog(ctx, &resp.Diagnostics, &result.Catalog, &config)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func setDataSourceStateFromCatalog(ctx context.Context, diags *diag.Diagnostics, catalog *models.Catalog, model *CatalogDataSourceModel) {
	model.Type = types.StringValue(catalog.Type)
	model.Provider = types.StringValue(catalog.Provider)
	model.Comment = types.StringValue(catalog.Comment)

	props, d := types.MapValueFrom(ctx, types.StringType, catalog.Properties)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Properties = props

	auditObj, d := auditToObjectValueForDS(ctx, catalog.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Audit = auditObj
}
