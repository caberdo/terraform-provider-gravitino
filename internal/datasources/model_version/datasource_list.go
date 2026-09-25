package model_version

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ModelVersionsDataSource{}
var _ datasource.DataSourceWithConfigure = &ModelVersionsDataSource{}

// ModelVersionItemAttrTypes describes one entry of the `versions` list.
var ModelVersionItemAttrTypes = map[string]attr.Type{
	"version":    types.Int64Type,
	"uri":        types.StringType,
	"uris":       types.MapType{ElemType: types.StringType},
	"aliases":    types.SetType{ElemType: types.StringType},
	"comment":    types.StringType,
	"properties": types.MapType{ElemType: types.StringType},
	"audit":      types.ObjectType{AttrTypes: AuditAttrTypes},
}

type ModelVersionsDataSource struct {
	client *client.Client
}

func NewModelVersionsDataSource() datasource.DataSource {
	return &ModelVersionsDataSource{}
}

func (d *ModelVersionsDataSource) SetClient(c *client.Client) {
	d.client = c
}

type ModelVersionsDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Catalog  types.String `tfsdk:"catalog"`
	Schema   types.String `tfsdk:"schema"`
	Model    types.String `tfsdk:"model"`
	Versions types.List   `tfsdk:"versions"`
}

func (d *ModelVersionsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_model_versions"
}

func (d *ModelVersionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all model versions of a Gravitino model.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Description: "The metalake name.",
				Required:    true,
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog name.",
				Required:    true,
			},
			"schema": schema.StringAttribute{
				Description: "The schema name.",
				Required:    true,
			},
			"model": schema.StringAttribute{
				Description: "The model name.",
				Required:    true,
			},
			"versions": schema.ListNestedAttribute{
				Description: "List of model versions with their details.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"version": schema.Int64Attribute{
							Description: "The model version number.",
							Computed:    true,
						},
						"uri": schema.StringAttribute{
							Description: "The unnamed URI of the model artifact.",
							Computed:    true,
						},
						"uris": schema.MapAttribute{
							Description: "The URIs of the model artifact, keyed by URI name.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"aliases": schema.SetAttribute{
							Description: "Aliases of the model version.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"comment": schema.StringAttribute{
							Description: "The model version comment.",
							Computed:    true,
						},
						"properties": schema.MapAttribute{
							Description: "Key-value properties for the model version.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"audit": schema.ObjectAttribute{
							Description:    "Audit information for the model version.",
							Computed:       true,
							AttributeTypes: AuditAttrTypes,
						},
					},
				},
			},
		},
	}
}

func (d *ModelVersionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider data", "Expected *client.Client, got unexpected type.")
		return
	}
	d.client = c
}

func (d *ModelVersionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ModelVersionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := config.Metalake.ValueString()
	catalog := config.Catalog.ValueString()
	schemaName := config.Schema.ValueString()
	model := config.Model.ValueString()

	versionsResp, err := d.client.ListModelVersions(ctx, metalake, catalog, schemaName, model, true)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("listing model versions", model, err)...)
		return
	}

	versions := versionsResp.Infos
	// Gravitino answers with version numbers instead of model version objects
	// when it does not honor details=true; fetch them individually then.
	if len(versions) == 0 && len(versionsResp.Versions) > 0 {
		versions = make([]models.ModelVersion, 0, len(versionsResp.Versions))
		for _, version := range versionsResp.Versions {
			result, err := d.client.GetModelVersion(ctx, metalake, catalog, schemaName, model, version)
			if err != nil {
				resp.Diagnostics.Append(client.NewResourceError("reading model version", fmt.Sprintf("%s version %d", model, version), err)...)
				return
			}
			versions = append(versions, result.ModelVersion)
		}
	}

	items := make([]attr.Value, 0, len(versions))
	for i := range versions {
		item, itemDiags := modelVersionListItemToObject(ctx, &versions[i])
		resp.Diagnostics.Append(itemDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, item)
	}

	listVal, listDiags := types.ListValue(
		types.ObjectType{AttrTypes: ModelVersionItemAttrTypes},
		items,
	)
	resp.Diagnostics.Append(listDiags...)
	config.Versions = listVal

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func modelVersionListItemToObject(ctx context.Context, mv *models.ModelVersion) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	ao, aDiags := auditToObject(mv.Audit)
	diags.Append(aDiags...)

	obj, oDiags := types.ObjectValue(ModelVersionItemAttrTypes, map[string]attr.Value{
		"version":    types.Int64Value(int64(mv.Version)),
		"uri":        optionalString(mv.URI),
		"uris":       mapValueFrom(ctx, mv.URIs, &diags),
		"aliases":    setValueFrom(ctx, mv.Aliases, &diags),
		"comment":    optionalString(mv.Comment),
		"properties": mapValueFrom(ctx, mv.Properties, &diags),
		"audit":      ao,
	})
	diags.Append(oDiags...)

	return obj, diags
}
