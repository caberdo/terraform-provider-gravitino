package function

import (
	"context"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var _ datasource.DataSource = &FunctionDataSource{}
var _ datasource.DataSourceWithConfigure = &FunctionDataSource{}

var dsAuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

type FunctionDataSource struct {
	client *client.Client
}

type FunctionDataSourceModel struct {
	Metalake      types.String `tfsdk:"metalake"`
	Catalog       types.String `tfsdk:"catalog"`
	Schema        types.String `tfsdk:"schema"`
	Name          types.String `tfsdk:"name"`
	FunctionType  types.String `tfsdk:"function_type"`
	Deterministic types.Bool   `tfsdk:"deterministic"`
	Comment       types.String `tfsdk:"comment"`
	Definitions   types.List   `tfsdk:"definitions"`
	Audit         types.Object `tfsdk:"audit"`
}

func NewFunctionDataSource() datasource.DataSource {
	return &FunctionDataSource{}
}

// SetClient injects the API client; used by tests.
func (d *FunctionDataSource) SetClient(c *client.Client) {
	d.client = c
}

func (d *FunctionDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_function"
}

func (d *FunctionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves a single Gravitino function by name.",
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
				Description: "The function name.",
				Required:    true,
			},
			"function_type": schema.StringAttribute{
				Description: "The type of the function (SCALAR, AGGREGATE or TABLE).",
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(models.AllFunctionTypes...),
				},
			},
			"deterministic": schema.BoolAttribute{
				Description: "Whether the function is deterministic.",
				Computed:    true,
			},
			"comment": schema.StringAttribute{
				Description: "The function comment.",
				Computed:    true,
			},
			"definitions": schema.ListNestedAttribute{
				Description: "The definitions of the function, including their implementations.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"parameters": schema.ListNestedAttribute{
							Description: "The parameters of the definition.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Description: "The name of the parameter.",
										Computed:    true,
									},
									"data_type": schema.StringAttribute{
										Description: "The Gravitino data type of the parameter.",
										Computed:    true,
									},
									"comment": schema.StringAttribute{
										Description: "The comment of the parameter.",
										Computed:    true,
									},
									"default_value": schema.StringAttribute{
										Description: "The default value expression of the parameter.",
										Computed:    true,
									},
								},
							},
						},
						"return_type": schema.StringAttribute{
							Description: "The return type of the definition (SCALAR and AGGREGATE functions).",
							Computed:    true,
						},
						"return_columns": schema.ListNestedAttribute{
							Description: "The return columns of the definition (TABLE functions).",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Description: "The name of the return column.",
										Computed:    true,
									},
									"data_type": schema.StringAttribute{
										Description: "The Gravitino data type of the return column.",
										Computed:    true,
									},
									"comment": schema.StringAttribute{
										Description: "The comment of the return column.",
										Computed:    true,
									},
								},
							},
						},
						"impls": schema.ListNestedAttribute{
							Description: "The implementations of the definition.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"language": schema.StringAttribute{
										Description: "The implementation language (SQL, JAVA or PYTHON).",
										Computed:    true,
									},
									"runtime": schema.StringAttribute{
										Description: "The runtime of the implementation (SPARK or TRINO).",
										Computed:    true,
									},
									"sql": schema.StringAttribute{
										Description: "The SQL expression of a SQL implementation.",
										Computed:    true,
									},
									"class_name": schema.StringAttribute{
										Description: "The class name of a JAVA implementation.",
										Computed:    true,
									},
									"handler": schema.StringAttribute{
										Description: "The handler of a PYTHON implementation.",
										Computed:    true,
									},
									"code_block": schema.StringAttribute{
										Description: "The code block of a PYTHON implementation.",
										Computed:    true,
									},
									"resources": schema.SingleNestedAttribute{
										Description: "External resources required by the implementation.",
										Computed:    true,
										Attributes: map[string]schema.Attribute{
											"jars": schema.ListAttribute{
												Description: "JAR file URIs.",
												Computed:    true,
												ElementType: types.StringType,
											},
											"files": schema.ListAttribute{
												Description: "File URIs.",
												Computed:    true,
												ElementType: types.StringType,
											},
											"archives": schema.ListAttribute{
												Description: "Archive URIs.",
												Computed:    true,
												ElementType: types.StringType,
											},
										},
									},
									"properties": schema.MapAttribute{
										Description: "Additional properties of the implementation.",
										Computed:    true,
										ElementType: types.StringType,
									},
								},
							},
						},
					},
				},
			},
			"audit": schema.ObjectAttribute{
				Description:    "Audit information for the function.",
				Computed:       true,
				AttributeTypes: dsAuditAttrTypes,
			},
		},
	}
}

func (d *FunctionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *FunctionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config FunctionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	functionResponse, err := d.client.GetFunction(ctx, config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString(), config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading function", config.Name.ValueString(), err)...)
		return
	}

	function := &functionResponse.Function
	config.FunctionType = types.StringValue(models.NormalizeFunctionType(function.FunctionType))
	config.Deterministic = types.BoolValue(function.Deterministic)
	config.Comment = types.StringNull()
	if function.Comment != "" {
		config.Comment = types.StringValue(function.Comment)
	}

	definitions, diags := models.FunctionDefinitionsToTF(ctx, function.Definitions)
	resp.Diagnostics.Append(diags...)
	config.Definitions = definitions

	audit, diags := dsAuditToObject(function.Audit)
	resp.Diagnostics.Append(diags...)
	config.Audit = audit
	if resp.Diagnostics.HasError() {
		return
	}

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
