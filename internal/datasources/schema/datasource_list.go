package schema

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

var _ datasource.DataSource = &SchemasDataSource{}
var _ datasource.DataSourceWithConfigure = &SchemasDataSource{}

var dslAuditAttrTypes = models.AuditAttrTypes

type SchemasDataSource struct {
	client *client.Client
}

type SchemasDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Catalog  types.String `tfsdk:"catalog"`
	Schemas  types.List   `tfsdk:"schemas"`
}

func NewSchemasDataSource() datasource.DataSource {
	return &SchemasDataSource{}
}

func (d *SchemasDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_schemas"
}

func (d *SchemasDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all schemas within a Gravitino metalake and catalog.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Description: "The metalake name.",
				Required:    true,
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog name.",
				Required:    true,
			},
			"schemas": schema.ListNestedAttribute{
				Description: "List of schemas with their details.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The schema name.",
							Computed:    true,
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
							AttributeTypes: dslAuditAttrTypes,
						},
					},
				},
			},
		},
	}
}

func (ds *SchemasDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		ds.client = c
	}
}

func (ds *SchemasDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SchemasDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	schemas, err := ds.client.ListSchemasDetails(ctx, config.Metalake.ValueString(), config.Catalog.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Metalake or catalog not found",
				fmt.Sprintf("No metalake %q or catalog %q exists.", config.Metalake.ValueString(), config.Catalog.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("listing schemas", config.Catalog.ValueString(), err)...)
		return
	}

	items := make([]attr.Value, 0, len(schemas))
	for i := range schemas {
		item, d := dslSchemaListItemToObject(ctx, &schemas[i])
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		items = []attr.Value{}
	}

	listVal, d := types.ListValue(
		types.ObjectType{AttrTypes: dslSchemaListItemAttrTypes()},
		items,
	)
	resp.Diagnostics.Append(d...)
	config.Schemas = listVal

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func dslSchemaListItemAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":       types.StringType,
		"comment":    types.StringType,
		"properties": types.MapType{ElemType: types.StringType},
		"audit":      types.ObjectType{AttrTypes: dslAuditAttrTypes},
	}
}

func dslSchemaListItemToObject(ctx context.Context, s *models.Schema) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	var props types.Map
	if len(s.Properties) > 0 {
		p, d := types.MapValueFrom(ctx, types.StringType, s.Properties)
		diags.Append(d...)
		props = p
	} else {
		props = types.MapNull(types.StringType)
	}

	auditObj, d := dslAuditToObject(s.Audit)
	diags.Append(d...)

	obj, d := types.ObjectValue(dslSchemaListItemAttrTypes(), map[string]attr.Value{
		"name":       types.StringValue(s.Name),
		"comment":    types.StringValue(s.Comment),
		"properties": props,
		"audit":      auditObj,
	})
	diags.Append(d...)

	return obj, diags
}

func dslAuditToObject(audit *models.Audit) (types.Object, diag.Diagnostics) {
	return models.AuditToObjectValue(context.Background(), audit)
}
