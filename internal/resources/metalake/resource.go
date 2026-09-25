package metalake

import (
	"context"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = (*MetalakeResource)(nil)
var _ resource.ResourceWithImportState = (*MetalakeResource)(nil)
var _ resource.ResourceWithModifyPlan = (*MetalakeResource)(nil)

type MetalakeResource struct {
	client *client.Client
}

type MetalakeResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
}

// reservedProperties are managed by Gravitino through dedicated endpoints
// (e.g. "in-use" via PATCH /metalakes/{name}) and must never be sent as
// regular setProperty/removeProperty updates or surfaced as drift.
var reservedProperties = map[string]bool{
	"in-use":               true,
	"gravitino.identifier": true,
}

func filterReservedProperties(props map[string]string) map[string]string {
	filtered := make(map[string]string)
	for k, v := range props {
		if !reservedProperties[k] {
			filtered[k] = v
		}
	}
	return filtered
}

func NewMetalakeResource() resource.Resource {
	return &MetalakeResource{}
}

func (r *MetalakeResource) SetClient(c *client.Client) {
	r.client = c
}

func (r *MetalakeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_metalake"
}

func (r *MetalakeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"comment": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"properties": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "A map of key-value properties. The reserved 'in-use' and 'gravitino.identifier' properties are managed by Gravitino and cannot be configured here.",
				Validators: []validator.Map{
					mapvalidator.KeysAre(stringvalidator.NoneOf("in-use", "gravitino.identifier")),
				},
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: models.AuditAttrTypes,
				Description:    "Audit information for the metalake.",
				// No UseStateForUnknown: the server rewrites lastModifier and
				// lastModifiedTime on every update, so the plan must stay
				// unknown here, otherwise the applied value would differ from
				// the planned value. Every code path writes a known audit.
			},
		},
	}
}

func (r *MetalakeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cli, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid provider data",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue.", req.ProviderData),
		)
		return
	}

	r.client = cli
}

// ModifyPlan marks the id as unknown when the metalake is renamed in place: the
// identifier is the name itself, so pinning the planned id to the prior state
// would make the applied id differ from the planned one ("Provider produced
// inconsistent result after apply").
func (r *MetalakeResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing to pin down while creating or destroying.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var plan, state MetalakeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Name.Equal(state.Name) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("id"), types.StringUnknown())...)
	}
}

func (r *MetalakeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MetalakeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating metalake", map[string]interface{}{"name": plan.Name.ValueString()})

	props := mapToProperties(ctx, plan.Properties, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	props = filterReservedProperties(props)

	createReq := &models.MetalakeCreateRequest{
		Name:       plan.Name.ValueString(),
		Comment:    plan.Comment.ValueString(),
		Properties: props,
	}

	result, err := r.client.CreateMetalake(ctx, createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating metalake", plan.Name.ValueString(), err)...)
		return
	}

	plan.ID = plan.Name

	metalakeToState(ctx, &result.Metalake, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created metalake", map[string]interface{}{"name": plan.Name.ValueString()})
}

func (r *MetalakeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MetalakeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading metalake", map[string]interface{}{"name": state.Name.ValueString()})

	result, err := r.client.GetMetalake(ctx, state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading metalake", state.Name.ValueString(), err)...)
		return
	}

	state.ID = state.Name

	metalakeToState(ctx, &result.Metalake, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *MetalakeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MetalakeResourceModel
	var state MetalakeResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating metalake", map[string]interface{}{"name": state.Name.ValueString()})

	// The metalake is renamed with a rename update request; the request itself
	// must target the current name.
	name := state.Name.ValueString()

	var updates []interface{}

	if !plan.Name.Equal(state.Name) {
		updates = append(updates, models.NewRenameMetalakeRequest(plan.Name.ValueString()))
	}

	if !plan.Comment.IsUnknown() && !plan.Comment.Equal(state.Comment) {
		newComment := plan.Comment.ValueString()
		updates = append(updates, models.NewUpdateMetalakeCommentRequest(newComment))
	}

	if !plan.Properties.IsUnknown() {
		oldProps := mapToProperties(ctx, state.Properties, &resp.Diagnostics)
		newProps := mapToProperties(ctx, plan.Properties, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}

		updates = append(updates, r.propertyUpdates(oldProps, newProps)...)
	}

	// Never send an empty update list: it is not a valid payload and would
	// replace the planned values with an empty response.
	if len(updates) > 0 {
		result, err := r.client.UpdateMetalake(ctx, name, updates)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating metalake", name, err)...)
			return
		}

		metalakeToState(ctx, &result.Metalake, &plan, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		// Nothing changed: keep the planned values. The audit is server-managed
		// and therefore unknown in the plan, so it must be filled with a known
		// value explicitly.
		plan.Audit = state.Audit
	}

	plan.ID = plan.Name

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated metalake", map[string]interface{}{"name": plan.Name.ValueString()})
}

func (r *MetalakeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MetalakeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting metalake", map[string]interface{}{"name": state.Name.ValueString()})

	_, err := r.client.DropMetalake(ctx, state.Name.ValueString(), true)
	if err != nil {
		if client.IsNotFoundError(err) {
			// Already gone: deleting twice is not an error.
			tflog.Debug(ctx, "Metalake already deleted", map[string]interface{}{"name": state.Name.ValueString()})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting metalake", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted metalake", map[string]interface{}{"name": state.Name.ValueString()})
}

func (r *MetalakeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// A metalake is identified by its bare name: it has no dot-separated
	// hierarchy, so a dotted ID is a user error (most likely a nested resource
	// ID) or an empty one.
	if req.ID == "" || strings.Contains(req.ID, ".") {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected the metalake name without dots, got: %s", req.ID),
		)
		return
	}

	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func mapToProperties(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	result := make(map[string]string, len(m.Elements()))
	d := m.ElementsAs(ctx, &result, false)
	if d.HasError() {
		*diags = append(*diags, d...)
		return nil
	}
	return result
}

func metalakeToState(ctx context.Context, m *models.Metalake, state *MetalakeResourceModel, diags *diag.Diagnostics) {
	state.Name = types.StringValue(m.Name)
	// Gravitino omits the comment when it is empty. Leaving the value untouched
	// would keep an unknown plan value for an unconfigured comment (Optional +
	// Computed without prior state), which Terraform rejects with "Provider
	// returned invalid result object after apply".
	state.Comment = commentFromServer(m.Comment, state.Comment)

	state.Properties = mergeProperties(ctx, state.Properties, m.Properties, diags)

	auditObj, d := models.AuditToObjectValue(ctx, m.Audit)
	diags.Append(d...)
	state.Audit = auditObj
}

// commentFromServer resolves a comment returned by the API into a known value:
// a non-empty server value wins, otherwise a known planned value is kept (the
// server omitted the comment) and an unknown planned value becomes null.
func commentFromServer(serverValue string, planned types.String) types.String {
	if serverValue != "" {
		return types.StringValue(serverValue)
	}
	if !planned.IsUnknown() && !planned.IsNull() {
		return planned
	}
	return types.StringNull()
}

// mergeProperties stores exactly the properties of the plan: server values are
// used for the keys the plan contains, but server-only keys (for example
// "in-use", which Gravitino returns for metalakes and catalogs) are never added.
//
// properties is Optional+Computed: the applied value must equal the planned
// value, otherwise Terraform fails the apply with "Provider produced
// inconsistent result after apply". A configured empty map stays an empty map
// (only a null plan stays null). Keys the server does not echo keep their
// planned value so that a missing echo cannot silently drop configuration.
//
// A null plan means "no configured properties" (for example while importing):
// then the server's properties are adopted, so that an import reconstructs the
// resource instead of dropping everything, while Gravitino-managed keys stay out
// of state.
func mergeProperties(ctx context.Context, plan types.Map, serverProps map[string]string, diags *diag.Diagnostics) types.Map {
	if plan.IsUnknown() {
		return types.MapNull(types.StringType)
	}

	if plan.IsNull() {
		// No configured properties (for example while importing): adopt the
		// server's properties so that an import reconstructs the resource,
		// minus the keys Gravitino manages itself.
		serverOnly := filterReservedProperties(serverProps)
		if len(serverOnly) == 0 {
			return types.MapNull(types.StringType)
		}
		return mapFromStringMap(ctx, serverOnly, diags)
	}

	planned := make(map[string]string)
	diags.Append(plan.ElementsAs(ctx, &planned, false)...)
	if diags.HasError() {
		return types.MapNull(types.StringType)
	}

	merged := make(map[string]string, len(planned))
	for k, v := range planned {
		if serverValue, ok := serverProps[k]; ok {
			merged[k] = serverValue
			continue
		}
		merged[k] = v
	}

	return mapFromStringMap(ctx, merged, diags)
}

func mapFromStringMap(ctx context.Context, values map[string]string, diags *diag.Diagnostics) types.Map {
	props, d := types.MapValueFrom(ctx, types.StringType, values)
	diags.Append(d...)
	if diags.HasError() {
		return types.MapNull(types.StringType)
	}
	return props
}

func (r *MetalakeResource) propertyUpdates(oldProps, newProps map[string]string) []interface{} {
	var updates []interface{}
	for key, oldVal := range oldProps {
		if reservedProperties[key] {
			continue
		}
		newVal, exists := newProps[key]
		if !exists {
			updates = append(updates, models.NewRemoveMetalakePropertyRequest(key))
		} else if oldVal != newVal {
			updates = append(updates, models.NewSetMetalakePropertyRequest(key, newVal))
		}
	}

	for key, newVal := range newProps {
		if reservedProperties[key] {
			continue
		}
		if _, exists := oldProps[key]; !exists {
			updates = append(updates, models.NewSetMetalakePropertyRequest(key, newVal))
		}
	}
	return updates
}
