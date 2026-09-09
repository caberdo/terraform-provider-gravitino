package idp_group

import (
	"context"
	"fmt"

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

var _ resource.Resource = &IdpGroupResource{}
var _ resource.ResourceWithImportState = &IdpGroupResource{}
var _ resource.ResourceWithConfigure = &IdpGroupResource{}

type IdpGroupResource struct {
	client *client.Client
}

func New() resource.Resource {
	return &IdpGroupResource{}
}

func (r *IdpGroupResource) SetClient(c *client.Client) {
	r.client = c
}

type IdpGroupResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Comment types.String `tfsdk:"comment"`
	Users   types.Set    `tfsdk:"users"`
}

func (r *IdpGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IdpGroupResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_idp_group"
}

func (r *IdpGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a built-in IDP group for local authentication.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "The name of the built-in IDP group.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The group name.",
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Optional description of the group.",
			},
			"users": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "The usernames of members in the group.",
			},
		},
	}
}

func (r *IdpGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IdpGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating IDP group", map[string]interface{}{"name": plan.Name.ValueString()})

	createReq := &models.IdpAddGroupRequest{
		Group:   plan.Name.ValueString(),
		Comment: plan.Comment.ValueString(),
	}

	result, err := r.client.AddIdpGroup(createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating IDP group", plan.Name.ValueString(), err)...)
		return
	}

	plan.ID = types.StringValue(result.Group.Name)

	users := listToSlice(plan.Users)
	if len(users) > 0 {
		_, err := r.client.ChangeIdpGroupMembership(plan.Name.ValueString(), &models.IdpGroupMembershipChangeRequest{UsersToAdd: users})
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("adding IDP group members", plan.Name.ValueString(), err)...)
			return
		}
	}

	usersList, d := stringSliceToList(ctx, users)
	resp.Diagnostics.Append(d...)
	plan.Users = usersList

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created IDP group", map[string]interface{}{"name": plan.Name.ValueString()})
}

func (r *IdpGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IdpGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading IDP group", map[string]interface{}{"name": state.Name.ValueString()})

	result, err := r.client.GetIdpGroup(state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading IDP group", state.Name.ValueString(), err)...)
		return
	}

	state.ID = types.StringValue(result.Group.Name)
	state.Name = types.StringValue(result.Group.Name)
	state.Comment = types.StringValue(result.Group.Comment)
	users, d := stringSliceToList(ctx, result.Group.Users)
	resp.Diagnostics.Append(d...)
	state.Users = users

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *IdpGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state IdpGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating IDP group", map[string]interface{}{"name": state.Name.ValueString()})

	oldUsers := listToSlice(state.Users)
	newUsers := listToSlice(plan.Users)

	var toAdd, toRemove []string
	for _, u := range newUsers {
		if !containsString(oldUsers, u) {
			toAdd = append(toAdd, u)
		}
	}
	for _, u := range oldUsers {
		if !containsString(newUsers, u) {
			toRemove = append(toRemove, u)
		}
	}

	if len(toAdd) > 0 || len(toRemove) > 0 {
		_, err := r.client.ChangeIdpGroupMembership(state.Name.ValueString(), &models.IdpGroupMembershipChangeRequest{
			UsersToAdd:    toAdd,
			UsersToRemove: toRemove,
		})
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating IDP group membership", state.Name.ValueString(), err)...)
			return
		}
	}

	state.Comment = plan.Comment
	users, d := stringSliceToList(ctx, newUsers)
	resp.Diagnostics.Append(d...)
	state.Users = users

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)

	tflog.Debug(ctx, "Updated IDP group", map[string]interface{}{"name": state.Name.ValueString()})
}

func (r *IdpGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IdpGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting IDP group", map[string]interface{}{"name": state.Name.ValueString()})

	_, err := r.client.RemoveIdpGroup(state.Name.ValueString(), true)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("deleting IDP group", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted IDP group", map[string]interface{}{"name": state.Name.ValueString()})
}

func (r *IdpGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func stringSliceToList(ctx context.Context, items []string) (types.Set, diag.Diagnostics) {
	vals := make([]attr.Value, 0, len(items))
	for _, s := range items {
		vals = append(vals, types.StringValue(s))
	}
	return types.SetValue(types.StringType, vals)
}

func listToSlice(l types.Set) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	result := make([]string, 0, len(l.Elements()))
	for _, v := range l.Elements() {
		if s, ok := v.(types.String); ok {
			result = append(result, s.ValueString())
		}
	}
	return result
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
