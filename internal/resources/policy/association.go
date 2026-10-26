package policy

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/resources/resourceutil"
	"github.com/gravitino/terraform-provider-gravitino/internal/tfutil"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &PolicyAssociationResource{}
var _ resource.ResourceWithImportState = &PolicyAssociationResource{}
var _ resource.ResourceWithConfigure = &PolicyAssociationResource{}

// PolicyAssociationResource attaches and detaches policies on one metadata
// object. Gravitino has no DELETE for an association: adding and removing
// policies both go through associatePolicies (POST with policiesToAdd /
// policiesToRemove), so destroy sends one POST that removes every policy
// recorded in state.
type PolicyAssociationResource struct {
	client *client.Client
}

func NewAssociation() resource.Resource {
	return &PolicyAssociationResource{}
}

func (r *PolicyAssociationResource) SetClient(c *client.Client) {
	r.client = c
}

type PolicyAssociationResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Metalake       types.String `tfsdk:"metalake"`
	ObjectType     types.String `tfsdk:"object_type"`
	ObjectFullName types.String `tfsdk:"object_full_name"`
	Policies       types.Set    `tfsdk:"policies"`
}

func (r *PolicyAssociationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func (r *PolicyAssociationResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_policy_association"
}

func (r *PolicyAssociationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Attaches policies to a catalog, schema, table, view, fileset, topic, model or function in a metalake. " +
			"Only direct associations are managed: policies inherited from a parent object are ignored by Read and are never removed by destroy. " +
			"Gravitino exposes no DELETE for an association, so destroy sends one associatePolicies request with policiesToRemove for every policy in state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					models.CompoundID("metalake", "object_type", "object_full_name"),
				},
				Description: "Composite identifier in the format 'metalake.object_type.object_full_name'.",
			},
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name. Changing this forces the association to be recreated.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"object_type": schema.StringAttribute{
				Required: true,
				Description: fmt.Sprintf("The metadata object type. One of: %s. VIEW and FUNCTION require Gravitino >= 1.3.1 and are rejected on an older server with an explicit diagnostic. "+
					"Columns cannot carry a policy.", strings.Join(models.PolicyObjectTypes, ", ")),
				Validators: []validator.String{
					stringvalidator.OneOf(models.PolicyObjectTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"object_full_name": schema.StringAttribute{
				Required: true,
				Description: "The full name of the metadata object relative to the metalake, for example 'my_catalog' for a CATALOG, " +
					"'my_catalog.my_schema' for a SCHEMA and 'my_catalog.my_schema.my_table' for a TABLE. Do not prefix it with the metalake name. " +
					"Changing this forces the association to be recreated.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"policies": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "The names of the policies to attach directly to the object. Adding and removing entries is applied in place through associatePolicies. " +
					"Each policy must list this object_type in its supported_object_types; a mismatch is rejected before the request is sent, naming the allowed types. " +
					"Set it to [] to detach every policy this resource manages while keeping the resource. Policies inherited from a parent object are not part of this set.",
			},
		},
	}
}

func (r *PolicyAssociationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PolicyAssociationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := plan.Metalake.ValueString()
	objectType := plan.ObjectType.ValueString()
	objectFullName := plan.ObjectFullName.ValueString()

	tflog.Debug(ctx, "Creating policy association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})

	if !r.checkObjectTypeSupported(ctx, objectType, &resp.Diagnostics) {
		return
	}

	names := tfutil.SetToStrings(plan.Policies)
	if !r.checkPoliciesSupportObjectType(ctx, metalake, objectType, names, &resp.Diagnostics) {
		return
	}

	if len(names) > 0 {
		if _, err := r.client.AssociatePolicies(ctx, metalake, objectType, objectFullName, &models.PolicyAssociationRequest{PoliciesToAdd: names}); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("associating policies with", objectFullName, err)...)
			return
		}
	}

	resp.Diagnostics.Append(setPolicyAssociationState(&plan, metalake, names)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created policy association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})
}

func (r *PolicyAssociationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PolicyAssociationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := state.Metalake.ValueString()
	objectType := state.ObjectType.ValueString()
	objectFullName := state.ObjectFullName.ValueString()

	tflog.Debug(ctx, "Reading policy association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})

	if !r.checkObjectTypeSupported(ctx, objectType, &resp.Diagnostics) {
		return
	}

	result, err := r.client.ListPoliciesForObject(ctx, metalake, objectType, objectFullName)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading policies of", objectFullName, err)...)
		return
	}

	names := make([]string, 0, len(result.Policies))
	for _, policy := range result.Policies {
		// Inherited policies cannot be detached at this object; another object
		// owns the association. Keeping them in state would make destroy try to
		// remove them and produce a perpetual diff.
		if policy.Inherited != nil && *policy.Inherited {
			continue
		}
		names = append(names, policy.Name)
	}

	resp.Diagnostics.Append(setPolicyAssociationState(&state, metalake, names)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)

	tflog.Debug(ctx, "Read policy association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})
}

func (r *PolicyAssociationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state PolicyAssociationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := plan.Metalake.ValueString()
	objectType := plan.ObjectType.ValueString()
	objectFullName := plan.ObjectFullName.ValueString()

	tflog.Debug(ctx, "Updating policy association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})

	if !r.checkObjectTypeSupported(ctx, objectType, &resp.Diagnostics) {
		return
	}

	current := tfutil.SetToStrings(state.Policies)
	desired := tfutil.SetToStrings(plan.Policies)
	toAdd, toRemove := resourceutil.DiffAssociationNames(current, desired)

	if !r.checkPoliciesSupportObjectType(ctx, metalake, objectType, toAdd, &resp.Diagnostics) {
		return
	}

	if len(toAdd) > 0 || len(toRemove) > 0 {
		assocReq := &models.PolicyAssociationRequest{PoliciesToAdd: toAdd, PoliciesToRemove: toRemove}
		if _, err := r.client.AssociatePolicies(ctx, metalake, objectType, objectFullName, assocReq); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating policies of", objectFullName, err)...)
			return
		}
	}

	resp.Diagnostics.Append(setPolicyAssociationState(&plan, metalake, desired)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated policy association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})
}

func (r *PolicyAssociationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PolicyAssociationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := state.Metalake.ValueString()
	objectType := state.ObjectType.ValueString()
	objectFullName := state.ObjectFullName.ValueString()

	tflog.Debug(ctx, "Deleting policy association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})

	names := tfutil.SetToStrings(state.Policies)
	if len(names) == 0 {
		tflog.Debug(ctx, "Deleted policy association", map[string]interface{}{
			"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
		})
		return
	}

	if _, err := r.client.AssociatePolicies(ctx, metalake, objectType, objectFullName, &models.PolicyAssociationRequest{PoliciesToRemove: names}); err != nil {
		if client.IsNotFoundError(err) {
			// The object (or one of its policies) is already gone, so the
			// association is gone with it.
			tflog.Debug(ctx, "Policy association already deleted", map[string]interface{}{
				"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
			})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting policies of", objectFullName, err)...)
		return
	}

	tflog.Debug(ctx, "Deleted policy association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})
}

func (r *PolicyAssociationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, ok := resourceutil.SplitImportID(req, resp, 3, "metalake.object_type.object_full_name")
	if !ok {
		return
	}

	metalake, objectType, objectFullName := parts[0], parts[1], parts[2]
	if !slices.Contains(models.PolicyObjectTypes, objectType) {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Unknown object type %q in import ID %q; expected one of: %s.",
				objectType, req.ID, strings.Join(models.PolicyObjectTypes, ", ")),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), metalake)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_type"), objectType)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_full_name"), objectFullName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), resourceutil.AssociationID(metalake, objectType, objectFullName))...)
}

// checkObjectTypeSupported resolves the version-restricted VIEW and FUNCTION
// object types before any request is sent, so a 1.3.0 server answers with an
// explicit "requires Gravitino >= 1.3.1" diagnostic instead of an opaque 400.
func (r *PolicyAssociationResource) checkObjectTypeSupported(ctx context.Context, objectType string, diags *diag.Diagnostics) bool {
	if err := r.client.CheckMetadataObjectTypeSupported(ctx, objectType); err != nil {
		diags.AddAttributeError(path.Root("object_type"), "Unsupported object_type value", err.Error())
		return false
	}
	return true
}

// checkPoliciesSupportObjectType verifies that every policy being attached
// lists objectType in its supported_object_types. The API otherwise rejects the
// whole association request with an opaque 400, so the provider reads each
// policy first and names the policy and the types it allows. A policy whose
// content is not visible (built-in content shapes, or no content) is sent
// through unchanged: the provider has nothing to compare.
func (r *PolicyAssociationResource) checkPoliciesSupportObjectType(ctx context.Context, metalake, objectType string, names []string, diags *diag.Diagnostics) bool {
	if len(names) == 0 {
		return true
	}

	objectType = models.CanonicalObjectType(objectType)
	var mismatches []string

	for _, name := range names {
		result, err := r.client.GetPolicy(ctx, metalake, name)
		if err != nil {
			if client.IsNotFoundError(err) {
				diags.AddAttributeError(
					path.Root("policies"),
					"Policy not found",
					fmt.Sprintf("No policy %q exists in metalake %q; create it before associating it with a %s.",
						name, metalake, objectType),
				)
				return false
			}
			diags.Append(client.NewResourceError("reading policy", name, err)...)
			return false
		}

		allowed := policySupportedObjectTypes(&result.Policy)
		if len(allowed) == 0 || slices.Contains(allowed, objectType) {
			continue
		}
		mismatches = append(mismatches, fmt.Sprintf("policy %q supports %s", name, strings.Join(allowed, ", ")))
	}

	if len(mismatches) > 0 {
		diags.AddAttributeError(
			path.Root("policies"),
			"Policy does not support the object type",
			fmt.Sprintf("Object type %s is not listed in the supported_object_types of %s. Attach a policy that supports %s, or change object_type.",
				objectType, strings.Join(mismatches, "; "), objectType),
		)
		return false
	}

	return true
}

func policySupportedObjectTypes(policy *models.Policy) []string {
	if policy == nil || policy.Content == nil {
		return nil
	}
	return models.NormalizePolicyObjectTypes(policy.Content.SupportedObjectTypes)
}

func setPolicyAssociationState(model *PolicyAssociationResourceModel, metalake string, names []string) diag.Diagnostics {
	model.Metalake = types.StringValue(metalake)
	model.ID = types.StringValue(resourceutil.AssociationID(metalake, model.ObjectType.ValueString(), model.ObjectFullName.ValueString()))

	policies, diags := tfutil.StringsToSet(names)
	if !diags.HasError() {
		model.Policies = policies
	}
	return diags
}
