package model_version

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &ModelVersionResource{}
var _ resource.ResourceWithImportState = &ModelVersionResource{}
var _ resource.ResourceWithConfigure = &ModelVersionResource{}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

type ModelVersionResource struct {
	client *client.Client
}

func NewModelVersionResource() resource.Resource {
	return &ModelVersionResource{}
}

func (r *ModelVersionResource) SetClient(c *client.Client) {
	r.client = c
}

type ModelVersionResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Metalake   types.String `tfsdk:"metalake"`
	Catalog    types.String `tfsdk:"catalog"`
	Schema     types.String `tfsdk:"schema"`
	Model      types.String `tfsdk:"model"`
	Version    types.Int64  `tfsdk:"version"`
	URI        types.String `tfsdk:"uri"`
	URIs       types.Map    `tfsdk:"uris"`
	Aliases    types.Set    `tfsdk:"aliases"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
}

func (r *ModelVersionResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_model_version"
}

func (r *ModelVersionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Links a model version to a Gravitino model. Model artifacts are attached to the version, not to the model: set either uri or uris.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The compound identifier in the format metalake.catalog.schema.model.version.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"metalake": schema.StringAttribute{
				Description: "The metalake name. Changing it forces a new model version to be linked, because Gravitino cannot move a model version between metalakes.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog name. Changing it forces a new model version to be linked, because Gravitino cannot move a model version between catalogs.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schema": schema.StringAttribute{
				Description: "The schema name. Changing it forces a new model version to be linked, because Gravitino cannot move a model version between schemas.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"model": schema.StringAttribute{
				Description: "The model name. Changing it forces a new model version to be linked, because model versions belong to their model.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.Int64Attribute{
				Description: "The model version number, assigned by Gravitino. Linking a model version always creates the next version number.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"uri": schema.StringAttribute{
				Description: "The unnamed URI of the model artifact. Gravitino has no operation to remove the unnamed URI, so removing it from the configuration keeps the existing value.",
				Optional:    true,
				Computed:    true,
			},
			"uris": schema.MapAttribute{
				Description: "The URIs of the model artifact, keyed by URI name. Adding a key sends an addUri update, changing a value an updateUri update and removing a key a removeUri update.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"aliases": schema.SetAttribute{
				Description: "Aliases for this model version. Gravitino rejects numeric aliases.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"comment": schema.StringAttribute{
				Description: "A comment describing the model version.",
				Optional:    true,
				Computed:    true,
			},
			"properties": schema.MapAttribute{
				Description: "Key-value properties for the model version.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"audit": schema.ObjectAttribute{
				Description:    "Audit information for the model version.",
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
			},
		},
	}
}

func (r *ModelVersionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ModelVersionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ModelVersionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating model version", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "model": plan.Model.ValueString()})

	uri := plan.URI.ValueString()
	uris := propertiesFromTF(ctx, plan.URIs, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if uri == "" && len(uris) == 0 {
		resp.Diagnostics.AddError(
			"Invalid model version configuration",
			"Either uri or uris must be set: Gravitino requires the artifact location of a model version.",
		)
		return
	}

	createReq := &models.ModelVersionLinkRequest{
		Aliases:    stringSetFromTF(ctx, plan.Aliases, &resp.Diagnostics),
		Comment:    plan.Comment.ValueString(),
		Properties: propertiesFromTF(ctx, plan.Properties, &resp.Diagnostics),
	}
	if resp.Diagnostics.HasError() {
		return
	}
	// Gravitino rejects a request that carries neither uri nor uris, and the
	// uris map is the primary representation of a version's artifact locations.
	if len(uris) > 0 {
		createReq.URIs = uris
	} else {
		createReq.URI = uri
	}

	if _, err := r.client.LinkModelVersion(ctx, plan.Metalake.ValueString(), plan.Catalog.ValueString(), plan.Schema.ValueString(), plan.Model.ValueString(), createReq); err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating model version", plan.Model.ValueString(), err)...)
		return
	}

	// Gravitino answers the link request with a plain BaseResponse: the version
	// number it assigned has to be read back. A newly assigned version is always
	// the highest version number of the model.
	linkedVersion, err := r.newestModelVersion(ctx, plan.Metalake.ValueString(), plan.Catalog.ValueString(), plan.Schema.ValueString(), plan.Model.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading the linked model version", plan.Model.ValueString(), err)...)
		return
	}

	r.setModelVersionState(ctx, linkedVersion, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created model version", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "model": plan.Model.ValueString(), "version": plan.Version.ValueInt64()})
}

func (r *ModelVersionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ModelVersionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading model version", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "model": state.Model.ValueString(), "version": state.Version.ValueInt64()})

	result, err := r.client.GetModelVersion(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Model.ValueString(), int32(state.Version.ValueInt64()))
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading model version", fmt.Sprintf("%s version %d", state.Model.ValueString(), state.Version.ValueInt64()), err)...)
		return
	}

	r.setModelVersionState(ctx, result.ModelVersion, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)

	tflog.Debug(ctx, "Read model version", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "model": state.Model.ValueString(), "version": state.Version.ValueInt64()})
}

func (r *ModelVersionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ModelVersionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating model version", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "model": state.Model.ValueString(), "version": state.Version.ValueInt64()})

	var updates []interface{}

	if attributeChanged(plan.Comment, state.Comment) {
		updates = append(updates, models.NewUpdateModelVersionCommentRequest(plan.Comment.ValueString()))
	}

	if attributeChanged(plan.Properties, state.Properties) {
		updates = append(updates, modelVersionPropertyUpdates(ctx, state.Properties, plan.Properties, &resp.Diagnostics)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if attributeChanged(plan.Aliases, state.Aliases) {
		added, removed := aliasDiff(ctx, state.Aliases, plan.Aliases, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(added) > 0 || len(removed) > 0 {
			updates = append(updates, models.NewUpdateModelVersionAliasesRequest(added, removed))
		}
	}

	if attributeChanged(plan.URI, state.URI) {
		if newURI := plan.URI.ValueString(); newURI != "" {
			updates = append(updates, models.NewUpdateModelVersionURIRequest(newURI, ""))
		} else {
			tflog.Warn(ctx, "Gravitino cannot unset the unnamed URI of a model version, keeping the existing URI", map[string]interface{}{"model": state.Model.ValueString(), "version": state.Version.ValueInt64()})
		}
	}

	if attributeChanged(plan.URIs, state.URIs) {
		updates = append(updates, uriUpdates(ctx, state.URIs, plan.URIs, &resp.Diagnostics)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	var result *models.ModelVersionResponse
	if len(updates) > 0 {
		updated, err := r.client.UpdateModelVersion(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Model.ValueString(), int32(state.Version.ValueInt64()), updates)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating model version", fmt.Sprintf("%s version %d", state.Model.ValueString(), state.Version.ValueInt64()), err)...)
			return
		}
		result = updated
	} else {
		// Nothing to send, but the computed attributes still need their real
		// values in state.
		current, err := r.client.GetModelVersion(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Model.ValueString(), int32(state.Version.ValueInt64()))
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("reading model version after update", fmt.Sprintf("%s version %d", state.Model.ValueString(), state.Version.ValueInt64()), err)...)
			return
		}
		result = current
	}

	r.setModelVersionState(ctx, result.ModelVersion, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated model version", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "model": plan.Model.ValueString(), "version": plan.Version.ValueInt64()})
}

func (r *ModelVersionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ModelVersionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting model version", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "model": state.Model.ValueString(), "version": state.Version.ValueInt64()})

	_, err := r.client.DeleteModelVersion(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Model.ValueString(), int32(state.Version.ValueInt64()))
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Debug(ctx, "Model version already deleted", map[string]interface{}{"model": state.Model.ValueString(), "version": state.Version.ValueInt64()})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting model version", fmt.Sprintf("%s version %d", state.Model.ValueString(), state.Version.ValueInt64()), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted model version", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "model": state.Model.ValueString(), "version": state.Version.ValueInt64()})
}

func (r *ModelVersionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ".", 5)
	if len(parts) != 5 {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected format: metalake.catalog.schema.model.version, got %q", req.ID))
		return
	}
	for _, part := range parts {
		if part == "" {
			resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected format: metalake.catalog.schema.model.version, got %q", req.ID))
			return
		}
	}

	version, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil || version < 0 {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("The version must be a non-negative integer, got %q", parts[4]))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schema"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("model"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("version"), version)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// newestModelVersion returns the model version with the highest version number.
// Gravitino assigns an increasing number to every linked version, so the newest
// version is the one that was just linked.
func (r *ModelVersionResource) newestModelVersion(ctx context.Context, metalake, catalog, schemaName, model string) (models.ModelVersion, error) {
	list, err := r.client.ListModelVersions(ctx, metalake, catalog, schemaName, model, true)
	if err != nil {
		return models.ModelVersion{}, err
	}

	if len(list.Infos) > 0 {
		newest := list.Infos[0]
		for _, version := range list.Infos[1:] {
			if version.Version > newest.Version {
				newest = version
			}
		}
		return newest, nil
	}

	// Gravitino only returned version numbers: fetch the highest one.
	if len(list.Versions) > 0 {
		version := list.Versions[0]
		for _, candidate := range list.Versions[1:] {
			if candidate > version {
				version = candidate
			}
		}
		result, err := r.client.GetModelVersion(ctx, metalake, catalog, schemaName, model, version)
		if err != nil {
			return models.ModelVersion{}, err
		}
		return result.ModelVersion, nil
	}

	return models.ModelVersion{}, fmt.Errorf("the server reported no model versions for model %q after linking one", model)
}

// setModelVersionState writes every attribute, including the computed ones, so
// no computed value is ever left unknown after an apply.
func (r *ModelVersionResource) setModelVersionState(ctx context.Context, mv models.ModelVersion, state *ModelVersionResourceModel, diags *diag.Diagnostics) {
	state.Version = types.Int64Value(int64(mv.Version))
	state.ID = types.StringValue(fmt.Sprintf("%s.%s.%s.%s.%d", state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Model.ValueString(), mv.Version))
	state.URI = optionalString(mv.URI, state.URI)
	state.Comment = optionalString(mv.Comment, state.Comment)
	state.URIs = mapValueFrom(ctx, mv.URIs, state.URIs, diags)
	state.Properties = mapValueFrom(ctx, mv.Properties, state.Properties, diags)
	state.Aliases = setValueFrom(ctx, mv.Aliases, state.Aliases, diags)

	auditObj, d := auditToObject(mv.Audit)
	diags.Append(d...)
	state.Audit = auditObj
}

// attributeChanged reports whether the plan asks for a change. An unknown plan
// value means the attribute was not configured, which is never a change
// request.
func attributeChanged(plan, current attr.Value) bool {
	if plan.IsUnknown() {
		return false
	}
	return !plan.Equal(current)
}

// optionalString maps an absent API value to null, while preserving a value
// that was explicitly configured as the empty string.
func optionalString(apiValue string, current types.String) types.String {
	if apiValue != "" {
		return types.StringValue(apiValue)
	}
	if current.IsNull() || current.IsUnknown() {
		return types.StringNull()
	}
	return types.StringValue("")
}

// mapValueFrom maps a string map to a Terraform value, preserving an explicitly
// configured empty map and using null when nothing was configured.
func mapValueFrom(ctx context.Context, values map[string]string, current types.Map, diags *diag.Diagnostics) types.Map {
	if len(values) > 0 {
		value, d := types.MapValueFrom(ctx, types.StringType, values)
		diags.Append(d...)
		return value
	}
	if !current.IsNull() && !current.IsUnknown() {
		return types.MapValueMust(types.StringType, map[string]attr.Value{})
	}
	return types.MapNull(types.StringType)
}

// setValueFrom maps a string slice to a Terraform set, preserving an explicitly
// configured empty set and using null when nothing was configured.
func setValueFrom(ctx context.Context, values []string, current types.Set, diags *diag.Diagnostics) types.Set {
	if len(values) > 0 {
		value, d := types.SetValueFrom(ctx, types.StringType, values)
		diags.Append(d...)
		return value
	}
	if !current.IsNull() && !current.IsUnknown() {
		return types.SetValueMust(types.StringType, []attr.Value{})
	}
	return types.SetNull(types.StringType)
}

func propertiesFromTF(ctx context.Context, value types.Map, diags *diag.Diagnostics) map[string]string {
	properties := make(map[string]string)
	if value.IsNull() || value.IsUnknown() {
		return properties
	}
	diags.Append(value.ElementsAs(ctx, &properties, false)...)
	return properties
}

func stringSetFromTF(ctx context.Context, value types.Set, diags *diag.Diagnostics) []string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var result []string
	diags.Append(value.ElementsAs(ctx, &result, false)...)
	return result
}

func modelVersionPropertyUpdates(ctx context.Context, oldValue, newValue types.Map, diags *diag.Diagnostics) []interface{} {
	oldProps := propertiesFromTF(ctx, oldValue, diags)
	newProps := propertiesFromTF(ctx, newValue, diags)
	if diags.HasError() {
		return nil
	}

	updates := make([]interface{}, 0, len(oldProps)+len(newProps))
	for _, key := range sortedKeys(oldProps) {
		if _, exists := newProps[key]; !exists {
			updates = append(updates, models.NewRemoveModelVersionPropertyRequest(key))
		}
	}
	for _, key := range sortedKeys(newProps) {
		if old, exists := oldProps[key]; !exists || old != newProps[key] {
			updates = append(updates, models.NewSetModelVersionPropertyRequest(key, newProps[key]))
		}
	}
	return updates
}

func uriUpdates(ctx context.Context, oldValue, newValue types.Map, diags *diag.Diagnostics) []interface{} {
	oldURIs := propertiesFromTF(ctx, oldValue, diags)
	newURIs := propertiesFromTF(ctx, newValue, diags)
	if diags.HasError() {
		return nil
	}

	updates := make([]interface{}, 0, len(oldURIs)+len(newURIs))
	for _, name := range sortedKeys(oldURIs) {
		if _, exists := newURIs[name]; !exists {
			updates = append(updates, models.NewRemoveModelVersionURIRequest(name))
		}
	}
	for _, name := range sortedKeys(newURIs) {
		uri := newURIs[name]
		if old, exists := oldURIs[name]; !exists {
			updates = append(updates, models.NewAddModelVersionURIRequest(name, uri))
		} else if old != uri {
			updates = append(updates, models.NewUpdateModelVersionURIRequest(uri, name))
		}
	}
	return updates
}

func aliasDiff(ctx context.Context, oldValue, newValue types.Set, diags *diag.Diagnostics) ([]string, []string) {
	oldAliases := stringSetFromTF(ctx, oldValue, diags)
	newAliases := stringSetFromTF(ctx, newValue, diags)
	if diags.HasError() {
		return nil, nil
	}

	oldSet := make(map[string]struct{}, len(oldAliases))
	for _, alias := range oldAliases {
		oldSet[alias] = struct{}{}
	}
	newSet := make(map[string]struct{}, len(newAliases))
	for _, alias := range newAliases {
		newSet[alias] = struct{}{}
	}

	added := make([]string, 0, len(newAliases))
	for _, alias := range newAliases {
		if _, exists := oldSet[alias]; !exists {
			added = append(added, alias)
		}
	}
	removed := make([]string, 0, len(oldAliases))
	for _, alias := range oldAliases {
		if _, exists := newSet[alias]; !exists {
			removed = append(removed, alias)
		}
	}
	// A set has no defined element order, so sort to keep the update payload stable.
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

// sortedKeys keeps the generated update requests deterministic.
func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func auditToObject(audit *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(AuditAttrTypes), nil
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

	return types.ObjectValue(AuditAttrTypes, map[string]attr.Value{
		"creator":            creator,
		"create_time":        createTime,
		"last_modifier":      lastModifier,
		"last_modified_time": lastModifiedTime,
	})
}
