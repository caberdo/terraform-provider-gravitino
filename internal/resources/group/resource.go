package group

import (
	"context"
	"fmt"
	"strings"

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
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &GroupResource{}
var _ resource.ResourceWithImportState = &GroupResource{}
var _ resource.ResourceWithConfigure = &GroupResource{}

type GroupResource struct {
	client *client.Client
}

func New() resource.Resource {
	return &GroupResource{}
}

func (r *GroupResource) SetClient(c *client.Client) {
	r.client = c
}

type GroupResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Metalake types.String `tfsdk:"metalake"`
	Name     types.String `tfsdk:"name"`
	Roles    types.Set    `tfsdk:"roles"`
	Audit    types.Object `tfsdk:"audit"`
}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

func (r *GroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GroupResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_group"
}

func (r *GroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "Composite identifier in the format 'metalake.group_name'.",
			},
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The group name.",
			},
			"roles": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "The roles assigned to the group.",
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
				Description:    "Audit information for the group.",
			},
		},
	}
}

func (r *GroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan GroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating group", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})

	result, err := r.client.AddGroup(plan.Metalake.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating group", plan.Name.ValueString(), err)...)
		return
	}

	metalake := plan.Metalake.ValueString()

	plannedRoles := listFromTF(plan.Roles)
	if len(plannedRoles) > 0 {
		grantResult, err := r.client.GrantRolesToGroup(metalake, plan.Name.ValueString(), plannedRoles)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("granting roles to group", plan.Name.ValueString(), err)...)
			return
		}
		setStateFromGroup(ctx, &resp.Diagnostics, metalake, &grantResult.Group, &plan)
	} else {
		setStateFromGroup(ctx, &resp.Diagnostics, metalake, &result.Group, &plan)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created group", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})
}

func (r *GroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state GroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading group", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	result, err := r.client.GetGroup(state.Metalake.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading group", state.Name.ValueString(), err)...)
		return
	}

	setStateFromGroup(ctx, &resp.Diagnostics, state.Metalake.ValueString(), &result.Group, &state)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *GroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state GroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating group", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	metalake := plan.Metalake.ValueString()
	groupName := state.Name.ValueString()

	oldRoles := listFromTF(state.Roles)
	newRoles := listFromTF(plan.Roles)

	rolesToGrant := diffRoles(newRoles, oldRoles)
	rolesToRevoke := diffRoles(oldRoles, newRoles)

	if len(rolesToGrant) > 0 {
		grantResult, err := r.client.GrantRolesToGroup(metalake, groupName, rolesToGrant)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("granting roles to group", groupName, err)...)
			return
		}
		setStateFromGroup(ctx, &resp.Diagnostics, metalake, &grantResult.Group, &plan)
	} else {
		setStateFromGroup(ctx, &resp.Diagnostics, metalake, nil, &plan)
	}

	if len(rolesToRevoke) > 0 {
		revokeResult, err := r.client.RevokeRolesFromGroup(metalake, groupName, rolesToRevoke)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("revoking roles from group", groupName, err)...)
			return
		}
		setStateFromGroup(ctx, &resp.Diagnostics, metalake, &revokeResult.Group, &plan)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated group", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})
}

func (r *GroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting group", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	_, err := r.client.RemoveGroup(state.Metalake.ValueString(), state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("deleting group", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted group", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})
}

func (r *GroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idx := strings.LastIndex(req.ID, ".")
	if idx == -1 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected 'metalake.group_name', got: %s", req.ID),
		)
		return
	}

	metalake := req.ID[:idx]
	name := req.ID[idx+1:]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), metalake)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func setStateFromGroup(ctx context.Context, diags *diag.Diagnostics, metalake string, group *models.Group, model *GroupResourceModel) {
	if group != nil {
		model.Name = types.StringValue(group.Name)
		model.ID = types.StringValue(metalake + "." + group.Name)

		roles, d := types.SetValueFrom(ctx, types.StringType, group.Roles)
		diags.Append(d...)
		if !diags.HasError() {
			model.Roles = roles
		}

		if group.Audit != nil {
			auditObj, d := auditToObjectValue(ctx, group.Audit)
			diags.Append(d...)
			if !diags.HasError() {
				model.Audit = auditObj
			}
		}
	} else {
		model.ID = types.StringValue(metalake + "." + model.Name.ValueString())
	}

	model.Metalake = types.StringValue(metalake)
}

func auditToObjectValue(ctx context.Context, audit *models.Audit) (types.Object, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(AuditAttrTypes), nil
	}

	creator := types.StringValue(audit.Creator)
	lastModifier := types.StringValue(audit.LastModifier)

	var createTime, lastModifiedTime types.String
	if audit.CreateTime != nil {
		createTime = types.StringValue(audit.CreateTime.Format("2006-01-02T15:04:05Z07:00"))
	} else {
		createTime = types.StringNull()
	}
	if audit.LastModifiedTime != nil {
		lastModifiedTime = types.StringValue(audit.LastModifiedTime.Format("2006-01-02T15:04:05Z07:00"))
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

func listFromTF(l types.Set) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var result []string
	for _, v := range l.Elements() {
		if strVal, ok := v.(types.String); ok {
			result = append(result, strVal.ValueString())
		}
	}
	return result
}

func diffRoles(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, s := range b {
		set[s] = true
	}
	var diff []string
	for _, s := range a {
		if !set[s] {
			diff = append(diff, s)
		}
	}
	return diff
}
