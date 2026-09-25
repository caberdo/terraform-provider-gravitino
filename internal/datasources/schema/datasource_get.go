package schema

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

var _ datasource.DataSource = &SchemaDataSource{}
var _ datasource.DataSourceWithConfigure = &SchemaDataSource{}

var dsAuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

type SchemaDataSource struct {
	client *client.Client
}

type SchemaDataSourceModel struct {
	Metalake   types.String `tfsdk:"metalake"`
	Catalog    types.String `tfsdk:"catalog"`
	Name       types.String `tfsdk:"name"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
}

func NewSchemaDataSource() datasource.DataSource {
	return &SchemaDataSource{}
}

func (d *SchemaDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_schema"
}

func (d *SchemaDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves a single Gravitino schema by name.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Description: "The metalake name.",
				Required:    true,
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog name.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The schema name.",
				Required:    true,
			},
			"comment": schema.StringAttribute{
				Description: "The schema comment.",
				Computed:    true,
			},
			"properties": schema.MapAttribute{
				Description: "Key-value properties for the schema.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"audit": schema.ObjectAttribute{
				Description:    "Audit information for the schema.",
				Computed:       true,
				AttributeTypes: dsAuditAttrTypes,
			},
		},
	}
}

func (ds *SchemaDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider data", "Expected *client.Client, got unexpected type.")
		return
	}
	ds.client = c
}

func (ds *SchemaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SchemaDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	schemaResp, err := ds.client.GetSchema(ctx, config.Metalake.ValueString(), config.Catalog.ValueString(), config.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Schema not found",
				fmt.Sprintf("No schema %q exists in metalake %q, catalog %q.", config.Name.ValueString(), config.Metalake.ValueString(), config.Catalog.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading schema", config.Name.ValueString(), err)...)
		return
	}

	config.Comment = types.StringValue(schemaResp.Schema.Comment)

	if len(schemaResp.Schema.Properties) > 0 {
		props, d := types.MapValueFrom(ctx, types.StringType, schemaResp.Schema.Properties)
		resp.Diagnostics.Append(d...)
		config.Properties = props
	} else {
		config.Properties = types.MapNull(types.StringType)
	}

	auditObj, d := dsAuditToObject(schemaResp.Schema.Audit)
	resp.Diagnostics.Append(d...)
	config.Audit = auditObj

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func dsAuditToObject(audit *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(dsAuditAttrTypes), nil
	}

	creator := types.StringNull()
	if audit.Creator != "" {
		creator = types.StringValue(audit.Creator)
	}

	createTime := types.StringNull()
	if audit.CreateTime != nil {
		createTime = types.StringValue(audit.CreateTime.Format(time.RFC3339))
	}

	lastModifier := types.StringNull()
	if audit.LastModifier != "" {
		lastModifier = types.StringValue(audit.LastModifier)
	}

	lastModifiedTime := types.StringNull()
	if audit.LastModifiedTime != nil {
		lastModifiedTime = types.StringValue(audit.LastModifiedTime.Format(time.RFC3339))
	}

	return types.ObjectValue(dsAuditAttrTypes, map[string]attr.Value{
		"creator":            creator,
		"create_time":        createTime,
		"last_modifier":      lastModifier,
		"last_modified_time": lastModifiedTime,
	})
}
