package idp_user

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

var _ resource.Resource = &IdpUserResource{}
var _ resource.ResourceWithImportState = &IdpUserResource{}
var _ resource.ResourceWithConfigure = &IdpUserResource{}

// IdpUserResourceDescription is the schema description shown in the generated docs.
// The built-in IDP REST API is not part of a default Gravitino 1.3.0 server:
// it needs `gravitino.authenticators = basic` (without `simple`),
// `gravitino.server.rest.extensionPackages = org.apache.gravitino.idp.web.rest.feature`
// and a service admin from `gravitino.authorization.serviceAdmins`. Without
// that configuration every IDP endpoint answers HTTP 404.
const IdpUserResourceDescription = "Manages a built-in IDP user for local authentication; the built-in IDP REST API needs " +
	"`gravitino.authenticators = basic` (without `simple`) and " +
	"`gravitino.server.rest.extensionPackages = org.apache.gravitino.idp.web.rest.feature`, " +
	"and calls must come from a `gravitino.authorization.serviceAdmins` service admin, " +
	"otherwise the endpoint answers HTTP 404."

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
		Description: IdpUserResourceDescription,
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
				PlanModifiers: []planmodifier.String{
					// PUT /idp/users/{user} only changes the password, so a
					// renamed user has to be recreated.
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "The password (12-64 characters). Required by the API and never returned by it.",
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

	createReq := &models.IdpAddUserRequest{
		User:     plan.Name.ValueString(),
		Password: plan.Password.ValueString(),
	}

	result, err := r.client.AddIdpUser(ctx, createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating IDP user", plan.Name.ValueString(), err)...)
		return
	}

	plan.ID = types.StringValue(result.User.Name)
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

	result, err := r.client.GetIdpUser(ctx, state.Name.ValueString())
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
	groups, d := stringSliceToList(ctx, result.User.Groups)
	resp.Diagnostics.Append(d...)
	state.Groups = groups
	// The password is write-only: it is kept from state and never refreshed.

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

	// `name` forces replacement, so the only in-place change is the password,
	// which is exactly what PUT /idp/users/{user} accepts.
	if !plan.Password.Equal(state.Password) {
		result, err := r.client.ChangeIdpUserPassword(ctx, state.Name.ValueString(), &models.IdpChangePasswordRequest{
			Password: plan.Password.ValueString(),
		})
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating IDP user", state.Name.ValueString(), err)...)
			return
		}

		state.ID = types.StringValue(result.User.Name)
		state.Name = types.StringValue(result.User.Name)
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

	_, err := r.client.RemoveIdpUser(ctx, state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			// Already gone: deleting twice is not an error.
			tflog.Debug(ctx, "IDP user already deleted", map[string]interface{}{"name": state.Name.ValueString()})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting IDP user", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted IDP user", map[string]interface{}{"name": state.Name.ValueString()})
}

func (r *IdpUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// ImportStatePassthroughID ignores an empty identifier silently, which
	// would leave an import without a name: reject it explicitly instead.
	if req.ID == "" || strings.Contains(req.ID, ":") {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected the username of an existing built-in IDP user (not containing ':'), got: %q", req.ID),
		)
		return
	}

	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func stringSliceToList(ctx context.Context, items []string) (types.Set, diag.Diagnostics) {
	vals := make([]attr.Value, 0, len(items))
	for _, s := range items {
		vals = append(vals, types.StringValue(s))
	}
	return types.SetValue(types.StringType, vals)
}
