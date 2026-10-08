package policy

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &PoliciesDataSource{}
var _ datasource.DataSourceWithConfigure = &PoliciesDataSource{}

type PoliciesDataSource struct {
	client *client.Client
}

func NewListDataSource() datasource.DataSource {
	return &PoliciesDataSource{}
}

func (d *PoliciesDataSource) SetClient(c *client.Client) {
	d.client = c
}

type PoliciesDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Policies types.List   `tfsdk:"policies"`
}

type policyItemModel struct {
	Name                 types.String `tfsdk:"name"`
	Comment              types.String `tfsdk:"comment"`
	PolicyType           types.String `tfsdk:"policy_type"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	SupportedObjectTypes types.Set    `tfsdk:"supported_object_types"`
	Properties           types.Map    `tfsdk:"properties"`
	CustomRules          types.Map    `tfsdk:"custom_rules"`
	Audit                types.Object `tfsdk:"audit"`
}

var PolicyItemAttrTypes = map[string]attr.Type{
	"name":                   types.StringType,
	"comment":                types.StringType,
	"policy_type":            types.StringType,
	"enabled":                types.BoolType,
	"supported_object_types": types.SetType{ElemType: types.StringType},
	"properties":             types.MapType{ElemType: types.StringType},
	"custom_rules":           types.MapType{ElemType: types.StringType},
	"audit":                  types.ObjectType{AttrTypes: AuditAttrTypes},
}

var AuditAttrTypes = models.AuditAttrTypes

func (d *PoliciesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *PoliciesDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_policies"
}

func (d *PoliciesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	nested := policyComputedAttributes()
	nested["name"] = schema.StringAttribute{
		Computed:    true,
		Description: "The policy name.",
	}

	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"policies": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: nested,
				},
			},
		},
	}
}

func (d *PoliciesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PoliciesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.ListPolicies(ctx, config.Metalake.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Metalake not found",
				fmt.Sprintf("No metalake %q exists; cannot list its policies.", config.Metalake.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("listing policies", config.Metalake.ValueString(), err)...)
		return
	}

	items := make([]attr.Value, 0, len(result.Policies))
	for _, p := range result.Policies {
		policy := p
		item := policyToItemModel(ctx, &policy, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if item == nil {
			continue
		}
		obj, objDiags := types.ObjectValueFrom(ctx, PolicyItemAttrTypes, item)
		resp.Diagnostics.Append(objDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, obj)
	}

	policiesList, listDiags := types.ListValue(types.ObjectType{AttrTypes: PolicyItemAttrTypes}, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Policies = policiesList
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func policyToItemModel(ctx context.Context, p *models.Policy, diags *diag.Diagnostics) *policyItemModel {
	if p == nil {
		return nil
	}

	item := &policyItemModel{
		Name:       types.StringValue(p.Name),
		Comment:    types.StringValue(p.Comment),
		PolicyType: types.StringValue(p.PolicyType),
		Enabled:    types.BoolValue(p.Enabled),
	}

	var supportedObjectTypes []string
	var properties map[string]string
	var customRules map[string]string
	if p.Content != nil {
		supportedObjectTypes = models.NormalizePolicyObjectTypes(p.Content.SupportedObjectTypes)
		properties = p.Content.Properties
		customRules = p.Content.CustomRules
	}

	typesSet, d := types.SetValueFrom(ctx, types.StringType, supportedObjectTypes)
	diags.Append(d...)
	if diags.HasError() {
		return nil
	}
	item.SupportedObjectTypes = typesSet

	props, d := types.MapValueFrom(ctx, types.StringType, properties)
	diags.Append(d...)
	if diags.HasError() {
		return nil
	}
	item.Properties = props

	rules, d := types.MapValueFrom(ctx, types.StringType, customRules)
	diags.Append(d...)
	if diags.HasError() {
		return nil
	}
	item.CustomRules = rules

	auditObj, d := auditToObjectValueForDS(ctx, p.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return nil
	}
	item.Audit = auditObj

	return item
}

func auditToObjectValueForDS(ctx context.Context, audit *models.Audit) (types.Object, diag.Diagnostics) {
	return models.AuditToObjectValue(ctx, audit)
}
