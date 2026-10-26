package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/providerbase"

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

var _ resource.Resource = &CatalogResource{}
var _ resource.ResourceWithImportState = &CatalogResource{}
var _ resource.ResourceWithConfigure = &CatalogResource{}

type CatalogResource struct {
	providerbase.Resource
}

func New() resource.Resource {
	return &CatalogResource{}
}

type CatalogResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Metalake     types.String `tfsdk:"metalake"`
	Name         types.String `tfsdk:"name"`
	Type         types.String `tfsdk:"type"`
	Provider     types.String `tfsdk:"catalog_provider"`
	Comment      types.String `tfsdk:"comment"`
	Properties   types.Map    `tfsdk:"properties"`
	InUse        types.Bool   `tfsdk:"in_use"`
	ForceDestroy types.Bool   `tfsdk:"force_destroy"`
	Audit        types.Object `tfsdk:"audit"`
}

var AuditAttrTypes = models.AuditAttrTypes

func (r *CatalogResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_catalog"
}

func (r *CatalogResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					models.CompoundID("metalake", "name"),
				},
				Description: "Composite identifier in the format 'metalake.catalog_name'.",
			},
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The catalog name.",
			},
			"type": schema.StringAttribute{
				Required:    true,
				Description: "The catalog type. Must be one of: relational, fileset, messaging, model. Changing the type forces a new catalog: the Gravitino API has no update request for it.",
				Validators: []validator.String{
					stringvalidator.OneOf("relational", "fileset", "messaging", "model"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog_provider": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The catalog provider. Must be one of: hive, lakehouse-iceberg, lakehouse-paimon, lakehouse-hudi, jdbc-mysql, jdbc-postgresql, jdbc-doris, jdbc-oceanbase, kafka, fileset, model. Changing the provider forces a new catalog: the Gravitino API has no update request for it.",
				Validators: []validator.String{
					stringvalidator.OneOf("hive", "lakehouse-iceberg", "lakehouse-paimon", "lakehouse-hudi", "jdbc-mysql", "jdbc-postgresql", "jdbc-doris", "jdbc-oceanbase", "kafka", "fileset", "model"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "A comment or description for the catalog. The Gravitino API can only replace an existing comment (updateComment), never remove it.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"in_use": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Description: "Whether the catalog is in use. Gravitino creates every catalog in use, so an " +
					"omitted value becomes true; set it to false to disable the catalog without dropping its " +
					"metadata (a disabled catalog is unavailable to clients and fails its connection tests). " +
					"Changing it sends PATCH /metalakes/{metalake}/catalogs/{catalog} with {\"inUse\": ...}. " +
					"Gravitino has no top-level inUse field in its response: the same state is exposed only " +
					"as the reserved 'in-use' property, which this resource filters out as server-managed, " +
					"so a change made outside Terraform is not detected. State keeps the value the provider " +
					"last wrote, and an imported catalog reports true (the creation default) until it is " +
					"written. The gravitino_catalog data source exposes the server value in properties.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"force_destroy": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				Description: "Whether destroying this catalog also drops every schema, table, view, fileset, " +
					"topic and model underneath it, including objects Terraform does not manage. Defaults " +
					"to false: the provider disables the catalog before dropping it (removing no metadata) " +
					"and Gravitino then refuses to drop one that still contains schemas, in which case the " +
					"provider reports a diagnostic naming this flag and restores the previous in-use state. " +
					"Set it to true to reproduce the forceful drop.",
			},
			"properties": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				ElementType: types.StringType,
				Description: "A map of key-value properties for the catalog. May contain credentials (for example JDBC passwords or S3 keys), therefore it is marked sensitive. The reserved 'in-use' key is managed by Gravitino (PATCH .../catalogs/{name}) and cannot be configured here.",
				Validators: []validator.Map{
					mapvalidator.KeysAre(stringvalidator.NoneOf("in-use")),
				},
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
				Description:    "Audit information for the catalog.",
				// No UseStateForUnknown: the server rewrites lastModifier and
				// lastModifiedTime on every update, so the plan must stay
				// unknown here, otherwise the applied value would differ from
				// the planned value. Every code path writes a known audit.
			},
		},
	}
}

func (r *CatalogResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan CatalogResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating catalog", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})

	createReq := &models.CatalogCreateRequest{
		Name:       plan.Name.ValueString(),
		Type:       plan.Type.ValueString(),
		Provider:   plan.Provider.ValueString(),
		Comment:    plan.Comment.ValueString(),
		Properties: mapFromTF(plan.Properties),
	}

	result, err := r.Client.CreateCatalog(ctx, plan.Metalake.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating catalog", plan.Name.ValueString(), err)...)
		return
	}

	plan.InUse = inUseOnCreate(plan.InUse)
	if plan.ForceDestroy.IsUnknown() || plan.ForceDestroy.IsNull() {
		// force_destroy is never sent to the API, but it must be known before it
		// is written to state: an unknown computed expression would otherwise
		// fail the apply with "Provider produced invalid result object".
		plan.ForceDestroy = types.BoolValue(false)
	}

	// Gravitino creates the catalog in use. Only a configured false needs a
	// second call; this is the only supported way to disable it without
	// dropping its metadata.
	if !plan.InUse.ValueBool() {
		if _, err := r.Client.SetCatalogInUse(ctx, plan.Metalake.ValueString(), plan.Name.ValueString(), false); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("disabling catalog", plan.Name.ValueString(), err)...)
			return
		}
	}

	setStateFromCatalog(ctx, &resp.Diagnostics, plan.Metalake.ValueString(), &result.Catalog, &plan)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created catalog", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})
}

func (r *CatalogResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state CatalogResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading catalog", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	result, err := r.Client.GetCatalog(ctx, state.Metalake.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading catalog", state.Name.ValueString(), err)...)
		return
	}

	setStateFromCatalog(ctx, &resp.Diagnostics, state.Metalake.ValueString(), &result.Catalog, &state)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *CatalogResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state CatalogResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating catalog", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	// The update request targets the *current* name; a rename is sent as an
	// update request whose effect is visible in the response.
	metalake := state.Metalake.ValueString()
	currentName := state.Name.ValueString()

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
	// refuses to operate on a disabled catalog.
	if inUseChanged && plan.InUse.ValueBool() {
		if _, err := r.Client.SetCatalogInUse(ctx, metalake, currentName, true); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("enabling catalog", currentName, err)...)
			return
		}
	}

	var updates []interface{}

	if !plan.Name.Equal(state.Name) {
		updates = append(updates, models.NewRenameCatalogRequest(plan.Name.ValueString()))
	}

	if !plan.Comment.IsUnknown() && !plan.Comment.Equal(state.Comment) {
		updates = append(updates, models.NewUpdateCatalogCommentRequest(plan.Comment.ValueString()))
	}

	if !plan.Properties.IsUnknown() {
		oldProps := mapFromTF(state.Properties)
		newProps := mapFromTF(plan.Properties)

		for k, v := range newProps {
			oldV, exists := oldProps[k]
			if !exists || oldV != v {
				updates = append(updates, models.NewSetCatalogPropertyRequest(k, v))
			}
		}

		for k := range oldProps {
			if _, exists := newProps[k]; !exists {
				updates = append(updates, models.NewRemoveCatalogPropertyRequest(k))
			}
		}
	}

	// A disabled catalog rejects alter requests, so the PUT below would fail.
	// Enable it for the duration of the metadata update and restore the
	// disabled state afterwards when the plan keeps it disabled.
	temporarilyEnabled := !inUseChanged && !plan.InUse.ValueBool() && len(updates) > 0
	if temporarilyEnabled {
		if _, err := r.Client.SetCatalogInUse(ctx, metalake, currentName, true); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("enabling catalog", currentName, err)...)
			return
		}
	}

	if len(updates) > 0 {
		result, err := r.Client.UpdateCatalog(ctx, metalake, currentName, updates)
		if err != nil {
			if temporarilyEnabled {
				if _, restoreErr := r.Client.SetCatalogInUse(ctx, metalake, currentName, false); restoreErr != nil {
					tflog.Warn(ctx, "Failed to re-disable catalog after a failed update", map[string]interface{}{
						"metalake": metalake,
						"name":     currentName,
						"error":    restoreErr.Error(),
					})
				}
			}
			resp.Diagnostics.Append(client.NewResourceError("updating catalog", currentName, err)...)
			return
		}
		setStateFromCatalog(ctx, &resp.Diagnostics, metalake, &result.Catalog, &plan)
	} else {
		// Nothing changed: keep the planned values. The audit is server-managed
		// and therefore unknown in the plan, so it must be filled with a known
		// value explicitly.
		plan.Audit = state.Audit
		setStateFromCatalog(ctx, &resp.Diagnostics, metalake, nil, &plan)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	if (inUseChanged && !plan.InUse.ValueBool()) || temporarilyEnabled {
		finalName := plan.Name.ValueString()
		if _, err := r.Client.SetCatalogInUse(ctx, metalake, finalName, false); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("disabling catalog", finalName, err)...)
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated catalog", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})
}

func (r *CatalogResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state CatalogResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting catalog", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	metalake := state.Metalake.ValueString()
	name := state.Name.ValueString()
	forceDestroy := state.ForceDestroy.ValueBool()

	// Gravitino refuses to drop an in-use catalog even when it is empty.
	// Disabling it removes no metadata and is what the server's own message
	// asks for; force_destroy=false still refuses a catalog with schemas.
	disabled := false
	if !forceDestroy {
		if _, err := r.Client.SetCatalogInUse(ctx, metalake, name, false); err != nil {
			if client.IsNotFoundError(err) {
				tflog.Debug(ctx, "Catalog already deleted", map[string]interface{}{"metalake": metalake, "name": name})
				return
			}
			resp.Diagnostics.Append(client.NewResourceError("disabling catalog before deletion", name, err)...)
			return
		}
		disabled = true
	}

	_, err := r.Client.DropCatalog(ctx, metalake, name, forceDestroy)
	if err != nil {
		if client.IsNotFoundError(err) {
			// Already gone: deleting twice is not an error.
			tflog.Debug(ctx, "Catalog already deleted", map[string]interface{}{"metalake": metalake, "name": name})
			return
		}
		// A refused destroy must not leave a live catalog out of service:
		// restore the state it had before this attempt.
		if disabled && state.InUse.ValueBool() {
			if _, restoreErr := r.Client.SetCatalogInUse(ctx, metalake, name, true); restoreErr != nil {
				tflog.Warn(ctx, "Failed to re-enable catalog after a failed drop", map[string]interface{}{
					"metalake": metalake,
					"name":     name,
					"error":    restoreErr.Error(),
				})
			}
		}
		resp.Diagnostics.Append(client.NewDropError("deleting catalog", name, forceDestroy, err)...)
		return
	}

	tflog.Debug(ctx, "Deleted catalog", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})
}

func (r *CatalogResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idx := strings.LastIndex(req.ID, ".")
	if idx <= 0 || idx == len(req.ID)-1 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected 'metalake.catalog_name', got: %s", req.ID),
		)
		return
	}

	metalake := req.ID[:idx]
	name := req.ID[idx+1:]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), metalake)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func setStateFromCatalog(ctx context.Context, diags *diag.Diagnostics, metalake string, catalog *models.Catalog, model *CatalogResourceModel) {
	if catalog != nil {
		model.Name = types.StringValue(catalog.Name)
		model.Type = types.StringValue(catalog.Type)
		model.Provider = types.StringValue(catalog.Provider)
		model.Comment = types.StringValue(catalog.Comment)
		model.ID = types.StringValue(metalake + "." + catalog.Name)

		mergeCatalogProperties(ctx, diags, model, catalog.Properties)

		if catalog.Audit != nil {
			auditObj, d := auditToObjectValue(ctx, catalog.Audit)
			diags.Append(d...)
			if !diags.HasError() {
				model.Audit = auditObj
			}
		} else {
			// audit is Computed and unknown in the plan on create; leaving it
			// unset would fail with "Provider produced inconsistent result after
			// apply" when the server omits the audit block.
			model.Audit = types.ObjectNull(AuditAttrTypes)
		}
	} else {
		model.ID = types.StringValue(metalake + "." + model.Name.ValueString())
		mergeCatalogProperties(ctx, diags, model, nil)
	}

	model.Metalake = types.StringValue(metalake)

	// in_use cannot be read back. A null value only occurs after an import:
	// adopt Gravitino's creation default so the planned value stays known
	// instead of flipping between null and unknown on every plan.
	if model.InUse.IsNull() {
		model.InUse = types.BoolValue(true)
	}
	// force_destroy has no server-side value either; an imported resource
	// adopts the safe default instead of leaving the attribute null.
	if model.ForceDestroy.IsNull() {
		model.ForceDestroy = types.BoolValue(false)
	}
}

// inUseOnCreate resolves the Optional+Computed in_use value for a new catalog:
// Gravitino creates every catalog in use, so an unconfigured (unknown or null)
// plan becomes true. A configured value is kept.
func inUseOnCreate(planned types.Bool) types.Bool {
	if planned.IsUnknown() || planned.IsNull() {
		return types.BoolValue(true)
	}
	return planned
}

var reservedCatalogProperties = map[string]bool{
	"in-use": true,
}

// mergeCatalogProperties stores exactly the properties of the plan: server
// values are used for the keys the plan contains, but server-only keys (for
// example "in-use", which Gravitino adds to every catalog) are never added.
//
// properties is Optional+Computed: the applied value must equal the planned
// value, otherwise Terraform fails the apply with "Provider produced
// inconsistent result after apply". A configured empty map stays an empty map
// (only a null plan stays null).
func mergeCatalogProperties(ctx context.Context, diags *diag.Diagnostics, model *CatalogResourceModel, serverProps map[string]string) {
	if model.Properties.IsUnknown() {
		model.Properties = types.MapNull(types.StringType)
		return
	}

	if model.Properties.IsNull() {
		// No configured properties (for example while importing): adopt the
		// server's properties so that an import reconstructs the resource,
		// minus the keys Gravitino manages itself.
		serverOnly := make(map[string]string)
		for k, v := range serverProps {
			if !reservedCatalogProperties[k] {
				serverOnly[k] = v
			}
		}
		if len(serverOnly) == 0 {
			model.Properties = types.MapNull(types.StringType)
			return
		}
		props, d := types.MapValueFrom(ctx, types.StringType, serverOnly)
		diags.Append(d...)
		if !diags.HasError() {
			model.Properties = props
		}
		return
	}

	planned := make(map[string]string)
	diags.Append(model.Properties.ElementsAs(ctx, &planned, false)...)
	if diags.HasError() {
		return
	}

	merged := make(map[string]string, len(planned))
	for k, v := range planned {
		if serverValue, ok := serverProps[k]; ok {
			merged[k] = serverValue
			continue
		}
		merged[k] = v
	}

	props, d := types.MapValueFrom(ctx, types.StringType, merged)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Properties = props
}

func auditToObjectValue(ctx context.Context, audit *models.Audit) (types.Object, diag.Diagnostics) {
	return models.AuditToObjectValue(ctx, audit)
}

func mapFromTF(m types.Map) map[string]string {
	result := make(map[string]string)
	if m.IsNull() || m.IsUnknown() {
		return result
	}
	for k, v := range m.Elements() {
		if strVal, ok := v.(types.String); ok {
			result[k] = strVal.ValueString()
		}
	}
	return result
}
