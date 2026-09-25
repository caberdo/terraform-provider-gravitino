package model

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

var _ datasource.DataSource = &ModelDataSource{}
var _ datasource.DataSourceWithConfigure = &ModelDataSource{}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

type ModelDataSource struct {
	client *client.Client
}

type ModelDataSourceModel struct {
	Metalake      types.String `tfsdk:"metalake"`
	Catalog       types.String `tfsdk:"catalog"`
	Schema        types.String `tfsdk:"schema"`
	Name          types.String `tfsdk:"name"`
	Comment       types.String `tfsdk:"comment"`
	LatestVersion types.Int64  `tfsdk:"latest_version"`
	Properties    types.Map    `tfsdk:"properties"`
	Audit         types.Object `tfsdk:"audit"`
}

func NewModelDataSource() datasource.DataSource {
	return &ModelDataSource{}
}

func (d *ModelDataSource) SetClient(c *client.Client) {
	d.client = c
}

func (d *ModelDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_model"
}

func (d *ModelDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves a single Gravitino model by name. Model artifacts are attached to the model versions of the model, not to the model itself.",
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
			"name": schema.StringAttribute{
				Description: "The model name.",
				Required:    true,
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
	}
}

func (d *ModelDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ModelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ModelDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	modelResp, err := d.client.GetModel(ctx, config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString(), config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading model", config.Name.ValueString(), err)...)
		return
	}

	config.Comment = optionalString(modelResp.Model.Comment)
	config.LatestVersion = types.Int64Value(int64(modelResp.Model.LatestVersion))
	config.Properties = mapValueFrom(ctx, modelResp.Model.Properties, &resp.Diagnostics)

	auditObj, auditDiags := auditToObject(modelResp.Model.Audit)
	resp.Diagnostics.Append(auditDiags...)
	config.Audit = auditObj

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func optionalString(apiValue string) types.String {
	if apiValue != "" {
		return types.StringValue(apiValue)
	}
	return types.StringNull()
}

func mapValueFrom(ctx context.Context, values map[string]string, diags *diag.Diagnostics) types.Map {
	if len(values) > 0 {
		value, d := types.MapValueFrom(ctx, types.StringType, values)
		diags.Append(d...)
		return value
	}
	return types.MapNull(types.StringType)
}

func auditToObject(a *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	if a == nil {
		return types.ObjectNull(AuditAttrTypes), nil
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

	return types.ObjectValue(AuditAttrTypes, map[string]attr.Value{
		"creator":            creator,
		"create_time":        createTime,
		"last_modifier":      lastModifier,
		"last_modified_time": lastModifiedTime,
	})
}
