package idp_user

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &IdpUserResource{}
var _ resource.ResourceWithImportState = &IdpUserResource{}
var _ resource.ResourceWithConfigure = &IdpUserResource{}

type IdpUserResource struct {
	client *client.Client
}

func New() resource.Resource {
	return &IdpUserResource{}
}

func (r *IdpUserResource) SetClient(c *client.Client) {
	r.client = c
}

type IdpUserResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Password types.String `tfsdk:"password"`
	Enabled  types.Bool   `tfsdk:"enabled"`
	Groups   types.Set    `tfsdk:"groups"`
}

func (r *IdpUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IdpUserResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_idp_user"
}

func (r *IdpUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a built-in IDP user for local authentication.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "The username of the built-in IDP user.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The username. Must not contain ':'.",
			},
			"password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The password (12-64 characters).",
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
				Description: "Whether the user is enabled. Disabled users cannot authenticate.",
			},
			"groups": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "The groups the user belongs to.",
			},
		},
	}
}

func (r *IdpUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IdpUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating IDP user", map[string]interface{}{"name": plan.Name.ValueString()})

	enabled := plan.Enabled.ValueBool()
	createReq := &models.IdpAddUserRequest{
		User:     plan.Name.ValueString(),
		Password: plan.Password.ValueString(),
		Enabled:  &enabled,
	}

	result, err := r.client.AddIdpUser(createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating IDP user", plan.Name.ValueString(), err)...)
		return
	}

	plan.ID = types.StringValue(result.User.Name)
	plan.Enabled = types.BoolValue(result.User.Enabled != nil && *result.User.Enabled)
	groups, d := stringSliceToList(ctx, result.User.Groups)
	resp.Diagnostics.Append(d...)
	plan.Groups = groups

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created IDP user", map[string]interface{}{"name": plan.Name.ValueString()})
}

func (r *IdpUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IdpUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading IDP user", map[string]interface{}{"name": state.Name.ValueString()})

	result, err := r.client.GetIdpUser(state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading IDP user", state.Name.ValueString(), err)...)
		return
	}

	state.ID = types.StringValue(result.User.Name)
	state.Name = types.StringValue(result.User.Name)
	state.Enabled = types.BoolValue(result.User.Enabled != nil && *result.User.Enabled)
	groups, d := stringSliceToList(ctx, result.User.Groups)
	resp.Diagnostics.Append(d...)
	state.Groups = groups

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *IdpUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state IdpUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating IDP user", map[string]interface{}{"name": state.Name.ValueString()})

	updateReq := &models.IdpUpdateUserRequest{}
	needsUpdate := false

	if !plan.Password.Equal(state.Password) && !plan.Password.IsNull() {
		updateReq.Password = plan.Password.ValueString()
		needsUpdate = true
	}
	if !plan.Enabled.Equal(state.Enabled) {
		enabled := plan.Enabled.ValueBool()
		updateReq.Enabled = &enabled
		needsUpdate = true
	}

	if needsUpdate {
		result, err := r.client.UpdateIdpUser(state.Name.ValueString(), updateReq)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating IDP user", state.Name.ValueString(), err)...)
			return
		}
		state.ID = types.StringValue(result.User.Name)
		state.Name = types.StringValue(result.User.Name)
		state.Enabled = types.BoolValue(result.User.Enabled != nil && *result.User.Enabled)
		groups, d := stringSliceToList(ctx, result.User.Groups)
		resp.Diagnostics.Append(d...)
		state.Groups = groups
	}

	state.Password = plan.Password
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)

	tflog.Debug(ctx, "Updated IDP user", map[string]interface{}{"name": state.Name.ValueString()})
}

func (r *IdpUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IdpUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting IDP user", map[string]interface{}{"name": state.Name.ValueString()})

	_, err := r.client.RemoveIdpUser(state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("deleting IDP user", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted IDP user", map[string]interface{}{"name": state.Name.ValueString()})
}

func (r *IdpUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func stringSliceToList(ctx context.Context, items []string) (types.Set, diag.Diagnostics) {
	vals := make([]attr.Value, 0, len(items))
	for _, s := range items {
		vals = append(vals, types.StringValue(s))
	}
	return types.SetValue(types.StringType, vals)
}
