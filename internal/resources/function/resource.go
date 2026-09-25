package function

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &FunctionResource{}
var _ resource.ResourceWithImportState = &FunctionResource{}
var _ resource.ResourceWithConfigure = &FunctionResource{}

var auditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

const dataTypeDescription = "The Gravitino data type. Primitive types use the name Gravitino spells " +
	"(`integer`, `long`, `float`, `double`, `decimal(10,2)`, `date`, `timestamp(3)`, `string`, " +
	"`varchar(10)`, `binary`, ...). Complex types use the JSON document of the API, produced with " +
	"jsonencode(), for example jsonencode({type = \"list\", elementType = \"integer\"}) or " +
	"jsonencode({type = \"struct\", fields = [{name = \"id\", type = \"integer\"}]})."

type FunctionResource struct {
	client *client.Client
}

type FunctionResourceModel struct {
	ID            types.String `tfsdk:"id"`
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

func New() resource.Resource {
	return &FunctionResource{}
}

// SetClient injects the API client; used by tests.
func (r *FunctionResource) SetClient(c *client.Client) {
	r.client = c
}

func (r *FunctionResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_function"
}

func (r *FunctionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Gravitino function within a metalake, catalog and schema.\n\n" +
			"A function is registered with one or more definitions (the parameters plus the return type, " +
			"or the return columns for table functions) and each definition holds one implementation per " +
			"runtime. Gravitino can update the comment, add and remove definitions and add, update and " +
			"remove implementations; every other change replaces the function.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The compound identifier in the format metalake.catalog.schema.function.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"metalake": schema.StringAttribute{
				Description: "The metalake name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schema": schema.StringAttribute{
				Description: "The schema name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The function name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"function_type": schema.StringAttribute{
				Description: "The type of the function.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(models.AllFunctionTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"deterministic": schema.BoolAttribute{
				Description: "Whether the function is deterministic. Defaults to false. Gravitino cannot change " +
					"this after registration, so a function whose deterministic flag differs from the configuration " +
					"is replaced (declare it explicitly for imported functions).",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Description: "A comment describing the function. Gravitino updates the comment in place; an " +
					"empty comment clears it. Clearing an existing comment replaces the function, because " +
					"the Gravitino alter endpoint rejects a null comment.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					commentModifier(),
				},
			},
			"definitions": schema.ListNestedAttribute{
				Description: "The definitions of the function (at least one). Gravitino identifies a " +
					"definition by its parameters: changing the parameters, the return type or the return " +
					"columns removes the old definition and adds the new one. Implementations are added, " +
					"updated and removed per runtime.",
				Required: true,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"parameters": schema.ListNestedAttribute{
							Description: "The parameters of the definition (SCALAR, AGGREGATE and TABLE functions).",
							Optional:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Description: "The name of the parameter.",
										Required:    true,
									},
									"data_type": schema.StringAttribute{
										Description: dataTypeDescription,
										Required:    true,
									},
									"comment": schema.StringAttribute{
										Description: "The comment of the parameter.",
										Optional:    true,
									},
									"default_value": schema.StringAttribute{
										Description: "The default value expression of the parameter, as the JSON " +
											"document of a Gravitino function argument, for example " +
											"jsonencode({type = \"literal\", dataType = \"integer\", value = \"1\"}).",
										Optional: true,
									},
								},
							},
						},
						"return_type": schema.StringAttribute{
							Description: "The return type of the definition, for SCALAR and AGGREGATE functions. " +
								dataTypeDescription,
							Optional: true,
						},
						"return_columns": schema.ListNestedAttribute{
							Description: "The return columns of the definition, for TABLE functions.",
							Optional:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Description: "The name of the return column.",
										Required:    true,
									},
									"data_type": schema.StringAttribute{
										Description: dataTypeDescription,
										Required:    true,
									},
									"comment": schema.StringAttribute{
										Description: "The comment of the return column.",
										Optional:    true,
									},
								},
							},
						},
						"impls": schema.ListNestedAttribute{
							Description: "The implementations of the definition. Gravitino stores one " +
								"implementation per runtime and discriminates them by language.",
							Optional: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"language": schema.StringAttribute{
										Description: "The implementation language (the API discriminator).",
										Required:    true,
										Validators: []validator.String{
											stringvalidator.OneOf(models.AllFunctionLanguages...),
										},
									},
									"runtime": schema.StringAttribute{
										Description: "The runtime of the implementation. Implementations are " +
											"identified by their runtime.",
										Required: true,
										Validators: []validator.String{
											stringvalidator.OneOf(models.AllFunctionRuntimes...),
										},
									},
									"sql": schema.StringAttribute{
										Description: "The SQL expression. Required for a SQL implementation.",
										Optional:    true,
									},
									"class_name": schema.StringAttribute{
										Description: "The fully qualified class name. Required for a JAVA implementation.",
										Optional:    true,
									},
									"handler": schema.StringAttribute{
										Description: "The name of the Python handler function (PYTHON implementations).",
										Optional:    true,
									},
									"code_block": schema.StringAttribute{
										Description: "The Python code block (PYTHON implementations).",
										Optional:    true,
									},
									"resources": schema.SingleNestedAttribute{
										Description: "External resources required by the implementation.",
										Optional:    true,
										Attributes: map[string]schema.Attribute{
											"jars": schema.ListAttribute{
												Description: "JAR file URIs.",
												Optional:    true,
												ElementType: types.StringType,
											},
											"files": schema.ListAttribute{
												Description: "File URIs.",
												Optional:    true,
												ElementType: types.StringType,
											},
											"archives": schema.ListAttribute{
												Description: "Archive URIs.",
												Optional:    true,
												ElementType: types.StringType,
											},
										},
									},
									"properties": schema.MapAttribute{
										Description: "Additional properties of the implementation.",
										Optional:    true,
										ElementType: types.StringType,
									},
								},
							},
						},
					},
				},
			},
			"audit": schema.ObjectAttribute{
				Description: "Audit information for the function. Gravitino rewrites it on every " +
					"alter (lastModifier/lastModifiedTime), so the upstream value is always read back " +
					"and the attribute is never planned from state.",
				Computed:       true,
				AttributeTypes: auditAttrTypes,
			},
		},
	}
}

func (r *FunctionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid provider data",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue.", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *FunctionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FunctionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Registering function", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"catalog":  plan.Catalog.ValueString(),
		"schema":   plan.Schema.ValueString(),
		"name":     plan.Name.ValueString(),
	})

	definitions, d := models.FunctionDefinitionsFromTF(ctx, plan.Definitions)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The registered definitions are also the reference for the values written to
	// state, so the plan is applied verbatim for everything the server echoes.
	registerRequest := &models.FunctionRegisterRequest{
		Name:          plan.Name.ValueString(),
		FunctionType:  plan.FunctionType.ValueString(),
		Deterministic: plan.Deterministic.ValueBool(),
		Comment:       plan.Comment.ValueString(),
		Definitions:   definitions,
	}

	functionResponse, err := r.client.RegisterFunction(ctx, plan.Metalake.ValueString(), plan.Catalog.ValueString(), plan.Schema.ValueString(), registerRequest)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("registering function", plan.Name.ValueString(), err)...)
		return
	}

	applyFunctionToModel(ctx, &functionResponse.Function, &plan, definitions, false, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Registered function", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"catalog":  plan.Catalog.ValueString(),
		"schema":   plan.Schema.ValueString(),
		"name":     plan.Name.ValueString(),
	})
}

func (r *FunctionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FunctionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading function", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})

	functionResponse, err := r.client.GetFunction(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Debug(ctx, "Function no longer exists, removing from state", map[string]interface{}{
				"metalake": state.Metalake.ValueString(),
				"catalog":  state.Catalog.ValueString(),
				"schema":   state.Schema.ValueString(),
				"name":     state.Name.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading function", state.Name.ValueString(), err)...)
		return
	}

	// The server is the source of truth for a refresh; the state of this run keeps
	// the spelling of the data type documents it already holds, so an equivalent but
	// reformatted answer (Gravitino re-encodes data type documents) does not produce
	// a whitespace only diff on every plan.
	refreshed, d := models.FunctionDefinitionsFromTF(ctx, state.Definitions)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	applyFunctionToModel(ctx, &functionResponse.Function, &state, refreshed, true, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	tflog.Debug(ctx, "Read function", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})
}

func (r *FunctionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state FunctionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating function", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})

	updates, d := buildFunctionUpdates(ctx, plan, state)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	var function *models.Function
	if len(updates) > 0 {
		functionResponse, err := r.client.UpdateFunction(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString(), updates)
		if err != nil {
			if client.IsNotFoundError(err) {
				tflog.Debug(ctx, "Function no longer exists, removing from state", map[string]interface{}{"name": state.Name.ValueString()})
				resp.State.RemoveResource(ctx)
				return
			}
			resp.Diagnostics.Append(client.NewResourceError("updating function", state.Name.ValueString(), err)...)
			return
		}
		function = &functionResponse.Function
	} else {
		functionResponse, err := r.client.GetFunction(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
		if err != nil {
			if client.IsNotFoundError(err) {
				tflog.Debug(ctx, "Function no longer exists, removing from state", map[string]interface{}{"name": state.Name.ValueString()})
				resp.State.RemoveResource(ctx)
				return
			}
			resp.Diagnostics.Append(client.NewResourceError("reading function after update", state.Name.ValueString(), err)...)
			return
		}
		function = &functionResponse.Function
	}

	// The plan is the reference for the definitions of this apply.
	planDefinitions, d := models.FunctionDefinitionsFromTF(ctx, plan.Definitions)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	applyFunctionToModel(ctx, function, &plan, planDefinitions, false, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Updated function", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"catalog":  plan.Catalog.ValueString(),
		"schema":   plan.Schema.ValueString(),
		"name":     plan.Name.ValueString(),
	})
}

func (r *FunctionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FunctionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Dropping function", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})

	if _, err := r.client.DropFunction(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString()); err != nil {
		if client.IsNotFoundError(err) {
			tflog.Debug(ctx, "Function already dropped", map[string]interface{}{"name": state.Name.ValueString()})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("dropping function", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Dropped function", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})
}

func (r *FunctionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ".", 4)
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("Expected the format metalake.catalog.schema.function, got %q.", req.ID))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schema"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// applyFunctionToModel copies an API function into the Terraform model.
//
// refreshing selects the role of previous:
//   - refreshing (Read): previous is the state being refreshed, the server decides
//     the content and a definition it echoed keeps the spelling of the state;
//   - after an apply (Create/Update): previous is the plan of the apply, which
//     decides the order, the spelling and the set of definitions.
func applyFunctionToModel(ctx context.Context, function *models.Function, m *FunctionResourceModel, previous []models.FunctionDefinition, refreshing bool, diags *diag.Diagnostics) {
	m.ID = types.StringValue(fmt.Sprintf("%s.%s.%s.%s", m.Metalake.ValueString(), m.Catalog.ValueString(), m.Schema.ValueString(), function.Name))
	m.Name = types.StringValue(function.Name)
	// The API answers "scalar"/"table" while Terraform (and the API request)
	// spells the type in upper case.
	m.FunctionType = types.StringValue(models.NormalizeFunctionType(function.FunctionType))
	m.Deterministic = types.BoolValue(function.Deterministic)

	m.Comment = types.StringNull()
	if function.Comment != "" {
		m.Comment = types.StringValue(function.Comment)
	}

	if refreshing {
		definitions, d := models.FunctionDefinitionsToTFRefreshed(ctx, function.Definitions, previous)
		diags.Append(d...)
		m.Definitions = definitions
	} else {
		definitions, d := models.FunctionDefinitionsToTFApplied(ctx, function.Definitions, previous)
		diags.Append(d...)
		m.Definitions = definitions
	}

	audit, d := auditToObject(function.Audit)
	diags.Append(d...)
	m.Audit = audit
}

func auditToObject(audit *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(auditAttrTypes), nil
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

	return types.ObjectValue(auditAttrTypes, map[string]attr.Value{
		"creator":            creator,
		"create_time":        createTime,
		"last_modifier":      lastModifier,
		"last_modified_time": lastModifiedTime,
	})
}
