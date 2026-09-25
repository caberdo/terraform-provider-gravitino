package model_version

import (
	"context"
	"fmt"
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

var _ datasource.DataSource = &ModelVersionDataSource{}
var _ datasource.DataSourceWithConfigure = &ModelVersionDataSource{}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

type ModelVersionDataSource struct {
	client *client.Client
}

func NewModelVersionDataSource() datasource.DataSource {
	return &ModelVersionDataSource{}
}

func (d *ModelVersionDataSource) SetClient(c *client.Client) {
	d.client = c
}

type ModelVersionDataSourceModel struct {
	Metalake   types.String `tfsdk:"metalake"`
	Catalog    types.String `tfsdk:"catalog"`
	Schema     types.String `tfsdk:"schema"`
	Model      types.String `tfsdk:"model"`
	Version    types.Int64  `tfsdk:"version"`
	Alias      types.String `tfsdk:"alias"`
	URI        types.String `tfsdk:"uri"`
	URIs       types.Map    `tfsdk:"uris"`
	Aliases    types.Set    `tfsdk:"aliases"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
}

func (d *ModelVersionDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_model_version"
}

func (d *ModelVersionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves a single Gravitino model version, either by version number or by alias.",
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
			"version": schema.Int64Attribute{
				Description: "The model version number. Exactly one of version and alias must be set.",
				Optional:    true,
				Computed:    true,
			},
			"alias": schema.StringAttribute{
				Description: "An alias of the model version. Exactly one of version and alias must be set.",
				Optional:    true,
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
	}
}

func (d *ModelVersionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ModelVersionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ModelVersionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := config.Metalake.ValueString()
	catalog := config.Catalog.ValueString()
	schemaName := config.Schema.ValueString()
	model := config.Model.ValueString()

	hasVersion := !config.Version.IsNull() && !config.Version.IsUnknown()
	hasAlias := !config.Alias.IsNull() && !config.Alias.IsUnknown()
	if hasVersion == hasAlias {
		resp.Diagnostics.AddError(
			"Invalid model version lookup",
			"Exactly one of version and alias must be set.",
		)
		return
	}

	var (
		result   *models.ModelVersionResponse
		err      error
		resource string
	)
	if hasAlias {
		resource = fmt.Sprintf("%s alias %q", model, config.Alias.ValueString())
		result, err = d.client.GetModelVersionByAlias(ctx, metalake, catalog, schemaName, model, config.Alias.ValueString())
	} else {
		resource = fmt.Sprintf("%s version %d", model, config.Version.ValueInt64())
		result, err = d.client.GetModelVersion(ctx, metalake, catalog, schemaName, model, int32(config.Version.ValueInt64()))
	}
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading model version", resource, err)...)
		return
	}

	mv := result.ModelVersion
	config.Version = types.Int64Value(int64(mv.Version))
	config.URI = optionalString(mv.URI)
	config.Comment = optionalString(mv.Comment)
	config.URIs = mapValueFrom(ctx, mv.URIs, &resp.Diagnostics)
	config.Properties = mapValueFrom(ctx, mv.Properties, &resp.Diagnostics)
	config.Aliases = setValueFrom(ctx, mv.Aliases, &resp.Diagnostics)

	auditObj, auditDiags := auditToObject(mv.Audit)
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

func setValueFrom(ctx context.Context, values []string, diags *diag.Diagnostics) types.Set {
	if len(values) > 0 {
		value, d := types.SetValueFrom(ctx, types.StringType, values)
		diags.Append(d...)
		return value
	}
	return types.SetNull(types.StringType)
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
