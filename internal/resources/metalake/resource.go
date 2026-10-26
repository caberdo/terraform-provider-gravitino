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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = (*MetalakeResource)(nil)
var _ resource.ResourceWithImportState = (*MetalakeResource)(nil)

type MetalakeResource struct {
	client *client.Client
}

type MetalakeResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Comment      types.String `tfsdk:"comment"`
	Properties   types.Map    `tfsdk:"properties"`
	InUse        types.Bool   `tfsdk:"in_use"`
	ForceDestroy types.Bool   `tfsdk:"force_destroy"`
	Audit        types.Object `tfsdk:"audit"`
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
					models.CompoundID("name"),
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
			"in_use": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Description: "Whether the metalake is in use. Gravitino creates every metalake in use, so " +
					"an omitted value becomes true; set it to false to disable the metalake without dropping " +
					"its metadata. Changing it sends PATCH /metalakes/{metalake} with {\"inUse\": ...}. " +
					"Gravitino has no top-level inUse field in its response: the same state is exposed only " +
					"as the reserved 'in-use' property, which this resource filters out as server-managed, " +
					"so a change made outside Terraform is not detected. State keeps the value the provider " +
					"last wrote, and an imported metalake reports true (the creation default) until it is " +
					"written. The gravitino_metalake data source exposes the server value in properties.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"force_destroy": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				Description: "Whether destroying this metalake also drops every catalog, schema, table, view, " +
					"fileset, topic and model underneath it, including objects Terraform does not manage. " +
					"Defaults to false: the provider disables the metalake before dropping it (removing no " +
					"metadata) and Gravitino then refuses to drop one that still contains catalogs, in which " +
					"case the provider reports a diagnostic naming this flag and restores the previous " +
					"in-use state. Set it to true to reproduce the forceful drop.",
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
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
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
	plan.InUse = inUseOnCreate(plan.InUse)
	if plan.ForceDestroy.IsUnknown() || plan.ForceDestroy.IsNull() {
		// force_destroy is never sent to the API, but it must be known before it
		// is written to state: an unknown computed expression would otherwise
		// fail the apply with "Provider produced invalid result object".
		plan.ForceDestroy = types.BoolValue(false)
	}

	// Gravitino creates the metalake in use. Only a configured false needs a
	// second call; this is the only supported way to disable it without
	// dropping its metadata.
	if !plan.InUse.ValueBool() {
		if _, err := r.client.SetMetalakeInUse(ctx, plan.Name.ValueString(), false); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("disabling metalake", plan.Name.ValueString(), err)...)
			return
		}
	}

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

	// in_use is Optional+Computed and written only through PATCH. Resolve an
	// unknown plan value (a fresh import) from state so the applied value is
	// never left unknown.
	if plan.InUse.IsUnknown() {
		plan.InUse = state.InUse
	}
	if plan.ForceDestroy.IsUnknown() || plan.ForceDestroy.IsNull() {
		if state.ForceDestroy.IsUnknown() || state.ForceDestroy.IsNull() {
			plan.ForceDestroy = types.BoolValue(false)
		} else {
			plan.ForceDestroy = state.ForceDestroy
		}
	}
	inUseChanged := !plan.InUse.IsNull() && !plan.InUse.Equal(state.InUse)

	// Enable before the metadata updates and disable after them: Gravitino
	// refuses to operate on a disabled metalake.
	if inUseChanged && plan.InUse.ValueBool() {
		if _, err := r.client.SetMetalakeInUse(ctx, name, true); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("enabling metalake", name, err)...)
			return
		}
	}

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

	// A disabled metalake only allows list/load/drop/enable, so the PUT below
	// would be rejected. Enable it for the duration of the metadata update and
	// restore the disabled state afterwards when the plan keeps it disabled.
	temporarilyEnabled := !inUseChanged && !plan.InUse.ValueBool() && len(updates) > 0
	if temporarilyEnabled {
		if _, err := r.client.SetMetalakeInUse(ctx, name, true); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("enabling metalake", name, err)...)
			return
		}
	}

	// Never send an empty update list: it is not a valid payload and would
	// replace the planned values with an empty response.
	if len(updates) > 0 {
		result, err := r.client.UpdateMetalake(ctx, name, updates)
		if err != nil {
			if temporarilyEnabled {
				if _, restoreErr := r.client.SetMetalakeInUse(ctx, name, false); restoreErr != nil {
					tflog.Warn(ctx, "Failed to re-disable metalake after a failed update", map[string]interface{}{
						"name":  name,
						"error": restoreErr.Error(),
					})
				}
			}
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

	if (inUseChanged && !plan.InUse.ValueBool()) || temporarilyEnabled {
		finalName := plan.Name.ValueString()
		if _, err := r.client.SetMetalakeInUse(ctx, finalName, false); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("disabling metalake", finalName, err)...)
			return
		}
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

	name := state.Name.ValueString()
	forceDestroy := state.ForceDestroy.ValueBool()

	// Gravitino refuses to drop an in-use metalake even when it is empty.
	// Disabling it removes no metadata and is what the server's own message
	// asks for; force_destroy=false still refuses a metalake with catalogs.
	disabled := false
	if !forceDestroy {
		if _, err := r.client.SetMetalakeInUse(ctx, name, false); err != nil {
			if client.IsNotFoundError(err) {
				tflog.Debug(ctx, "Metalake already deleted", map[string]interface{}{"name": name})
				return
			}
			resp.Diagnostics.Append(client.NewResourceError("disabling metalake before deletion", name, err)...)
			return
		}
		disabled = true
	}

	_, err := r.client.DropMetalake(ctx, name, forceDestroy)
	if err != nil {
		if client.IsNotFoundError(err) {
			// Already gone: deleting twice is not an error.
			tflog.Debug(ctx, "Metalake already deleted", map[string]interface{}{"name": name})
			return
		}
		// A refused destroy must not leave a live metalake out of service:
		// restore the state it had before this attempt.
		if disabled && state.InUse.ValueBool() {
			if _, restoreErr := r.client.SetMetalakeInUse(ctx, name, true); restoreErr != nil {
				tflog.Warn(ctx, "Failed to re-enable metalake after a failed drop", map[string]interface{}{
					"name":  name,
					"error": restoreErr.Error(),
				})
			}
		}
		resp.Diagnostics.Append(client.NewDropError("deleting metalake", name, forceDestroy, err)...)
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

	// in_use cannot be read back. A null value only occurs after an import:
	// adopt Gravitino's creation default so the planned value stays known
	// instead of flipping between null and unknown on every plan.
	if state.InUse.IsNull() {
		state.InUse = types.BoolValue(true)
	}
	// force_destroy has no server-side value either; an imported resource
	// adopts the safe default instead of leaving the attribute null.
	if state.ForceDestroy.IsNull() {
		state.ForceDestroy = types.BoolValue(false)
	}

	auditObj, d := models.AuditToObjectValue(ctx, m.Audit)
	diags.Append(d...)
	state.Audit = auditObj
}

// inUseOnCreate resolves the Optional+Computed in_use value for a new
// metalake: Gravitino creates every metalake in use, so an unconfigured
// (unknown or null) plan becomes true. A configured value is kept.
func inUseOnCreate(planned types.Bool) types.Bool {
	if planned.IsUnknown() || planned.IsNull() {
		return types.BoolValue(true)
	}
	return planned
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
