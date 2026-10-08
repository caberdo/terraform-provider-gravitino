package function

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &FunctionsDataSource{}
var _ datasource.DataSourceWithConfigure = &FunctionsDataSource{}

var dslAuditAttrTypes = models.AuditAttrTypes

type FunctionsDataSource struct {
	client *client.Client
}

type FunctionsDataSourceModel struct {
	Metalake  types.String `tfsdk:"metalake"`
	Catalog   types.String `tfsdk:"catalog"`
	Schema    types.String `tfsdk:"schema"`
	Functions types.List   `tfsdk:"functions"`
}

func NewFunctionsDataSource() datasource.DataSource {
	return &FunctionsDataSource{}
}

// SetClient injects the API client; used by tests.
func (d *FunctionsDataSource) SetClient(c *client.Client) {
	d.client = c
}

func (d *FunctionsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_functions"
}

func (d *FunctionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all functions within a Gravitino metalake, catalog and schema.",
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
			"functions": schema.ListNestedAttribute{
				Description: "List of functions with their details.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The function name.",
							Computed:    true,
						},
						"function_type": schema.StringAttribute{
							Description: "The type of the function (SCALAR, AGGREGATE or TABLE).",
							Computed:    true,
						},
						"deterministic": schema.BoolAttribute{
							Description: "Whether the function is deterministic.",
							Computed:    true,
						},
						"comment": schema.StringAttribute{
							Description: "The function comment.",
							Computed:    true,
						},
						"definitions": functionDefinitionsAttribute(),
						"audit": schema.ObjectAttribute{
							Description:    "Audit information for the function.",
							Computed:       true,
							AttributeTypes: dslAuditAttrTypes,
						},
					},
				},
			},
		},
	}
}

func (d *FunctionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *FunctionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config FunctionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	functions, err := d.client.ListFunctionsDetails(ctx, config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("listing functions", config.Schema.ValueString(), err)...)
		return
	}

	items := make([]attr.Value, 0, len(functions))
	for i := range functions {
		item, diags := dslFunctionListItemToObject(ctx, &functions[i])
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		items = []attr.Value{}
	}

	list, diags := types.ListValue(types.ObjectType{AttrTypes: dslFunctionListItemAttrTypes()}, items)
	resp.Diagnostics.Append(diags...)
	config.Functions = list

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func dslFunctionListItemAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":          types.StringType,
		"function_type": types.StringType,
		"deterministic": types.BoolType,
		"comment":       types.StringType,
		"definitions":   models.FunctionDefinitionsListType(),
		"audit":         types.ObjectType{AttrTypes: dslAuditAttrTypes},
	}
}

func dslFunctionListItemToObject(ctx context.Context, f *models.Function) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	definitions, d := models.FunctionDefinitionsToTF(ctx, f.Definitions)
	diags.Append(d...)

	audit, d := dslAuditToObject(f.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return types.ObjectNull(dslFunctionListItemAttrTypes()), diags
	}

	comment := types.StringNull()
	if f.Comment != "" {
		comment = types.StringValue(f.Comment)
	}

	return types.ObjectValue(dslFunctionListItemAttrTypes(), map[string]attr.Value{
		"name":          types.StringValue(f.Name),
		"function_type": types.StringValue(models.NormalizeFunctionType(f.FunctionType)),
		"deterministic": types.BoolValue(f.Deterministic),
		"comment":       comment,
		"definitions":   definitions,
		"audit":         audit,
	})
}

func dslAuditToObject(audit *models.Audit) (types.Object, diag.Diagnostics) {
	return models.AuditToObjectValue(context.Background(), audit)
}
