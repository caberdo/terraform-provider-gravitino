package tag

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/providerbase"
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

var _ resource.Resource = &TagAssociationResource{}
var _ resource.ResourceWithImportState = &TagAssociationResource{}
var _ resource.ResourceWithConfigure = &TagAssociationResource{}

// TagAssociationResource attaches and detaches tags on one metadata object. The
// Gravitino API has no DELETE for an association: adding and removing tags both
// go through associateTags (POST with tagsToAdd / tagsToRemove), so destroy
// sends one POST that removes every tag recorded in state.
type TagAssociationResource struct {
	providerbase.Resource
}

func NewAssociation() resource.Resource {
	return &TagAssociationResource{}
}

type TagAssociationResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Metalake       types.String `tfsdk:"metalake"`
	ObjectType     types.String `tfsdk:"object_type"`
	ObjectFullName types.String `tfsdk:"object_full_name"`
	Tags           types.Set    `tfsdk:"tags"`
}

func (r *TagAssociationResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_tag_association"
}

func (r *TagAssociationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Attaches tags to a catalog, schema, table, view, column, fileset, topic, model or function in a metalake. " +
			"Only direct associations are managed: tags inherited from a parent object are ignored by Read and are never removed by destroy. " +
			"Gravitino exposes no DELETE for an association, so destroy sends one associateTags request with tagsToRemove for every tag in state.",
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
				Description: fmt.Sprintf("The metadata object type. One of: %s. VIEW requires Gravitino >= 1.3.1 and is rejected on an older server with an explicit diagnostic.",
					strings.Join(models.TagObjectTypes, ", ")),
				Validators: []validator.String{
					stringvalidator.OneOf(models.TagObjectTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"object_full_name": schema.StringAttribute{
				Required: true,
				Description: "The full name of the metadata object relative to the metalake, for example 'my_catalog' for a CATALOG, " +
					"'my_catalog.my_schema' for a SCHEMA, 'my_catalog.my_schema.my_table' for a TABLE and " +
					"'my_catalog.my_schema.my_table.my_column' for a COLUMN. Do not prefix it with the metalake name. Changing this forces the association to be recreated.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tags": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "The names of the tags to attach directly to the object. Adding and removing entries is applied in place through associateTags. " +
					"Set it to [] to detach every tag this resource manages while keeping the resource. Tags inherited from a parent object are not part of this set.",
			},
		},
	}
}

func (r *TagAssociationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TagAssociationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := plan.Metalake.ValueString()
	objectType := plan.ObjectType.ValueString()
	objectFullName := plan.ObjectFullName.ValueString()

	tflog.Debug(ctx, "Creating tag association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})

	if !r.checkObjectTypeSupported(ctx, objectType, &resp.Diagnostics) {
		return
	}

	names := tfutil.SetToStrings(plan.Tags)
	if len(names) > 0 {
		if _, err := r.Client.AssociateTags(ctx, metalake, objectType, objectFullName, &models.TagsAssociateRequest{TagsToAdd: names}); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("associating tags with", objectFullName, err)...)
			return
		}
	}

	resp.Diagnostics.Append(setTagAssociationState(&plan, metalake, names)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created tag association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})
}

func (r *TagAssociationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TagAssociationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := state.Metalake.ValueString()
	objectType := state.ObjectType.ValueString()
	objectFullName := state.ObjectFullName.ValueString()

	tflog.Debug(ctx, "Reading tag association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})

	if !r.checkObjectTypeSupported(ctx, objectType, &resp.Diagnostics) {
		return
	}

	result, err := r.Client.ListTagsForObject(ctx, metalake, objectType, objectFullName)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading tags of", objectFullName, err)...)
		return
	}

	names := make([]string, 0, len(result.Tags))
	for _, tag := range result.Tags {
		// Inherited tags cannot be detached at this object; another object owns
		// the association. Keeping them in state would make destroy try to
		// remove them and produce a perpetual diff.
		if tag.Inherited != nil && *tag.Inherited {
			continue
		}
		names = append(names, tag.Name)
	}

	resp.Diagnostics.Append(setTagAssociationState(&state, metalake, names)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)

	tflog.Debug(ctx, "Read tag association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})
}

func (r *TagAssociationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state TagAssociationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := plan.Metalake.ValueString()
	objectType := plan.ObjectType.ValueString()
	objectFullName := plan.ObjectFullName.ValueString()

	tflog.Debug(ctx, "Updating tag association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})

	if !r.checkObjectTypeSupported(ctx, objectType, &resp.Diagnostics) {
		return
	}

	current := tfutil.SetToStrings(state.Tags)
	desired := tfutil.SetToStrings(plan.Tags)
	toAdd, toRemove := resourceutil.DiffAssociationNames(current, desired)

	if len(toAdd) > 0 || len(toRemove) > 0 {
		assocReq := &models.TagsAssociateRequest{TagsToAdd: toAdd, TagsToRemove: toRemove}
		if _, err := r.Client.AssociateTags(ctx, metalake, objectType, objectFullName, assocReq); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating tags of", objectFullName, err)...)
			return
		}
	}

	resp.Diagnostics.Append(setTagAssociationState(&plan, metalake, desired)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated tag association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})
}

func (r *TagAssociationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TagAssociationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := state.Metalake.ValueString()
	objectType := state.ObjectType.ValueString()
	objectFullName := state.ObjectFullName.ValueString()

	tflog.Debug(ctx, "Deleting tag association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})

	names := tfutil.SetToStrings(state.Tags)
	if len(names) == 0 {
		tflog.Debug(ctx, "Deleted tag association", map[string]interface{}{
			"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
		})
		return
	}

	if _, err := r.Client.AssociateTags(ctx, metalake, objectType, objectFullName, &models.TagsAssociateRequest{TagsToRemove: names}); err != nil {
		if client.IsNotFoundError(err) {
			// The object (or one of its tags) is already gone, so the
			// association is gone with it.
			tflog.Debug(ctx, "Tag association already deleted", map[string]interface{}{
				"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
			})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting tags of", objectFullName, err)...)
		return
	}

	tflog.Debug(ctx, "Deleted tag association", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})
}

func (r *TagAssociationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, ok := resourceutil.SplitImportID(req, resp, 3, "metalake.object_type.object_full_name")
	if !ok {
		return
	}

	metalake, objectType, objectFullName := parts[0], parts[1], parts[2]
	if !slices.Contains(models.TagObjectTypes, objectType) {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Unknown object type %q in import ID %q; expected one of: %s.",
				objectType, req.ID, strings.Join(models.TagObjectTypes, ", ")),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), metalake)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_type"), objectType)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_full_name"), objectFullName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), resourceutil.AssociationID(metalake, objectType, objectFullName))...)
}

// checkObjectTypeSupported resolves the version-restricted VIEW object type
// before any request is sent, so a 1.3.0 server answers with an explicit
// "requires Gravitino >= 1.3.1" diagnostic instead of an opaque 400.
func (r *TagAssociationResource) checkObjectTypeSupported(ctx context.Context, objectType string, diags *diag.Diagnostics) bool {
	if err := r.Client.CheckMetadataObjectTypeSupported(ctx, objectType); err != nil {
		diags.AddAttributeError(path.Root("object_type"), "Unsupported object_type value", err.Error())
		return false
	}
	return true
}

func setTagAssociationState(model *TagAssociationResourceModel, metalake string, names []string) diag.Diagnostics {
	model.Metalake = types.StringValue(metalake)
	model.ID = types.StringValue(resourceutil.AssociationID(metalake, model.ObjectType.ValueString(), model.ObjectFullName.ValueString()))

	tags, diags := tfutil.StringsToSet(names)
	if !diags.HasError() {
		model.Tags = tags
	}
	return diags
}
