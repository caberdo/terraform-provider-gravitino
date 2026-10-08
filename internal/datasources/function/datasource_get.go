package function

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &FunctionDataSource{}
var _ datasource.DataSourceWithConfigure = &FunctionDataSource{}

var dsAuditAttrTypes = models.AuditAttrTypes

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
			"definitions": functionDefinitionsAttribute(),
			"audit": schema.ObjectAttribute{
				Description:    "Audit information for the function.",
				Computed:       true,
				AttributeTypes: dsAuditAttrTypes,
			},
		},
	}
}

func (d *FunctionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
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

func dsAuditToObject(audit *models.Audit) (types.Object, diag.Diagnostics) {
	return models.AuditToObjectValue(context.Background(), audit)
}
