package policy

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &PolicyDataSource{}
var _ datasource.DataSourceWithConfigure = &PolicyDataSource{}

// PolicyDataSource reads a single policy by name.
type PolicyDataSource struct {
	client *client.Client
}

func NewDataSource() datasource.DataSource {
	return &PolicyDataSource{}
}

func (d *PolicyDataSource) SetClient(c *client.Client) {
	d.client = c
}

type PolicyDataSourceModel struct {
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

func (d *PolicyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *PolicyDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_policy"
}

func (d *PolicyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := policyComputedAttributes()
	attributes["metalake"] = schema.StringAttribute{
		Required:    true,
		Description: "The metalake name.",
	}
	attributes["name"] = schema.StringAttribute{
		Required:    true,
		Description: "The policy name.",
	}

	resp.Schema = schema.Schema{
		Description: "Reads a single Gravitino policy by name, including its content and audit information.",
		Attributes:  attributes,
	}
}

func (d *PolicyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PolicyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.GetPolicy(ctx, config.Metalake.ValueString(), config.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Policy not found",
				fmt.Sprintf("No policy %q exists in metalake %q.", config.Name.ValueString(), config.Metalake.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading policy", config.Name.ValueString(), err)...)
		return
	}

	setStateFromPolicy(ctx, &resp.Diagnostics, &result.Policy, &config)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// policyComputedAttributes returns the computed policy attributes shared by the
// list and get data sources. metalake and name are added by each schema with the
// appropriate Required or Computed setting.
func policyComputedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"comment": schema.StringAttribute{
			Computed:    true,
			Description: "The policy comment.",
		},
		"policy_type": schema.StringAttribute{
			Computed:    true,
			Description: "The policy type.",
		},
		"enabled": schema.BoolAttribute{
			Computed:    true,
			Description: "Whether the policy is enabled.",
		},
		"supported_object_types": schema.SetAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "The object types this policy supports.",
		},
		"properties": schema.MapAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "The policy properties.",
		},
		"custom_rules": schema.MapAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "The policy custom rules.",
		},
		"audit": schema.ObjectAttribute{
			Computed:       true,
			AttributeTypes: AuditAttrTypes,
			Description:    "Audit information for the policy.",
		},
	}
}

func setStateFromPolicy(ctx context.Context, diags *diag.Diagnostics, p *models.Policy, model *PolicyDataSourceModel) {
	model.Name = types.StringValue(p.Name)
	model.Comment = types.StringValue(p.Comment)
	model.PolicyType = types.StringValue(p.PolicyType)
	model.Enabled = types.BoolValue(p.Enabled)

	var supportedObjectTypes []string
	var properties, customRules map[string]string
	if p.Content != nil {
		supportedObjectTypes = models.NormalizePolicyObjectTypes(p.Content.SupportedObjectTypes)
		properties = p.Content.Properties
		customRules = p.Content.CustomRules
	}

	typesSet, d := types.SetValueFrom(ctx, types.StringType, supportedObjectTypes)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.SupportedObjectTypes = typesSet

	props, d := types.MapValueFrom(ctx, types.StringType, properties)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Properties = props

	rules, d := types.MapValueFrom(ctx, types.StringType, customRules)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.CustomRules = rules

	auditObj, d := auditToObjectValueForDS(ctx, p.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Audit = auditObj
}
