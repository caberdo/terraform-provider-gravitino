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
	SecurableObjects types.List   `tfsdk:"securable_objects"`
	Audit            types.Object `tfsdk:"audit"`
}

type securableObjectModel struct {
	FullName   types.String `tfsdk:"full_name"`
	Type       types.String `tfsdk:"type"`
	Privileges types.List   `tfsdk:"privileges"`
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
	"privileges": types.ListType{ElemType: types.ObjectType{AttrTypes: PrivilegeAttrTypes}},
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
				Required:    true,
				Description: "The metalake name.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The role name.",
			},
			"properties": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "A map of key-value properties for the role.",
			},
			"securable_objects": schema.ListNestedAttribute{
				Optional:    true,
				Description: "The securable objects and their privileges assigned to the role.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"full_name": schema.StringAttribute{
							Required:    true,
							Description: "The full name of the securable object.",
						},
						"type": schema.StringAttribute{
							Required:    true,
							Description: "The type of the securable object.",
							Validators: []validator.String{
								stringvalidator.OneOf(models.AllObjectTypes...),
							},
						},
						"privileges": schema.ListNestedAttribute{
							Required:    true,
							Description: "The privileges for the securable object.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Required:    true,
										Description: "The privilege name.",
										Validators: []validator.String{
											stringvalidator.OneOf(models.AllPrivileges...),
										},
									},
									"condition": schema.StringAttribute{
										Required:    true,
										Description: "The privilege condition.",
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
				Description:    "Audit information for the role.",
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

	tflog.Debug(ctx, "Creating role", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})

	createReq := &models.RoleCreateRequest{
		Name:             plan.Name.ValueString(),
		Properties:       mapFromTF(plan.Properties),
		SecurableObjects: securableObjectsFromTF(ctx, plan.SecurableObjects),
	}

	result, err := r.client.CreateRole(plan.Metalake.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating role", plan.Name.ValueString(), err)...)
		return
	}

	setStateFromRole(ctx, &resp.Diagnostics, plan.Metalake.ValueString(), &result.Role, &plan)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created role", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})
}

func (r *RoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading role", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	result, err := r.client.GetRole(state.Metalake.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading role", state.Name.ValueString(), err)...)
		return
	}

	setStateFromRole(ctx, &resp.Diagnostics, state.Metalake.ValueString(), &result.Role, &state)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *RoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state RoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := plan.Metalake.ValueString()
	roleName := state.Name.ValueString()

	tflog.Debug(ctx, "Updating role", map[string]interface{}{"metalake": metalake, "name": roleName})

	changed := false

	oldProps := mapFromTF(state.Properties)
	newProps := mapFromTF(plan.Properties)

	for k, v := range newProps {
		oldV, exists := oldProps[k]
		if !exists || oldV != v {
			changed = true
		}
	}
	for k := range oldProps {
		if _, exists := newProps[k]; !exists {
			changed = true
		}
	}

	planSecurables := securableObjectMapFromTF(ctx, plan.SecurableObjects)
	stateSecurables := securableObjectMapFromTF(ctx, state.SecurableObjects)

	securablesChanged := false
	for key, pso := range planSecurables {
		sso, exists := stateSecurables[key]
		if !exists || !privilegesEqual(pso.Privileges, sso.Privileges) {
			securablesChanged = true
			break
		}
	}
	if !securablesChanged {
		for key := range stateSecurables {
			if _, exists := planSecurables[key]; !exists {
				securablesChanged = true
				break
			}
		}
	}

	if securablesChanged {
		overrides := securableObjectsFromTF(ctx, plan.SecurableObjects)
		if _, err := r.client.OverrideRolePrivileges(metalake, roleName, overrides); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating role privileges", roleName, err)...)
			return
		}
		changed = true
	}

	if !plan.Name.Equal(state.Name) {
		resp.Diagnostics.AddError(
			"Role name update not supported",
			"Renaming a role is not supported. The resource must be recreated.",
		)
		return
	}

	if changed {
		result, err := r.client.GetRole(metalake, roleName)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("reading role after update", roleName, err)...)
			return
		}
		setStateFromRole(ctx, &resp.Diagnostics, metalake, &result.Role, &plan)
	} else {
		setStateFromRole(ctx, &resp.Diagnostics, metalake, nil, &plan)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated role", map[string]interface{}{"metalake": metalake, "name": plan.Name.ValueString()})
}

func (r *RoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting role", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	_, err := r.client.DeleteRole(state.Metalake.ValueString(), state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("deleting role", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted role", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})
}

func (r *RoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idx := strings.LastIndex(req.ID, ".")
	if idx == -1 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected 'metalake.role_name', got: %s", req.ID),
		)
		return
	}

	metalake := req.ID[:idx]
	name := req.ID[idx+1:]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), metalake)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func setStateFromRole(ctx context.Context, diags *diag.Diagnostics, metalake string, role *models.RoleDetail, model *RoleResourceModel) {
	if role != nil {
		model.Name = types.StringValue(role.Name)
		model.ID = types.StringValue(metalake + "." + role.Name)

		props, d := types.MapValueFrom(ctx, types.StringType, role.Properties)
		diags.Append(d...)
		if !diags.HasError() {
			model.Properties = props
		}

		secObjs, d := securableObjectsToTF(ctx, role.SecurableObjects)
		diags.Append(d...)
		if !diags.HasError() {
			model.SecurableObjects = secObjs
		}

		if role.Audit != nil {
			auditObj, d := auditToObjectValue(ctx, role.Audit)
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

func securableObjectsToTF(ctx context.Context, objects []models.SecurableObject) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics

	if len(objects) == 0 {
		return types.ListNull(types.ObjectType{AttrTypes: SecurableObjectAttrTypes}), diags
	}

	items := make([]attr.Value, 0, len(objects))
	for _, obj := range objects {
		o := obj

		privItems := make([]attr.Value, 0, len(o.Privileges))
		for _, priv := range o.Privileges {
			p := priv
			privAttrs := map[string]attr.Value{
				"name":      types.StringValue(strings.ToUpper(p.Name)),
				"condition": types.StringValue(strings.ToUpper(p.Condition)),
			}
			privObj, d := types.ObjectValue(PrivilegeAttrTypes, privAttrs)
			diags.Append(d...)
			if diags.HasError() {
				return types.ListNull(types.ObjectType{AttrTypes: SecurableObjectAttrTypes}), diags
			}
			privItems = append(privItems, privObj)
		}

		privList, d := types.ListValue(types.ObjectType{AttrTypes: PrivilegeAttrTypes}, privItems)
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(types.ObjectType{AttrTypes: SecurableObjectAttrTypes}), diags
		}

		soAttrs := map[string]attr.Value{
			"full_name":  types.StringValue(o.FullName),
			"type":       types.StringValue(strings.ToUpper(o.Type)),
			"privileges": privList,
		}
		soObj, d := types.ObjectValue(SecurableObjectAttrTypes, soAttrs)
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(types.ObjectType{AttrTypes: SecurableObjectAttrTypes}), diags
		}
		items = append(items, soObj)
	}

	return types.ListValue(types.ObjectType{AttrTypes: SecurableObjectAttrTypes}, items)
}

func securableObjectsFromTF(ctx context.Context, l types.List) []models.SecurableObject {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var result []models.SecurableObject
	for _, v := range l.Elements() {
		if objVal, ok := v.(types.Object); ok {
			so := securableObjectModel{}
			if d := objVal.As(ctx, &so, basetypes.ObjectAsOptions{}); d.HasError() {
				continue
			}
			var privileges []models.Privilege
			if !so.Privileges.IsNull() && !so.Privileges.IsUnknown() {
				for _, pv := range so.Privileges.Elements() {
					if prv, ok := pv.(types.Object); ok {
						var pm privilegeModel
						if d := prv.As(ctx, &pm, basetypes.ObjectAsOptions{}); d.HasError() {
							continue
						}
						privileges = append(privileges, models.Privilege{
							Name:      pm.Name.ValueString(),
							Condition: pm.Condition.ValueString(),
						})
					}
				}
			}
			result = append(result, models.SecurableObject{
				FullName:   so.FullName.ValueString(),
				Type:       so.Type.ValueString(),
				Privileges: privileges,
			})
		}
	}
	return result
}

type securableObjectMapEntry struct {
	FullName   string
	Type       string
	Privileges []models.Privilege
}

func securableObjectMapFromTF(ctx context.Context, l types.List) map[string]securableObjectMapEntry {
	result := make(map[string]securableObjectMapEntry)
	if l.IsNull() || l.IsUnknown() {
		return result
	}
	for _, v := range l.Elements() {
		if objVal, ok := v.(types.Object); ok {
			so := securableObjectModel{}
			if d := objVal.As(ctx, &so, basetypes.ObjectAsOptions{}); d.HasError() {
				continue
			}
			var privileges []models.Privilege
			if !so.Privileges.IsNull() && !so.Privileges.IsUnknown() {
				for _, pv := range so.Privileges.Elements() {
					if prv, ok := pv.(types.Object); ok {
						var pm privilegeModel
						if d := prv.As(ctx, &pm, basetypes.ObjectAsOptions{}); d.HasError() {
							continue
						}
						privileges = append(privileges, models.Privilege{
							Name:      pm.Name.ValueString(),
							Condition: pm.Condition.ValueString(),
						})
					}
				}
			}
			key := so.FullName.ValueString() + "/" + so.Type.ValueString()
			result[key] = securableObjectMapEntry{
				FullName:   so.FullName.ValueString(),
				Type:       so.Type.ValueString(),
				Privileges: privileges,
			}
		}
	}
	return result
}

func privilegesEqual(a, b []models.Privilege) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Condition != b[i].Condition {
			return false
		}
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
