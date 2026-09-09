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

var _ datasource.DataSource = &ModelVersionDataSource{}
var _ datasource.DataSourceWithConfigure = &ModelVersionDataSource{}

var dsAuditAttrTypes = map[string]attr.Type{
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
	Version    types.String `tfsdk:"version"`
	URI        types.String `tfsdk:"uri"`
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
		Description: "Retrieves a single Gravitino model version by identifier.",
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
			"version": schema.StringAttribute{
				Description: "The model version identifier.",
				Required:    true,
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
				AttributeTypes: dsAuditAttrTypes,
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

	mvResp, err := d.client.GetModelVersion(config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString(), config.Model.ValueString(), config.Version.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read model version", err.Error())
		return
	}

	mv := mvResp.ModelVersion
	config.URI = types.StringValue(mv.URI)
	config.Comment = types.StringValue(mv.Comment)

	if len(mv.Aliases) > 0 {
		aliasesList, d := types.SetValueFrom(ctx, types.StringType, mv.Aliases)
		resp.Diagnostics.Append(d...)
		config.Aliases = aliasesList
	} else {
		config.Aliases = types.SetNull(types.StringType)
	}

	if len(mv.Properties) > 0 {
		props, propsDiags := types.MapValueFrom(ctx, types.StringType, mv.Properties)
		resp.Diagnostics.Append(propsDiags...)
		config.Properties = props
	} else {
		config.Properties = types.MapNull(types.StringType)
	}

	auditObj, auditDiags := dsAuditToObject(mv.Audit)
	resp.Diagnostics.Append(auditDiags...)
	config.Audit = auditObj

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func dsAuditToObject(a *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	if a == nil {
		return types.ObjectNull(dsAuditAttrTypes), nil
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

	return types.ObjectValue(dsAuditAttrTypes, map[string]attr.Value{
		"creator":            creator,
		"create_time":        createTime,
		"last_modifier":      lastModifier,
		"last_modified_time": lastModifiedTime,
	})
}
