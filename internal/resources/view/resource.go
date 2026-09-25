package view

import (
	"context"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &ViewResource{}
var _ resource.ResourceWithImportState = &ViewResource{}
var _ resource.ResourceWithConfigure = &ViewResource{}
var _ resource.ResourceWithModifyPlan = &ViewResource{}

type ViewResource struct {
	client *client.Client
}

type ViewResourceModel struct {
	ID              types.String                     `tfsdk:"id"`
	Metalake        types.String                     `tfsdk:"metalake"`
	Catalog         types.String                     `tfsdk:"catalog"`
	Schema          types.String                     `tfsdk:"schema"`
	Name            types.String                     `tfsdk:"name"`
	Comment         types.String                     `tfsdk:"comment"`
	Columns         []models.ColumnTFSDK             `tfsdk:"column"`
	Representations []models.ViewRepresentationTFSDK `tfsdk:"representation"`
	DefaultCatalog  types.String                     `tfsdk:"default_catalog"`
	DefaultSchema   types.String                     `tfsdk:"default_schema"`
	Properties      types.Map                        `tfsdk:"properties"`
	Audit           types.Object                     `tfsdk:"audit"`
}

func NewViewResource() resource.Resource {
	return &ViewResource{}
}

func (r *ViewResource) SetClient(c *client.Client) {
	r.client = c
}

func (r *ViewResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_view"
}

func (r *ViewResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Gravitino view within a metalake, catalog, and schema.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Compound identifier in the format metalake.catalog.schema.view.",
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
				Description: "The view name. Gravitino renames views in place through the `rename` update.",
				Required:    true,
			},
			"comment": schema.StringAttribute{
				Description: "A comment describing the view. Gravitino v1.3.0 has no view comment update operation, so changing the comment replaces the view.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"column": schema.ListNestedAttribute{
				Description: "The output columns of the view. When omitted Gravitino derives them from the representations. Changing the columns forces a new view.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The column name.",
							Required:    true,
						},
						"type": schema.StringAttribute{
							Description: "The column data type: a Gravitino primitive such as `long` or `varchar(10)`, or a JSON object for a `struct`, `list`, `map`, `union` or `unparsed` type.",
							Required:    true,
							Validators: []validator.String{
								columnTypeValidator{},
							},
						},
						"comment": schema.StringAttribute{
							Description: "The column comment.",
							Optional:    true,
						},
						"nullable": schema.BoolAttribute{
							Description: "Whether the column is nullable.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(true),
						},
						"auto_increment": schema.BoolAttribute{
							Description: "Whether the column is auto increment.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
						"default_value": schema.StringAttribute{
							Description: "The default value of the column, using the data type of the column.",
							Optional:    true,
						},
					},
				},
			},
			"representation": schema.ListNestedAttribute{
				Description: "The representations of the view body, keyed by dialect. At least one is required. Changing them forces a new view.",
				Required:    true,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Description: "The representation type discriminator.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.OneOf("sql"),
							},
						},
						"dialect": schema.StringAttribute{
							Description: "The SQL dialect of this representation, e.g. `trino`.",
							Required:    true,
						},
						"sql": schema.StringAttribute{
							Description: "The SQL text of the view.",
							Required:    true,
						},
					},
				},
			},
			"default_catalog": schema.StringAttribute{
				Description: "The default catalog used to resolve unqualified identifiers in the view representations. Changing it replaces the view.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"default_schema": schema.StringAttribute{
				Description: "The default schema used to resolve unqualified identifiers in the view representations. Changing it replaces the view.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"properties": schema.MapAttribute{
				Description: "Key-value properties for the view.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"audit": schema.ObjectAttribute{
				Description:    "Audit information for the view.",
				Computed:       true,
				AttributeTypes: models.AuditAttrTypes,
			},
		},
	}
}

func (r *ViewResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ModifyPlan keeps the planned compound ID in sync with the planned name.
// Terraform compares the plan with the applied result, so a view that is
// renamed in place must already plan the new ID: the id attribute carries
// stringplanmodifier.UseStateForUnknown and would otherwise keep the old value.
func (r *ViewResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Destroying the resource has no plan to adjust.
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan ViewResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, value := range []types.String{plan.Metalake, plan.Catalog, plan.Schema, plan.Name} {
		if value.IsNull() || value.IsUnknown() {
			// Not known yet; Create/Update computes the ID after apply.
			return
		}
	}

	id := fmt.Sprintf("%s.%s.%s.%s",
		plan.Metalake.ValueString(),
		plan.Catalog.ValueString(),
		plan.Schema.ValueString(),
		plan.Name.ValueString(),
	)

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("id"), id)...)

	tflog.Debug(ctx, "Planned view id", map[string]interface{}{"id": id})
}

func (r *ViewResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ViewResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating view", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})

	// Views use the shared tables.yaml#/Column schema, so the table family's
	// column conversion applies unchanged.
	columns, columnDiags := models.TableColumnsFromModel(plan.Columns)
	resp.Diagnostics.Append(columnDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := &models.ViewCreateRequest{
		Name:            plan.Name.ValueString(),
		Comment:         plan.Comment.ValueString(),
		Columns:         columns,
		Representations: models.ViewRepresentationsToAPI(plan.Representations),
		DefaultCatalog:  plan.DefaultCatalog.ValueString(),
		DefaultSchema:   plan.DefaultSchema.ValueString(),
		Properties:      propertiesFromPlan(ctx, plan.Properties, &resp.Diagnostics),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	viewResp, err := r.client.CreateView(ctx, plan.Metalake.ValueString(), plan.Catalog.ValueString(), plan.Schema.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating view", plan.Name.ValueString(), err)...)
		return
	}

	r.readViewToState(ctx, viewResp, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Created view", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})
}

func (r *ViewResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ViewResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading view", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	viewResp, err := r.client.GetView(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading view", state.Name.ValueString(), err)...)
		return
	}

	r.readViewToState(ctx, viewResp, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	tflog.Debug(ctx, "Read view", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})
}

func (r *ViewResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ViewResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating view", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	var updates []interface{}

	if !plan.Name.IsUnknown() && !plan.Name.Equal(state.Name) {
		updates = append(updates, models.NewRenameViewRequest(plan.Name.ValueString()))
	}

	// A not yet known property map is resolved by Terraform before the next
	// apply; diffing it now would send bogus removeProperty updates.
	if !plan.Properties.IsUnknown() && !state.Properties.IsUnknown() && !plan.Properties.Equal(state.Properties) {
		oldProps := mapFromState(ctx, state.Properties, &resp.Diagnostics)
		newProps := mapFromState(ctx, plan.Properties, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}

		for k := range oldProps {
			if _, exists := newProps[k]; !exists {
				updates = append(updates, models.NewRemoveViewPropertyRequest(k))
			}
		}
		for k, v := range newProps {
			if oldVal, exists := oldProps[k]; !exists || oldVal != v {
				updates = append(updates, models.NewSetViewPropertyRequest(k, v))
			}
		}
	}

	if len(updates) > 0 {
		viewResp, err := r.client.UpdateView(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString(), updates)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating view", state.Name.ValueString(), err)...)
			return
		}
		r.readViewToState(ctx, viewResp, &plan, &resp.Diagnostics)
	} else {
		viewResp, err := r.client.GetView(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("reading view after update", state.Name.ValueString(), err)...)
			return
		}
		r.readViewToState(ctx, viewResp, &plan, &resp.Diagnostics)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Updated view", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})
}

func (r *ViewResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ViewResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting view", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	_, err := r.client.DropView(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Debug(ctx, "View already deleted", map[string]interface{}{"name": state.Name.ValueString()})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting view", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted view", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})
}

func (r *ViewResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ".", 4)
	if len(parts) != 4 {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected format: metalake.catalog.schema.view, got: %s", req.ID))
		return
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected format: metalake.catalog.schema.view, got: %s", req.ID))
			return
		}
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schema"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *ViewResource) readViewToState(ctx context.Context, viewResp *models.ViewResponse, m *ViewResourceModel, diags *diag.Diagnostics) {
	view := viewResp.View

	m.Name = types.StringValue(view.Name)
	m.Comment = stringFromResponse(view.Comment, m.Comment)
	m.Columns = models.TableColumnsToModel(ctx, view.Columns, diags)
	m.Representations = models.ViewRepresentationsToState(view.Representations)
	m.DefaultCatalog = stringFromResponse(view.DefaultCatalog, m.DefaultCatalog)
	m.DefaultSchema = stringFromResponse(view.DefaultSchema, m.DefaultSchema)
	m.Properties = mapFromResponse(ctx, view.Properties, m.Properties, diags)
	m.ID = types.StringValue(fmt.Sprintf("%s.%s.%s.%s",
		m.Metalake.ValueString(),
		m.Catalog.ValueString(),
		m.Schema.ValueString(),
		view.Name,
	))

	auditObj, d := models.AuditToObjectValue(ctx, view.Audit)
	diags.Append(d...)
	m.Audit = auditObj
}

// stringFromResponse maps an optional string response field to a framework
// value, preserving an explicitly configured empty string so the applied value
// stays equal to the planned value.
func stringFromResponse(value string, planned types.String) types.String {
	if value != "" {
		return types.StringValue(value)
	}
	if !planned.IsNull() && !planned.IsUnknown() {
		return types.StringValue("")
	}
	return types.StringNull()
}

func mapFromResponse(ctx context.Context, props map[string]string, planned types.Map, diags *diag.Diagnostics) types.Map {
	if len(props) > 0 {
		v, d := types.MapValueFrom(ctx, types.StringType, props)
		diags.Append(d...)
		return v
	}
	if !planned.IsNull() && !planned.IsUnknown() {
		v, d := types.MapValueFrom(ctx, types.StringType, map[string]string{})
		diags.Append(d...)
		return v
	}
	return types.MapNull(types.StringType)
}

// propertiesFromPlan converts the planned properties into the map sent to the
// API. Unknown (not yet computed) values are omitted.
func propertiesFromPlan(ctx context.Context, planned types.Map, diags *diag.Diagnostics) map[string]string {
	props := make(map[string]string)
	if planned.IsNull() || planned.IsUnknown() {
		return props
	}
	diags.Append(planned.ElementsAs(ctx, &props, false)...)
	return props
}

// mapFromState converts a framework map into a plain Go map, tolerating null
// and unknown values.
func mapFromState(ctx context.Context, value types.Map, diags *diag.Diagnostics) map[string]string {
	props := make(map[string]string)
	if value.IsNull() || value.IsUnknown() {
		return props
	}
	diags.Append(value.ElementsAs(ctx, &props, false)...)
	return props
}

// columnTypeValidator validates the Terraform representation of a column type
// against datatype.yaml#/DataType: either a primitive type name (optionally
// parameterised) or a JSON object describing a structured type.
type columnTypeValidator struct{}

func (v columnTypeValidator) Description(_ context.Context) string {
	return "must be a Gravitino primitive type such as \"long\" or \"varchar(10)\", or a JSON object describing a struct, list, map, union or unparsed type"
}

func (v columnTypeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v columnTypeValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := models.ParseDataType(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid column type", err.Error())
	}
}
