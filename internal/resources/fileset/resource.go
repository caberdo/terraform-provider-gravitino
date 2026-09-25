package fileset

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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

var _ resource.Resource = &FilesetResource{}
var _ resource.ResourceWithImportState = &FilesetResource{}
var _ resource.ResourceWithConfigure = &FilesetResource{}

type FilesetResource struct {
	client *client.Client
}

func New() resource.Resource {
	return &FilesetResource{}
}

func (r *FilesetResource) SetClient(c *client.Client) {
	r.client = c
}

type FilesetResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Metalake        types.String `tfsdk:"metalake"`
	Catalog         types.String `tfsdk:"catalog"`
	Schema          types.String `tfsdk:"schema"`
	Name            types.String `tfsdk:"name"`
	Comment         types.String `tfsdk:"comment"`
	Type            types.String `tfsdk:"type"`
	StorageLocation types.String `tfsdk:"storage_location"`
	Properties      types.Map    `tfsdk:"properties"`
	Audit           types.Object `tfsdk:"audit"`
}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

func (r *FilesetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *FilesetResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_fileset"
}

func (r *FilesetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Gravitino fileset within a metalake, catalog and schema.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				// No UseStateForUnknown: the id embeds the fileset name, which the
				// rename update request changes in place.
				Description: "Composite identifier in the format 'metalake.catalog.schema.fileset'. It changes when the fileset is renamed.",
			},
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name. Changing it forces the fileset to be recreated, because a fileset is addressed by its metalake, catalog, schema and name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog": schema.StringAttribute{
				Required:    true,
				Description: "The catalog name. Changing it forces the fileset to be recreated, because a fileset is addressed by its metalake, catalog, schema and name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schema": schema.StringAttribute{
				Required:    true,
				Description: "The schema name. Changing it forces the fileset to be recreated, because a fileset is addressed by its metalake, catalog, schema and name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The fileset name. Renaming a fileset is applied in place through the Gravitino rename update request.",
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "A comment or description for the fileset. Changing it is applied in place; setting it to an empty string removes the comment.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The fileset type, either 'managed' or 'external'. Changing it forces the fileset to be recreated: the Gravitino fileset update API cannot change the type.",
				Validators: []validator.String{
					stringvalidator.OneOf("managed", "external"),
				},
				PlanModifiers: []planmodifier.String{
					// UseStateForUnknown must run first: an omitted Optional+Computed
					// attribute is unknown in the plan, and RequiresReplace would then
					// consider every update a change of this attribute.
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"storage_location": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The storage location for the fileset. Mandatory for external filesets; managed filesets default to the location of their namespace. Changing it forces the fileset to be recreated: the Gravitino fileset update API cannot change the storage location.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"properties": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "A map of key-value properties for the fileset. Adding, changing and removing entries is applied in place.",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
				Description:    "Audit information for the fileset. The server updates last_modifier and last_modified_time on every in-place update, so this attribute deliberately has no UseStateForUnknown plan modifier: the refreshed values are written to state after apply.",
			},
		},
	}
}

func (r *FilesetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FilesetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating fileset", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})

	// comment, type, storage_location and properties are Optional+Computed, so an
	// attribute omitted from the configuration is UNKNOWN at this point. Sending
	// the zero value is correct for them: the fields are nullable/optional in
	// FilesetCreateRequest and the server fills in the defaults.
	createReq := &models.FilesetCreateRequest{
		Name:            plan.Name.ValueString(),
		Comment:         plan.Comment.ValueString(),
		Type:            plan.Type.ValueString(),
		StorageLocation: plan.StorageLocation.ValueString(),
		Properties:      propertyMap(ctx, plan.Properties, &resp.Diagnostics),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CreateFileset(ctx, plan.Metalake.ValueString(), plan.Catalog.ValueString(), plan.Schema.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating fileset", plan.Name.ValueString(), err)...)
		return
	}

	mergeFilesetResponse(ctx, &resp.Diagnostics, plan.Metalake.ValueString(), plan.Catalog.ValueString(), plan.Schema.ValueString(), &result.Fileset, &plan, false)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created fileset", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})
}

func (r *FilesetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FilesetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading fileset", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	result, err := r.client.GetFileset(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading fileset", state.Name.ValueString(), err)...)
		return
	}

	mergeFilesetResponse(ctx, &resp.Diagnostics, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), &result.Fileset, &state, true)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)

	tflog.Debug(ctx, "Read fileset", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})
}

func (r *FilesetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state FilesetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The fileset is still addressed by its state (old) name: a rename must be
	// sent TO the old name, never to the new one.
	oldName := state.Name.ValueString()

	tflog.Debug(ctx, "Updating fileset", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": oldName})

	// type, storage_location and the parents are RequiresReplace, so only name,
	// comment and properties can reach this point.
	var updates []interface{}

	if !plan.Name.IsUnknown() && !plan.Name.Equal(state.Name) {
		updates = append(updates, models.NewRenameFilesetRequest(plan.Name.ValueString()))
	}

	// comment and properties are Optional+Computed: when the configuration omits
	// them their planned value can still be unknown. An unknown value carries no
	// information, so nothing may be sent for it.
	if !plan.Comment.IsUnknown() && !plan.Comment.Equal(state.Comment) {
		if plan.Comment.IsNull() || plan.Comment.ValueString() == "" {
			updates = append(updates, models.NewRemoveFilesetCommentRequest())
		} else {
			updates = append(updates, models.NewUpdateFilesetCommentRequest(plan.Comment.ValueString()))
		}
	}

	if !plan.Properties.IsUnknown() && !plan.Properties.Equal(state.Properties) {
		oldProps := propertyMap(ctx, state.Properties, &resp.Diagnostics)
		newProps := propertyMap(ctx, plan.Properties, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}

		for k, v := range newProps {
			if oldV, exists := oldProps[k]; !exists || oldV != v {
				updates = append(updates, models.NewSetFilesetPropertyRequest(k, v))
			}
		}
		for k := range oldProps {
			if _, exists := newProps[k]; !exists {
				updates = append(updates, models.NewRemoveFilesetPropertyRequest(k))
			}
		}
	}

	if len(updates) == 0 {
		// Nothing to send: every value that must be written to state is already
		// known, so the plan (with the unknowns filled from the prior state) is the
		// new state.
		resolveUnknowns(&plan, &state)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		tflog.Debug(ctx, "Updated fileset", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})
		return
	}

	result, err := r.client.UpdateFileset(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), oldName, updates)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("updating fileset", oldName, err)...)
		return
	}

	// The response carries the updated fileset, including the new name and id after
	// a rename.
	mergeFilesetResponse(ctx, &resp.Diagnostics, plan.Metalake.ValueString(), plan.Catalog.ValueString(), plan.Schema.ValueString(), &result.Fileset, &plan, false)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated fileset", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})
}

func (r *FilesetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FilesetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting fileset", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	_, err := r.client.DropFileset(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			// Already gone: the delete is idempotent.
			tflog.Debug(ctx, "Fileset already deleted", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting fileset", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted fileset", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})
}

func (r *FilesetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ".", 4)
	if len(parts) != 4 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected 'metalake.catalog.schema.fileset', got: %s", req.ID),
		)
		return
	}

	metalake := parts[0]
	catalog := parts[1]
	schemaName := parts[2]
	name := parts[3]

	for _, part := range parts {
		if part == "" {
			resp.Diagnostics.AddError(
				"Invalid import ID",
				fmt.Sprintf("The metalake, catalog, schema and fileset segments must not be empty, got: %q", req.ID),
			)
			return
		}
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), metalake)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog"), catalog)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schema"), schemaName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// mergeFilesetResponse copies an API fileset response into the Terraform model.
//
// Values that are already known in `model` win over the response in apply mode
// (refresh == false): Terraform rejects an applied state that differs from the
// plan for an attribute that was known in the plan, and the server rewrites what
// it is given (it appends the "default-location-name" property and normalizes
// "file:///path" storage locations to "file:/path"). Unknown values are filled
// from the response, which is what makes id and audit known after apply.
//
// In refresh mode (Read) the server's values are adopted for everything except
// storage_location. Gravitino cannot change the storage location of an existing
// fileset (the fileset update API has no such request), so the only way the
// echoed value can differ from the configured one is server-side normalisation;
// adopting it would make Terraform plan a replacement on every run. An empty
// state value is still filled from the response, so imports keep working.
func mergeFilesetResponse(ctx context.Context, diags *diag.Diagnostics, metalake, catalog, schemaName string, fileset *models.Fileset, model *FilesetResourceModel, refresh bool) {
	if fileset != nil {
		name := fileset.Name
		if !refresh && !model.Name.IsUnknown() && !model.Name.IsNull() && model.Name.ValueString() != "" {
			// The configured (planned) name is the promise Terraform holds us to.
			name = model.Name.ValueString()
		}
		model.Name = types.StringValue(name)

		if refresh || model.Comment.IsUnknown() || model.Comment.IsNull() {
			model.Comment = types.StringValue(fileset.Comment)
		}
		if refresh || model.Type.IsUnknown() || model.Type.IsNull() {
			model.Type = types.StringValue(fileset.Type)
		}
		if model.StorageLocation.IsUnknown() || model.StorageLocation.IsNull() {
			model.StorageLocation = types.StringValue(fileset.StorageLocation)
		}
		if model.Properties.IsUnknown() || model.Properties.IsNull() {
			// No configured property set (create with the attribute omitted, or an
			// import): adopt the server's map, minus the properties Gravitino manages
			// itself, so the value is known after apply and no server-only key leaks
			// into state.
			props, d := types.MapValueFrom(ctx, types.StringType, serverProperties(fileset.Properties))
			diags.Append(d...)
			if !diags.HasError() {
				model.Properties = props
			}
		} else if refresh {
			// Refresh mode: keep the configured key set, take the server's value for
			// every key it reports. Server-only keys (the "default-location-name"
			// property Gravitino appends) are never added, configured keys are never
			// dropped.
			model.Properties = projectProperties(ctx, model.Properties, fileset.Properties, diags)
		}

		model.ID = types.StringValue(metalake + "." + catalog + "." + schemaName + "." + fileset.Name)

		auditObj, d := auditToObjectValue(ctx, fileset.Audit)
		diags.Append(d...)
		if !diags.HasError() {
			model.Audit = auditObj
		}
	}

	model.Metalake = types.StringValue(metalake)
	model.Catalog = types.StringValue(catalog)
	model.Schema = types.StringValue(schemaName)
}

// serverManagedProperties are the property keys Gravitino adds to a fileset
// itself (measured against 1.3.0: a created fileset always carries
// "default-location-name"). They are never written into state when the
// configuration does not ask for them, otherwise they would look like permanent
// drift and would be sent back as removeProperty requests.
var serverManagedProperties = map[string]struct{}{
	"default-location-name": {},
}

// serverProperties returns the server's property map without the keys Gravitino
// manages itself.
func serverProperties(props map[string]string) map[string]string {
	out := make(map[string]string, len(props))
	for k, v := range props {
		if _, managed := serverManagedProperties[k]; managed {
			continue
		}
		out[k] = v
	}
	return out
}

// projectProperties returns the values of `current` (the configured key set) with
// the value the server reports for each key that it also reports.
func projectProperties(ctx context.Context, current types.Map, server map[string]string, diags *diag.Diagnostics) types.Map {
	out := make(map[string]string)
	for k, v := range current.Elements() {
		if serverValue, ok := server[k]; ok {
			out[k] = serverValue
			continue
		}
		if s, ok := v.(types.String); ok && !s.IsUnknown() {
			out[k] = s.ValueString()
		}
	}
	value, d := types.MapValueFrom(ctx, types.StringType, out)
	diags.Append(d...)
	return value
}

// resolveUnknowns replaces every unknown value in the new state with the prior
// state's value (or null when the prior value is not usable). Terraform rejects a
// state that still contains unknown values after apply.
func resolveUnknowns(model, prior *FilesetResourceModel) {
	model.ID = resolveUnknownString(model.ID, prior.ID)
	model.Name = resolveUnknownString(model.Name, prior.Name)
	model.Comment = resolveUnknownString(model.Comment, prior.Comment)
	model.Type = resolveUnknownString(model.Type, prior.Type)
	model.StorageLocation = resolveUnknownString(model.StorageLocation, prior.StorageLocation)

	if model.Properties.IsUnknown() {
		if prior.Properties.IsNull() || prior.Properties.IsUnknown() {
			model.Properties = types.MapNull(types.StringType)
		} else {
			model.Properties = prior.Properties
		}
	}
	if model.Audit.IsUnknown() {
		if prior.Audit.IsNull() || prior.Audit.IsUnknown() {
			model.Audit = types.ObjectNull(AuditAttrTypes)
		} else {
			model.Audit = prior.Audit
		}
	}
}

func resolveUnknownString(plan, prior types.String) types.String {
	if !plan.IsUnknown() {
		return plan
	}
	if prior.IsNull() || prior.IsUnknown() {
		return types.StringNull()
	}
	return prior
}

func auditToObjectValue(ctx context.Context, audit *models.Audit) (types.Object, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(AuditAttrTypes), nil
	}

	creator := types.StringValue(audit.Creator)
	lastModifier := types.StringValue(audit.LastModifier)

	var createTime, lastModifiedTime types.String
	if audit.CreateTime != nil {
		createTime = types.StringValue(audit.CreateTime.Format(time.RFC3339))
	} else {
		createTime = types.StringNull()
	}
	if audit.LastModifiedTime != nil {
		lastModifiedTime = types.StringValue(audit.LastModifiedTime.Format(time.RFC3339))
	} else {
		lastModifiedTime = types.StringNull()
	}

	attrs := map[string]attr.Value{
		"creator":            creator,
		"create_time":        createTime,
		"last_modifier":      lastModifier,
		"last_modified_time": lastModifiedTime,
	}

	return types.ObjectValue(AuditAttrTypes, attrs)
}

// propertyMap converts a Terraform map attribute into a plain Go map, appending
// any conversion diagnostics. Null and unknown maps yield an empty map.
func propertyMap(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	result := make(map[string]string)
	if m.IsNull() || m.IsUnknown() {
		return result
	}
	diags.Append(m.ElementsAs(ctx, &result, false)...)
	return result
}
