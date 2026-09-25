package role

import (
	"context"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &RoleResource{}
var _ resource.ResourceWithImportState = &RoleResource{}
var _ resource.ResourceWithConfigure = &RoleResource{}

type RoleResource struct {
	client *client.Client
}

func NewRoleResource() resource.Resource {
	return &RoleResource{}
}

func (r *RoleResource) SetClient(c *client.Client) {
	r.client = c
}

type RoleResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Metalake         types.String `tfsdk:"metalake"`
	Name             types.String `tfsdk:"name"`
	Properties       types.Map    `tfsdk:"properties"`
	SecurableObjects types.Set    `tfsdk:"securable_objects"`
	Audit            types.Object `tfsdk:"audit"`
}

type securableObjectModel struct {
	FullName   types.String `tfsdk:"full_name"`
	Type       types.String `tfsdk:"type"`
	Privileges types.Set    `tfsdk:"privileges"`
}

type privilegeModel struct {
	Name      types.String `tfsdk:"name"`
	Condition types.String `tfsdk:"condition"`
}

var PrivilegeAttrTypes = map[string]attr.Type{
	"name":      types.StringType,
	"condition": types.StringType,
}

var SecurableObjectAttrTypes = map[string]attr.Type{
	"full_name":  types.StringType,
	"type":       types.StringType,
	"privileges": types.SetType{ElemType: types.ObjectType{AttrTypes: PrivilegeAttrTypes}},
}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

func (r *RoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RoleResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_role"
}

func (r *RoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "Composite identifier in the format 'metalake.role_name'.",
			},
			"metalake": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "The metalake name. A role cannot be moved to another metalake, so changing it recreates the role.",
			},
			"name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "The role name. Gravitino has no role rename API, so changing it recreates the role.",
			},
			"properties": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					// Unconfigured properties are computed, which would make the planned
					// value unknown; keep the state value in that case so that an update
					// of the securable objects is not turned into a replacement.
					mapplanmodifier.UseStateForUnknown(),
					// Only a configured (and changed) value forces a replacement.
					mapplanmodifier.RequiresReplaceIfConfigured(),
				},
				Description: "A map of key-value properties for the role. Gravitino v1.3.0 only exposes PUT " +
					"/metalakes/{metalake}/permissions/roles/{role} (privilege override) for roles, so changing " +
					"the properties recreates the role.",
			},
			"securable_objects": schema.SetNestedAttribute{
				Optional: true,
				Computed: true,
				Description: "The securable objects and the privileges granted on them. The create request must contain this array " +
					"(Gravitino rejects a request without it), and it may be empty. Updates are applied with the privilege " +
					"override endpoint. `full_name` values are relative to the metalake: 'my_catalog' for a CATALOG, " +
					"'my_catalog.my_schema.my_table' for a TABLE, and the metalake name itself for a METALAKE.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"full_name": schema.StringAttribute{
							Required: true,
							Description: "The full name of the securable object, relative to the metalake (without the metalake " +
								"prefix): the metalake name itself for a METALAKE (e.g. 'my_metalake'), 'my_catalog' for a " +
								"CATALOG, 'my_catalog.my_schema' for a SCHEMA and 'my_catalog.my_schema.my_table' for a TABLE. " +
								"Gravitino rejects a metalake prefix on anything but a METALAKE with HTTP 400 " +
								"IllegalNamespaceException (e.g. 'my_metalake.my_catalog' for a CATALOG).",
						},
						"type": schema.StringAttribute{
							Required:    true,
							Description: "The type of the securable object: METALAKE, CATALOG, SCHEMA, TABLE, FILESET, TOPIC, ROLE, MODEL, FUNCTION, TAG, POLICY or JOB_TEMPLATE. Gravitino is case-insensitive on input but always serialises lower case, so this provider reports the canonical upper-case spelling in the state.",
							Validators: []validator.String{
								stringvalidator.OneOf(models.AllObjectTypes...),
							},
						},
						"privileges": schema.SetNestedAttribute{
							Required:    true,
							Description: "The privileges for the securable object. Gravitino requires at least one privilege per securable object.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Required:    true,
										Description: "The privilege name, e.g. SELECT_TABLE. Gravitino is case-insensitive on input but always serialises lower case, so this provider reports the canonical upper-case spelling in the state.",
										Validators: []validator.String{
											stringvalidator.OneOf(models.AllPrivileges...),
										},
									},
									"condition": schema.StringAttribute{
										Required:    true,
										Description: "The privilege condition, ALLOW or DENY. Gravitino always serialises it lower case; this provider reports the canonical upper-case spelling in the state.",
										Validators: []validator.String{
											stringvalidator.OneOf(models.PrivilegeConditionAllow, models.PrivilegeConditionDeny),
										},
									},
								},
							},
						},
					},
				},
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
				// No UseStateForUnknown: Gravitino rewrites audit on every update
				// (lastModifier/lastModifiedTime), and a planned-known value that the
				// provider then changes fails the apply with "inconsistent result".
				Description: "Audit information for the role.",
			},
		},
	}
}

func (r *RoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := plan.Metalake.ValueString()
	name := plan.Name.ValueString()

	tflog.Debug(ctx, "Creating role", map[string]interface{}{"metalake": metalake, "name": name})

	createReq := &models.RoleCreateRequest{
		Name:             name,
		Properties:       mapFromTF(plan.Properties),
		SecurableObjects: securableObjectsFromTF(ctx, plan.SecurableObjects),
	}

	result, err := r.client.CreateRole(ctx, metalake, createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating role", name, err)...)
		return
	}

	setStateFromRole(ctx, &resp.Diagnostics, metalake, &result.Role, &plan)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created role", map[string]interface{}{"metalake": metalake, "name": name})
}

func (r *RoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := state.Metalake.ValueString()
	name := state.Name.ValueString()

	tflog.Debug(ctx, "Reading role", map[string]interface{}{"metalake": metalake, "name": name})

	result, err := r.client.GetRole(ctx, metalake, name)
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Debug(ctx, "Role does not exist anymore, removing it from state", map[string]interface{}{"metalake": metalake, "name": name})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading role", name, err)...)
		return
	}

	setStateFromRole(ctx, &resp.Diagnostics, metalake, &result.Role, &state)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)

	tflog.Debug(ctx, "Read role", map[string]interface{}{"metalake": metalake, "name": name})
}

func (r *RoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state RoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := plan.Metalake.ValueString()
	roleName := plan.Name.ValueString()

	tflog.Debug(ctx, "Updating role", map[string]interface{}{"metalake": metalake, "name": roleName})

	// `metalake`, `name` and `properties` are RequiresReplace: the only mutable part
	// of a role is its list of securable objects with their privileges, which
	// Gravitino updates with PUT /metalakes/{metalake}/permissions/roles/{role}.
	if securableObjectsChanged(ctx, plan, state) {
		overrides := securableObjectsFromTF(ctx, plan.SecurableObjects)

		tflog.Debug(ctx, "Overriding role privileges", map[string]interface{}{"metalake": metalake, "name": roleName, "securable_objects": len(overrides)})

		if _, err := r.client.OverrideRolePrivileges(ctx, metalake, roleName, overrides); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating role privileges", roleName, err)...)
			return
		}
	}

	// Read back so that every computed attribute (audit included) holds a value that
	// is known to Terraform.
	result, err := r.client.GetRole(ctx, metalake, roleName)
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Debug(ctx, "Role does not exist anymore, removing it from state", map[string]interface{}{"metalake": metalake, "name": roleName})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading role after update", roleName, err)...)
		return
	}

	setStateFromRole(ctx, &resp.Diagnostics, metalake, &result.Role, &plan)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated role", map[string]interface{}{"metalake": metalake, "name": roleName})
}

func (r *RoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := state.Metalake.ValueString()
	name := state.Name.ValueString()

	tflog.Debug(ctx, "Deleting role", map[string]interface{}{"metalake": metalake, "name": name})

	_, err := r.client.DeleteRole(ctx, metalake, name)
	if err != nil {
		if client.IsNotFoundError(err) {
			// Already gone: deleting is idempotent.
			tflog.Debug(ctx, "Role already deleted", map[string]interface{}{"metalake": metalake, "name": name})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting role", name, err)...)
		return
	}

	tflog.Debug(ctx, "Deleted role", map[string]interface{}{"metalake": metalake, "name": name})
}

func (r *RoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tflog.Debug(ctx, "Importing role", map[string]interface{}{"id": req.ID})

	parts := strings.Split(req.ID, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected 'metalake.role_name', got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// setStateFromRole copies the API representation of a role into the model. role must
// not be nil: callers pass the response of a create, read or update call.
func setStateFromRole(ctx context.Context, diags *diag.Diagnostics, metalake string, role *models.Role, model *RoleResourceModel) {
	model.Metalake = types.StringValue(metalake)
	model.Name = types.StringValue(role.Name)
	model.ID = types.StringValue(metalake + "." + role.Name)

	props, d := types.MapValueFrom(ctx, types.StringType, role.Properties)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Properties = props

	secObjs, d := securableObjectsToTF(ctx, role.SecurableObjects)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.SecurableObjects = secObjs

	if role.Audit != nil {
		auditObj, d := auditToObjectValue(ctx, role.Audit)
		diags.Append(d...)
		if diags.HasError() {
			return
		}
		model.Audit = auditObj
		return
	}

	model.Audit = types.ObjectNull(AuditAttrTypes)
}

// securableObjectsToTF converts the API representation into the Terraform value. An
// empty list becomes an empty set, never a null set: the planned value of a
// configured (or previously stored) empty collection has to match the state.
func securableObjectsToTF(ctx context.Context, objects []models.SecurableObject) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics

	emptySet := func() types.Set {
		return types.SetValueMust(types.ObjectType{AttrTypes: SecurableObjectAttrTypes}, []attr.Value{})
	}

	items := make([]attr.Value, 0, len(objects))
	for _, obj := range objects {
		o := obj

		privItems := make([]attr.Value, 0, len(o.Privileges))
		for _, priv := range o.Privileges {
			p := priv
			privAttrs := map[string]attr.Value{
				"name":      types.StringValue(models.CanonicalPrivilege(p.Name)),
				"condition": types.StringValue(models.CanonicalPrivilege(p.Condition)),
			}
			privObj, d := types.ObjectValue(PrivilegeAttrTypes, privAttrs)
			diags.Append(d...)
			if diags.HasError() {
				return emptySet(), diags
			}
			privItems = append(privItems, privObj)
		}

		privSet, d := types.SetValue(types.ObjectType{AttrTypes: PrivilegeAttrTypes}, privItems)
		diags.Append(d...)
		if diags.HasError() {
			return emptySet(), diags
		}

		soAttrs := map[string]attr.Value{
			"full_name":  types.StringValue(o.FullName),
			"type":       types.StringValue(models.CanonicalObjectType(o.Type)),
			"privileges": privSet,
		}
		soObj, d := types.ObjectValue(SecurableObjectAttrTypes, soAttrs)
		diags.Append(d...)
		if diags.HasError() {
			return emptySet(), diags
		}
		items = append(items, soObj)
	}

	secObjs, d := types.SetValue(types.ObjectType{AttrTypes: SecurableObjectAttrTypes}, items)
	diags.Append(d...)
	if diags.HasError() {
		return emptySet(), diags
	}
	return secObjs, diags
}

// securableObjectsFromTF converts the Terraform value into the API representation. The
// returned slice is never nil, so the JSON body always contains the
// `securableObjects` / `overrides` array that the API requires.
func securableObjectsFromTF(ctx context.Context, l types.Set) []models.SecurableObject {
	result := make([]models.SecurableObject, 0)
	if l.IsNull() || l.IsUnknown() {
		return result
	}
	for _, v := range l.Elements() {
		objVal, ok := v.(types.Object)
		if !ok {
			continue
		}
		var so securableObjectModel
		if d := objVal.As(ctx, &so, basetypes.ObjectAsOptions{}); d.HasError() {
			continue
		}
		result = append(result, models.SecurableObject{
			FullName:   so.FullName.ValueString(),
			Type:       models.CanonicalObjectType(so.Type.ValueString()),
			Privileges: privilegesFromTF(ctx, so.Privileges),
		})
	}
	return result
}

func privilegesFromTF(ctx context.Context, l types.Set) []models.Privilege {
	privileges := make([]models.Privilege, 0)
	if l.IsNull() || l.IsUnknown() {
		return privileges
	}
	for _, pv := range l.Elements() {
		prv, ok := pv.(types.Object)
		if !ok {
			continue
		}
		var pm privilegeModel
		if d := prv.As(ctx, &pm, basetypes.ObjectAsOptions{}); d.HasError() {
			continue
		}
		privileges = append(privileges, models.Privilege{
			Name:      pm.Name.ValueString(),
			Condition: pm.Condition.ValueString(),
		})
	}
	return privileges
}

// securableObjectsChanged reports whether the privileges stored in the state differ
// from the ones the plan asks for. The comparison is set based: both Terraform sets and
// the Gravitino API treat the securable objects and privileges as unordered.
func securableObjectsChanged(ctx context.Context, plan, state RoleResourceModel) bool {
	if plan.SecurableObjects.IsUnknown() {
		// Nothing to compare yet; the value is computed by the server.
		return false
	}

	planned := securableObjectMapFromTF(ctx, plan.SecurableObjects)
	current := securableObjectMapFromTF(ctx, state.SecurableObjects)

	if len(planned) != len(current) {
		return true
	}

	for key, pso := range planned {
		sso, exists := current[key]
		if !exists || !privilegesEqual(pso.Privileges, sso.Privileges) {
			return true
		}
	}

	return false
}

type securableObjectMapEntry struct {
	FullName   string
	Type       string
	Privileges []models.Privilege
}

func securableObjectMapFromTF(ctx context.Context, l types.Set) map[string]securableObjectMapEntry {
	result := make(map[string]securableObjectMapEntry)
	if l.IsNull() || l.IsUnknown() {
		return result
	}
	for _, v := range l.Elements() {
		objVal, ok := v.(types.Object)
		if !ok {
			continue
		}
		var so securableObjectModel
		if d := objVal.As(ctx, &so, basetypes.ObjectAsOptions{}); d.HasError() {
			continue
		}
		key := so.FullName.ValueString() + "/" + models.CanonicalObjectType(so.Type.ValueString())
		result[key] = securableObjectMapEntry{
			FullName:   so.FullName.ValueString(),
			Type:       models.CanonicalObjectType(so.Type.ValueString()),
			Privileges: privilegesFromTF(ctx, so.Privileges),
		}
	}
	return result
}

// privilegesEqual compares two privilege lists as multisets.
func privilegesEqual(a, b []models.Privilege) bool {
	if len(a) != len(b) {
		return false
	}

	counts := make(map[models.Privilege]int, len(a))
	for _, p := range a {
		counts[p]++
	}
	for _, p := range b {
		if counts[p] == 0 {
			return false
		}
		counts[p]--
	}
	return true
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

func mapFromTF(m types.Map) map[string]string {
	result := make(map[string]string)
	if m.IsNull() || m.IsUnknown() {
		return result
	}
	for k, v := range m.Elements() {
		if strVal, ok := v.(types.String); ok {
			result[k] = strVal.ValueString()
		}
	}
	return result
}
