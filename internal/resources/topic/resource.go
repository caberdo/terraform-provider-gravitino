package topic

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &TopicResource{}
var _ resource.ResourceWithImportState = &TopicResource{}
var _ resource.ResourceWithConfigure = &TopicResource{}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

type TopicResource struct {
	client *client.Client
}

type TopicResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Metalake   types.String `tfsdk:"metalake"`
	Catalog    types.String `tfsdk:"catalog"`
	Schema     types.String `tfsdk:"schema"`
	Name       types.String `tfsdk:"name"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
}

func NewTopicResource() resource.Resource {
	return &TopicResource{}
}

func (r *TopicResource) SetClient(c *client.Client) {
	r.client = c
}

func (r *TopicResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_topic"
}

func (r *TopicResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Gravitino topic within a metalake catalog schema.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The compound identifier in the format metalake.catalog.schema.topic.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"metalake": schema.StringAttribute{
				Description: "The metalake name. Changing this forces a new topic to be created (Gravitino topics cannot be moved between metalakes).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog name. Changing this forces a new topic to be created (Gravitino topics cannot be moved between catalogs).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schema": schema.StringAttribute{
				Description: "The schema name. Changing this forces a new topic to be created (Gravitino topics cannot be moved between schemas).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The topic name. Changing this forces a new topic to be created: Gravitino supports no rename update for topics.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Description: "A comment describing the topic. Removing the attribute (or setting it to an empty string) clears the comment: Gravitino has no removeComment update, so the provider sends an updateComment update with an empty newComment.",
				Optional:    true,
			},
			"properties": schema.MapAttribute{
				Description: "Key-value properties for the topic. Adding, changing and removing entries is applied in place; only keys that appear in this map are written to state. Omit the attribute to keep the current properties, or set it to {} to remove all of them.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"audit": schema.ObjectAttribute{
				Description:    "Audit information for the topic. Gravitino rewrites lastModifier/lastModifiedTime on every update, so this stays unknown in the plan and is written on every read.",
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
			},
		},
	}
}

func (r *TopicResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TopicResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TopicResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating topic", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})

	properties := make(map[string]string)
	if !plan.Properties.IsNull() && !plan.Properties.IsUnknown() {
		resp.Diagnostics.Append(plan.Properties.ElementsAs(ctx, &properties, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	createReq := &models.TopicCreateRequest{
		Name:       plan.Name.ValueString(),
		Comment:    plan.Comment.ValueString(),
		Properties: properties,
	}

	topicResp, err := r.client.CreateTopic(ctx, plan.Metalake.ValueString(), plan.Catalog.ValueString(), plan.Schema.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating topic", plan.Name.ValueString(), err)...)
		return
	}

	r.readTopicToState(ctx, topicResp, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Created topic", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})
}

func (r *TopicResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TopicResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading topic", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	topicResp, err := r.client.GetTopic(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading topic", state.Name.ValueString(), err)...)
		return
	}

	r.readTopicToState(ctx, topicResp, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TopicResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state TopicResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating topic", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	var updates []interface{}

	if !plan.Comment.Equal(state.Comment) {
		updates = append(updates, models.NewUpdateTopicCommentRequest(plan.Comment.ValueString()))
	}

	if !plan.Properties.Equal(state.Properties) {
		oldProps := make(map[string]string)
		if !state.Properties.IsNull() && !state.Properties.IsUnknown() {
			resp.Diagnostics.Append(state.Properties.ElementsAs(ctx, &oldProps, false)...)
		}

		newProps := make(map[string]string)
		if !plan.Properties.IsNull() && !plan.Properties.IsUnknown() {
			resp.Diagnostics.Append(plan.Properties.ElementsAs(ctx, &newProps, false)...)
		}
		if resp.Diagnostics.HasError() {
			return
		}

		for k := range oldProps {
			if _, exists := newProps[k]; !exists {
				updates = append(updates, models.NewRemoveTopicPropertyRequest(k))
			}
		}
		for k, v := range newProps {
			if oldVal, exists := oldProps[k]; !exists || oldVal != v {
				updates = append(updates, models.NewSetTopicPropertyRequest(k, v))
			}
		}
	}

	if len(updates) > 0 {
		topicResp, err := r.client.UpdateTopic(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString(), updates)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating topic", state.Name.ValueString(), err)...)
			return
		}
		r.readTopicToState(ctx, topicResp, &plan, &resp.Diagnostics)
	} else {
		topicResp, err := r.client.GetTopic(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("reading topic after update", state.Name.ValueString(), err)...)
			return
		}
		r.readTopicToState(ctx, topicResp, &plan, &resp.Diagnostics)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Updated topic", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})
}

func (r *TopicResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TopicResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting topic", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	_, err := r.client.DropTopic(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Debug(ctx, "Topic already deleted", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting topic", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted topic", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})
}

func (r *TopicResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ".")
	if len(parts) != 4 {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected format: metalake.catalog.schema.topic, got: %s", req.ID))
		return
	}
	for _, part := range parts {
		if part == "" {
			resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Import ID must not contain empty segments, got: %s", req.ID))
			return
		}
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schema"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// commentFromServer resolves a topic comment to a known value: the server value
// when it has one, otherwise the planned value when the plan is known, otherwise
// null. An absent server comment must never leave the state unknown.
func commentFromServer(server string, planned types.String) types.String {
	if server != "" {
		return types.StringValue(server)
	}
	if !planned.IsNull() && !planned.IsUnknown() {
		return types.StringValue(planned.ValueString())
	}
	return types.StringNull()
}

func (r *TopicResource) readTopicToState(ctx context.Context, topicResp *models.TopicResponse, m *TopicResourceModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(fmt.Sprintf("%s.%s.%s.%s", m.Metalake.ValueString(), m.Catalog.ValueString(), m.Schema.ValueString(), topicResp.Topic.Name))
	m.Name = types.StringValue(topicResp.Topic.Name)
	m.Comment = commentFromServer(topicResp.Topic.Comment, m.Comment)

	m.Properties = propertiesToState(ctx, topicResp.Topic.Properties, m.Properties, diags)

	auditObj, d := auditToObject(topicResp.Topic.Audit)
	diags.Append(d...)
	m.Audit = auditObj
}

// propertiesToState maps the properties the server reports onto the keys that are
// in the plan/state. Gravitino may report keys the configuration does not contain;
// storing those would fail Terraform's "inconsistent result after apply" check,
// while dropping configured keys would cause perpetual drift.
func propertiesToState(ctx context.Context, server map[string]string, planned types.Map, diags *diag.Diagnostics) types.Map {
	if planned.IsNull() || planned.IsUnknown() {
		if len(server) == 0 {
			return types.MapNull(types.StringType)
		}
		props, d := types.MapValueFrom(ctx, types.StringType, server)
		diags.Append(d...)
		return props
	}

	// A known map (including a configured empty map) keeps exactly its own keys.
	result := make(map[string]string, len(planned.Elements()))
	for key := range planned.Elements() {
		if value, ok := server[key]; ok {
			result[key] = value
			continue
		}
		if plannedValue, ok := planned.Elements()[key].(types.String); ok {
			result[key] = plannedValue.ValueString()
		}
	}

	props, d := types.MapValueFrom(ctx, types.StringType, result)
	diags.Append(d...)
	return props
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
