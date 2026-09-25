package model

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &ModelResource{}
var _ resource.ResourceWithImportState = &ModelResource{}
var _ resource.ResourceWithConfigure = &ModelResource{}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

type ModelResource struct {
	client *client.Client
}

func New() resource.Resource {
	return &ModelResource{}
}

func (r *ModelResource) SetClient(c *client.Client) {
	r.client = c
}

type ModelResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Metalake      types.String `tfsdk:"metalake"`
	Catalog       types.String `tfsdk:"catalog"`
	Schema        types.String `tfsdk:"schema"`
	Name          types.String `tfsdk:"name"`
	Comment       types.String `tfsdk:"comment"`
	LatestVersion types.Int64  `tfsdk:"latest_version"`
	Properties    types.Map    `tfsdk:"properties"`
	Audit         types.Object `tfsdk:"audit"`
}

func (r *ModelResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_model"
}

func (r *ModelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Gravitino model within a metalake, catalog, and schema. Model artifacts are not attached to the model itself but to a gravitino_model_version.",
		Attributes: map[string]schema.Attribute{
			// id embeds the model name, which is renameable, so it is planned as
			// unknown on updates. It is written on every apply path (create,
			// read and update), so it never stays unknown in state.
			"id": schema.StringAttribute{
				Description: "The compound identifier in the format metalake.catalog.schema.model.",
				Computed:    true,
			},
			"metalake": schema.StringAttribute{
				Description: "The metalake name. Changing it forces a new model to be registered, because Gravitino cannot move a model between metalakes.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog name. Changing it forces a new model to be registered, because Gravitino cannot move a model between catalogs.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schema": schema.StringAttribute{
				Description: "The schema name. Changing it forces a new model to be registered, because Gravitino cannot move a model between schemas.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The model name. Renaming it sends a `rename` update to Gravitino.",
				Required:    true,
			},
			"comment": schema.StringAttribute{
				Description: "A comment describing the model.",
				Optional:    true,
			},
			"latest_version": schema.Int64Attribute{
				Description: "The latest version number of the model. Gravitino assigns the number and increases it every time a model version is linked to this model.",
				Computed:    true,
			},
			"properties": schema.MapAttribute{
				Description: "Key-value properties for the model.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"audit": schema.ObjectAttribute{
				Description:    "Audit information for the model.",
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
			},
		},
	}
}

func (r *ModelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ModelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ModelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating model", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})

	properties := propertiesFromTF(ctx, plan.Properties, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := &models.ModelRegisterRequest{
		Name:       plan.Name.ValueString(),
		Comment:    plan.Comment.ValueString(),
		Properties: properties,
	}

	modelResp, err := r.client.CreateModel(ctx, plan.Metalake.ValueString(), plan.Catalog.ValueString(), plan.Schema.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating model", plan.Name.ValueString(), err)...)
		return
	}

	r.readModelToState(ctx, &modelResp.Model, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Created model", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})
}

func (r *ModelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ModelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading model", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	modelResp, err := r.client.GetModel(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading model", state.Name.ValueString(), err)...)
		return
	}

	r.readModelToState(ctx, &modelResp.Model, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	tflog.Debug(ctx, "Read model", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})
}

func (r *ModelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ModelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating model", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	var updates []interface{}

	if !plan.Name.Equal(state.Name) {
		updates = append(updates, models.NewRenameModelRequest(plan.Name.ValueString()))
	}
	if !plan.Comment.Equal(state.Comment) {
		updates = append(updates, models.NewUpdateModelCommentRequest(plan.Comment.ValueString()))
	}
	if !plan.Properties.Equal(state.Properties) {
		updates = append(updates, modelPropertyUpdates(ctx, state.Properties, plan.Properties, &resp.Diagnostics)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	var modelResp *models.ModelResponse
	if len(updates) > 0 {
		result, err := r.client.UpdateModel(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString(), updates)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating model", state.Name.ValueString(), err)...)
			return
		}
		modelResp = result
	} else {
		result, err := r.client.GetModel(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("reading model after update", state.Name.ValueString(), err)...)
			return
		}
		modelResp = result
	}

	r.readModelToState(ctx, &modelResp.Model, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Updated model", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})
}

func (r *ModelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ModelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting model", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	_, err := r.client.DropModel(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Debug(ctx, "Model already deleted", map[string]interface{}{"name": state.Name.ValueString()})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting model", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted model", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})
}

func (r *ModelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ".", 4)
	if len(parts) != 4 {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected format: metalake.catalog.schema.model, got %q", req.ID))
		return
	}
	for _, part := range parts {
		if part == "" {
			resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected format: metalake.catalog.schema.model, got %q", req.ID))
			return
		}
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schema"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readModelToState writes every attribute, including the computed ones, so no
// computed value is ever left unknown after an apply.
func (r *ModelResource) readModelToState(ctx context.Context, m *models.Model, state *ModelResourceModel, diags *diag.Diagnostics) {
	state.Name = types.StringValue(m.Name)
	state.ID = types.StringValue(fmt.Sprintf("%s.%s.%s.%s", state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), m.Name))
	state.LatestVersion = types.Int64Value(int64(m.LatestVersion))
	state.Comment = optionalString(m.Comment, state.Comment)
	state.Properties = mapValueFrom(ctx, m.Properties, state.Properties, diags)

	auditObj, d := auditToObject(m.Audit)
	diags.Append(d...)
	state.Audit = auditObj
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

// mapValueFrom maps a property map to a Terraform value, preserving an
// explicitly configured empty map and using null when nothing was configured.
func mapValueFrom(ctx context.Context, props map[string]string, current types.Map, diags *diag.Diagnostics) types.Map {
	if len(props) > 0 {
		value, d := types.MapValueFrom(ctx, types.StringType, props)
		diags.Append(d...)
		return value
	}
	if !current.IsNull() && !current.IsUnknown() {
		return types.MapValueMust(types.StringType, map[string]attr.Value{})
	}
	return types.MapNull(types.StringType)
}

func propertiesFromTF(ctx context.Context, value types.Map, diags *diag.Diagnostics) map[string]string {
	properties := make(map[string]string)
	if value.IsNull() || value.IsUnknown() {
		return properties
	}
	diags.Append(value.ElementsAs(ctx, &properties, false)...)
	return properties
}

// modelPropertyUpdates diffs the old and new property maps into the setProperty
// and removeProperty updates Gravitino understands.
func modelPropertyUpdates(ctx context.Context, oldValue, newValue types.Map, diags *diag.Diagnostics) []interface{} {
	oldProps := propertiesFromTF(ctx, oldValue, diags)
	newProps := propertiesFromTF(ctx, newValue, diags)
	if diags.HasError() {
		return nil
	}

	updates := make([]interface{}, 0, len(oldProps)+len(newProps))
	for key := range oldProps {
		if _, exists := newProps[key]; !exists {
			updates = append(updates, models.NewRemoveModelPropertyRequest(key))
		}
	}
	for key, value := range newProps {
		if oldValue, exists := oldProps[key]; !exists || oldValue != value {
			updates = append(updates, models.NewSetModelPropertyRequest(key, value))
		}
	}
	return updates
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
