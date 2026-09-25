package owner

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &OwnerResource{}
var _ resource.ResourceWithImportState = &OwnerResource{}
var _ resource.ResourceWithConfigure = &OwnerResource{}

type OwnerResource struct {
	client *client.Client
}

func NewOwnerResource() resource.Resource {
	return &OwnerResource{}
}

func (r *OwnerResource) SetClient(c *client.Client) {
	r.client = c
}

type OwnerResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Metalake       types.String `tfsdk:"metalake"`
	ObjectType     types.String `tfsdk:"object_type"`
	ObjectFullName types.String `tfsdk:"object_full_name"`
	OwnerName      types.String `tfsdk:"owner_name"`
	OwnerType      types.String `tfsdk:"owner_type"`
}

func (r *OwnerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OwnerResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_owner"
}

func (r *OwnerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "Composite identifier in the format 'metalake.object_type.object_full_name'.",
			},
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"object_type": schema.StringAttribute{
				Required:    true,
				Description: "The metadata object type. One of: METALAKE, CATALOG, SCHEMA, TABLE, FILESET, TOPIC, ROLE (upper case singular, as required by the API path). Changing it forces a new owner binding: the Gravitino API only has PUT /owners/{type}/{fullName}, it cannot move an owner between objects.",
				Validators: []validator.String{
					stringvalidator.OneOf(models.OwnerObjectTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"object_full_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the metadata object, relative to the metalake (the API rejects a metalake prefix with HTTP 400 IllegalNamespaceException): METALAKE = the metalake name, CATALOG = the catalog name, SCHEMA = 'catalog.schema', TABLE = 'catalog.schema.table', and likewise for FILESET, TOPIC and ROLE. Changing it forces a new owner binding: the Gravitino API only has PUT /owners/{type}/{fullName}, it cannot move an owner between objects.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"owner_name": schema.StringAttribute{
				Required:    true,
				Description: "The owner name.",
			},
			"owner_type": schema.StringAttribute{
				Required:    true,
				Description: "The owner type (USER or GROUP). Gravitino matches this case-insensitively and always answers in lowercase; the provider normalises responses back to upper case.",
				Validators: []validator.String{
					stringvalidator.OneOf(models.OwnerTypeUser, models.OwnerTypeGroup),
				},
			},
		},
	}
}

func (r *OwnerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OwnerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating owner", map[string]interface{}{
		"metalake":         plan.Metalake.ValueString(),
		"object_type":      plan.ObjectType.ValueString(),
		"object_full_name": plan.ObjectFullName.ValueString(),
	})

	createReq := &models.SetOwnerRequest{
		Name: plan.OwnerName.ValueString(),
		Type: plan.OwnerType.ValueString(),
	}

	_, err := r.client.SetOwner(ctx,
		plan.Metalake.ValueString(),
		plan.ObjectType.ValueString(),
		plan.ObjectFullName.ValueString(),
		createReq,
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("setting owner", plan.ObjectFullName.ValueString(), err)...)
		return
	}

	plan.ID = types.StringValue(
		plan.Metalake.ValueString() + "." + plan.ObjectType.ValueString() + "." + plan.ObjectFullName.ValueString(),
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created owner", map[string]interface{}{
		"metalake":         plan.Metalake.ValueString(),
		"object_full_name": plan.ObjectFullName.ValueString(),
	})
}

func (r *OwnerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OwnerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading owner", map[string]interface{}{
		"metalake":         state.Metalake.ValueString(),
		"object_type":      state.ObjectType.ValueString(),
		"object_full_name": state.ObjectFullName.ValueString(),
	})

	result, err := r.client.GetOwner(ctx,
		state.Metalake.ValueString(),
		state.ObjectType.ValueString(),
		state.ObjectFullName.ValueString(),
	)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading owner", state.ObjectFullName.ValueString(), err)...)
		return
	}

	state.OwnerName = types.StringValue(result.Owner.Name)
	// Gravitino echoes the owner type in lowercase ("user"/"group") even though
	// the canonical API enum (and this provider's configuration) is uppercase.
	// Normalise it, otherwise the applied state would differ from the config and
	// Terraform would fail with "Provider produced inconsistent result".
	state.OwnerType = types.StringValue(models.NormalizeOwnerType(result.Owner.Type))

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *OwnerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan OwnerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating owner", map[string]interface{}{
		"metalake":         plan.Metalake.ValueString(),
		"object_type":      plan.ObjectType.ValueString(),
		"object_full_name": plan.ObjectFullName.ValueString(),
	})

	updateReq := &models.SetOwnerRequest{
		Name: plan.OwnerName.ValueString(),
		Type: plan.OwnerType.ValueString(),
	}

	_, err := r.client.SetOwner(ctx,
		plan.Metalake.ValueString(),
		plan.ObjectType.ValueString(),
		plan.ObjectFullName.ValueString(),
		updateReq,
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("updating owner", plan.ObjectFullName.ValueString(), err)...)
		return
	}

	plan.ID = types.StringValue(
		plan.Metalake.ValueString() + "." + plan.ObjectType.ValueString() + "." + plan.ObjectFullName.ValueString(),
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated owner", map[string]interface{}{
		"metalake":         plan.Metalake.ValueString(),
		"object_full_name": plan.ObjectFullName.ValueString(),
	})
}

func (r *OwnerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	tflog.Debug(ctx, "Deleting owner (removing from state only)")
	resp.State.RemoveResource(ctx)
}

func (r *OwnerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ".", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected 'metalake.object_type.object_full_name', got: %s", req.ID),
		)
		return
	}

	metalake := parts[0]
	objectType := parts[1]
	objectFullName := parts[2]

	if !slices.Contains(models.OwnerObjectTypes, objectType) {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Unknown object type %q in import ID %q; expected one of: %s", objectType, req.ID, strings.Join(models.OwnerObjectTypes, ", ")),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), metalake)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_type"), objectType)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_full_name"), objectFullName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
