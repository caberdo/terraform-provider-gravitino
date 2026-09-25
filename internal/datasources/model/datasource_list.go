package model

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

var _ datasource.DataSource = &ModelsDataSource{}
var _ datasource.DataSourceWithConfigure = &ModelsDataSource{}

// ModelItemAttrTypes describes one entry of the `models` list.
var ModelItemAttrTypes = map[string]attr.Type{
	"name":           types.StringType,
	"comment":        types.StringType,
	"latest_version": types.Int64Type,
	"properties":     types.MapType{ElemType: types.StringType},
	"audit":          types.ObjectType{AttrTypes: AuditAttrTypes},
}

type ModelsDataSource struct {
	client *client.Client
}

type ModelsDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Catalog  types.String `tfsdk:"catalog"`
	Schema   types.String `tfsdk:"schema"`
	Models   types.List   `tfsdk:"models"`
}

func NewModelsDataSource() datasource.DataSource {
	return &ModelsDataSource{}
}

func (d *ModelsDataSource) SetClient(c *client.Client) {
	d.client = c
}

func (d *ModelsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_models"
}

func (d *ModelsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all models within a Gravitino metalake, catalog, and schema.",
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
			"models": schema.ListNestedAttribute{
				Description: "List of models with their details.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The model name.",
							Computed:    true,
						},
						"comment": schema.StringAttribute{
							Description: "The model comment.",
							Computed:    true,
						},
						"latest_version": schema.Int64Attribute{
							Description: "The latest version number of the model.",
							Computed:    true,
						},
						"properties": schema.MapAttribute{
							Description: "Key-value properties for the model.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"audit": schema.ObjectAttribute{
							Description:    "Audit information for the model.",
							Computed:       true,
							AttributeTypes: AuditAttrTypes,
						},
					},
				},
			},
		},
	}
}

func (d *ModelsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ModelsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ModelsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mods, err := d.client.ListModelsDetails(ctx, config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("listing models", fmt.Sprintf("%s.%s.%s", config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString()), err)...)
		return
	}

	items := make([]attr.Value, 0, len(mods))
	for i := range mods {
		item, itemDiags := modelListItemToObject(ctx, &mods[i])
		resp.Diagnostics.Append(itemDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, item)
	}

	listVal, listDiags := types.ListValue(
		types.ObjectType{AttrTypes: ModelItemAttrTypes},
		items,
	)
	resp.Diagnostics.Append(listDiags...)
	config.Models = listVal

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func modelListItemToObject(ctx context.Context, m *models.Model) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	props := mapValueFrom(ctx, m.Properties, &diags)

	ao, aDiags := auditToObject(m.Audit)
	diags.Append(aDiags...)

	obj, oDiags := types.ObjectValue(ModelItemAttrTypes, map[string]attr.Value{
		"name":           types.StringValue(m.Name),
		"comment":        optionalString(m.Comment),
		"latest_version": types.Int64Value(int64(m.LatestVersion)),
		"properties":     props,
		"audit":          ao,
	})
	diags.Append(oDiags...)

	return obj, diags
}
