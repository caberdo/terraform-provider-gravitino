package model_version

import (
	"context"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var _ datasource.DataSource = &ModelVersionsDataSource{}
var _ datasource.DataSourceWithConfigure = &ModelVersionsDataSource{}

var dslAuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
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
		Description: "Lists all model versions within a Gravitino model.",
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
						"version": schema.StringAttribute{
							Description: "The model version identifier.",
							Computed:    true,
						},
						"uri": schema.StringAttribute{
							Description: "The URI of the model version artifact.",
							Computed:    true,
						},
						"aliases": schema.SetAttribute{
							Description: "Aliases for this model version.",
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
							AttributeTypes: dslAuditAttrTypes,
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

func versionListItemAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"version":    types.StringType,
		"uri":        types.StringType,
		"aliases":    types.SetType{ElemType: types.StringType},
		"comment":    types.StringType,
		"properties": types.MapType{ElemType: types.StringType},
		"audit":      types.ObjectType{AttrTypes: dslAuditAttrTypes},
	}
}

func (d *ModelVersionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ModelVersionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	versionsResp, err := d.client.ListModelVersions(config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString(), config.Model.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to list model versions", err.Error())
		return
	}

	items := make([]attr.Value, 0, len(versionsResp.ModelVersions))
	for i := range versionsResp.ModelVersions {
		item, itemDiags := versionListItemToObject(ctx, &versionsResp.ModelVersions[i])
		resp.Diagnostics.Append(itemDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		items = []attr.Value{}
	}

	listVal, listDiags := types.ListValue(
		types.ObjectType{AttrTypes: versionListItemAttrTypes()},
		items,
	)
	resp.Diagnostics.Append(listDiags...)
	config.Versions = listVal

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func versionListItemToObject(ctx context.Context, mv *models.ModelVersion) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	var aliasesList types.Set
	if len(mv.Aliases) > 0 {
		a, aDiags := types.SetValueFrom(ctx, types.StringType, mv.Aliases)
		diags.Append(aDiags...)
		aliasesList = a
	} else {
		aliasesList = types.SetNull(types.StringType)
	}

	var props types.Map
	if len(mv.Properties) > 0 {
		p, pDiags := types.MapValueFrom(ctx, types.StringType, mv.Properties)
		diags.Append(pDiags...)
		props = p
	} else {
		props = types.MapNull(types.StringType)
	}

	ao, aDiags := dslAuditToObject(mv.Audit)
	diags.Append(aDiags...)

	obj, oDiags := types.ObjectValue(versionListItemAttrTypes(), map[string]attr.Value{
		"version":    types.StringValue(mv.Version),
		"uri":        types.StringValue(mv.URI),
		"aliases":    aliasesList,
		"comment":    types.StringValue(mv.Comment),
		"properties": props,
		"audit":      ao,
	})
	diags.Append(oDiags...)

	return obj, diags
}

func dslAuditToObject(a *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	if a == nil {
		return types.ObjectNull(dslAuditAttrTypes), nil
	}

	creator := types.StringNull()
	if a.Creator != "" {
		creator = types.StringValue(a.Creator)
	}

	createTime := types.StringNull()
	if a.CreateTime != nil {
		createTime = types.StringValue(a.CreateTime.Format(time.RFC3339))
	}

	lastModifier := types.StringNull()
	if a.LastModifier != "" {
		lastModifier = types.StringValue(a.LastModifier)
	}

	lastModifiedTime := types.StringNull()
	if a.LastModifiedTime != nil {
		lastModifiedTime = types.StringValue(a.LastModifiedTime.Format(time.RFC3339))
	}

	return types.ObjectValue(dslAuditAttrTypes, map[string]attr.Value{
		"creator":            creator,
		"create_time":        createTime,
		"last_modifier":      lastModifier,
		"last_modified_time": lastModifiedTime,
	})
}
