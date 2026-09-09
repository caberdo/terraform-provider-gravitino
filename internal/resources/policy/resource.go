package policy

import (
	"context"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &PolicyResource{}
var _ resource.ResourceWithImportState = &PolicyResource{}
var _ resource.ResourceWithConfigure = &PolicyResource{}

type PolicyResource struct {
	client *client.Client
}

func New() resource.Resource {
	return &PolicyResource{}
}

func (r *PolicyResource) SetClient(c *client.Client) {
	r.client = c
}

type PolicyResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	Metalake             types.String `tfsdk:"metalake"`
	Name                 types.String `tfsdk:"name"`
	Comment              types.String `tfsdk:"comment"`
	PolicyType           types.String `tfsdk:"policy_type"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	SupportedObjectTypes types.Set    `tfsdk:"supported_object_types"`
	Properties           types.Map    `tfsdk:"properties"`
	CustomRules          types.Map    `tfsdk:"custom_rules"`
	Audit                types.Object `tfsdk:"audit"`
}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

func (r *PolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PolicyResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_policy"
}

func (r *PolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "Composite identifier in the format 'metalake.policy_name'.",
			},
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The policy name.",
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "A comment or description for the policy.",
			},
			"policy_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("custom"),
				Description: "The policy type. Only 'custom' is currently supported.",
				Validators: []validator.String{
					stringvalidator.OneOf("custom"),
				},
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether the policy is enabled.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"supported_object_types": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "The object types this policy supports. One or more of: CATALOG, SCHEMA, TABLE, FILESET, TOPIC, MODEL.",
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.OneOf("CATALOG", "SCHEMA", "TABLE", "FILESET", "TOPIC", "MODEL"),
					),
				},
			},
			"properties": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "A map of key-value properties for the policy.",
			},
			"custom_rules": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "A map of custom rules for the policy. Non-string values must be encoded as JSON strings.",
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
				Description:    "Audit information for the policy.",
			},
		},
	}
}

func (r *PolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating policy", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})

	createReq := buildPolicyCreateRequest(plan)

	result, err := r.client.CreatePolicy(plan.Metalake.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating policy", plan.Name.ValueString(), err)...)
		return
	}

	setStateFromPolicy(ctx, &resp.Diagnostics, plan.Metalake.ValueString(), &result.Policy, &plan)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created policy", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})
}

func (r *PolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading policy", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	result, err := r.client.GetPolicy(state.Metalake.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading policy", state.Name.ValueString(), err)...)
		return
	}

	setStateFromPolicy(ctx, &resp.Diagnostics, state.Metalake.ValueString(), &result.Policy, &state)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *PolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state PolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := plan.Metalake.ValueString()
	name := state.Name.ValueString()

	tflog.Debug(ctx, "Updating policy", map[string]interface{}{"metalake": metalake, "name": name})

	if !plan.Name.Equal(state.Name) {
		resp.Diagnostics.AddError(
			"Policy name update not supported",
			"Renaming a policy is not supported. The resource must be recreated.",
		)
		return
	}

	commentChanged := !plan.Comment.Equal(state.Comment)
	contentChanged := !policyContentEqual(plan, state)
	enabledChanged := !plan.Enabled.Equal(state.Enabled)

	updated := false

	if commentChanged || contentChanged {
		updateReq := buildPolicyCreateRequest(plan)
		result, err := r.client.UpdatePolicy(metalake, name, updateReq)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating policy", name, err)...)
			return
		}
		setStateFromPolicy(ctx, &resp.Diagnostics, metalake, &result.Policy, &plan)
		updated = true
	}

	if enabledChanged {
		result, err := r.client.SetPolicyEnabled(metalake, name, plan.Enabled.ValueBool())
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating policy enabled state", name, err)...)
			return
		}
		setStateFromPolicy(ctx, &resp.Diagnostics, metalake, &result.Policy, &plan)
		updated = true
	}

	if !updated {
		setStateFromPolicy(ctx, &resp.Diagnostics, metalake, nil, &plan)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated policy", map[string]interface{}{"metalake": metalake, "name": plan.Name.ValueString()})
}

func (r *PolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting policy", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	_, err := r.client.DeletePolicy(state.Metalake.ValueString(), state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("deleting policy", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted policy", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})
}

func (r *PolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idx := strings.LastIndex(req.ID, ".")
	if idx == -1 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected 'metalake.policy_name', got: %s", req.ID),
		)
		return
	}

	metalake := req.ID[:idx]
	name := req.ID[idx+1:]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), metalake)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func buildPolicyCreateRequest(model PolicyResourceModel) *models.PolicyCreateRequest {
	return &models.PolicyCreateRequest{
		Name:       model.Name.ValueString(),
		Comment:    model.Comment.ValueString(),
		PolicyType: model.PolicyType.ValueString(),
		Enabled:    model.Enabled.ValueBool(),
		Content: &models.PolicyContent{
			SupportedObjectTypes: listFromTF(model.SupportedObjectTypes),
			Properties:           mapFromTF(model.Properties),
			CustomRules:          mapFromTF(model.CustomRules),
		},
	}
}

func policyContentEqual(plan, state PolicyResourceModel) bool {
	if !stringSlicesEqual(listFromTF(plan.SupportedObjectTypes), listFromTF(state.SupportedObjectTypes)) {
		return false
	}
	if !mapsEqual(mapFromTF(plan.Properties), mapFromTF(state.Properties)) {
		return false
	}
	if !mapsEqual(mapFromTF(plan.CustomRules), mapFromTF(state.CustomRules)) {
		return false
	}
	return true
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]int, len(a))
	for _, v := range a {
		set[v]++
	}
	for _, v := range b {
		set[v]--
		if set[v] < 0 {
			return false
		}
	}
	return true
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func setStateFromPolicy(ctx context.Context, diags *diag.Diagnostics, metalake string, policy *models.Policy, model *PolicyResourceModel) {
	if policy != nil {
		model.Name = types.StringValue(policy.Name)
		model.Comment = types.StringValue(policy.Comment)
		model.PolicyType = types.StringValue(policy.PolicyType)
		model.Enabled = types.BoolValue(policy.Enabled)
		model.ID = types.StringValue(metalake + "." + policy.Name)

		if policy.Content != nil {
			typesSet, d := types.SetValueFrom(ctx, types.StringType, policy.Content.SupportedObjectTypes)
			diags.Append(d...)
			if !diags.HasError() {
				model.SupportedObjectTypes = typesSet
			}

			props, d := types.MapValueFrom(ctx, types.StringType, policy.Content.Properties)
			diags.Append(d...)
			if !diags.HasError() {
				model.Properties = props
			}

			rules, d := types.MapValueFrom(ctx, types.StringType, policy.Content.CustomRules)
			diags.Append(d...)
			if !diags.HasError() {
				model.CustomRules = rules
			}
		}

		if policy.Audit != nil {
			auditObj, d := auditToObjectValue(ctx, policy.Audit)
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
