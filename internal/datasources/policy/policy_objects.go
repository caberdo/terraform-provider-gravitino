package policy

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &PolicyObjectsDataSource{}
var _ datasource.DataSourceWithConfigure = &PolicyObjectsDataSource{}

// PolicyObjectsDataSource lists the metadata objects a policy is directly
// attached to. Inherited reach is not included: the API lists direct
// attachments only.
type PolicyObjectsDataSource struct {
	client *client.Client
}

func NewPolicyObjectsDataSource() datasource.DataSource {
	return &PolicyObjectsDataSource{}
}

func (d *PolicyObjectsDataSource) SetClient(c *client.Client) {
	d.client = c
}

type PolicyObjectsDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Policy   types.String `tfsdk:"policy"`
	Objects  types.List   `tfsdk:"objects"`
}

type policyObjectItemModel struct {
	Type     types.String `tfsdk:"type"`
	FullName types.String `tfsdk:"full_name"`
}

var PolicyObjectItemAttrTypes = map[string]attr.Type{
	"type":      types.StringType,
	"full_name": types.StringType,
}

func (d *PolicyObjectsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *PolicyObjectsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_policy_objects"
}

func (d *PolicyObjectsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the metadata objects a policy is directly attached to. Objects that only inherit the policy are not included.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"policy": schema.StringAttribute{
				Required:    true,
				Description: "The policy name.",
			},
			"objects": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Computed:    true,
							Description: "The metadata object type, canonicalised to the upper-case spelling of the API enum.",
						},
						"full_name": schema.StringAttribute{
							Computed:    true,
							Description: "The full name of the metadata object, relative to the metalake.",
						},
					},
				},
			},
		},
	}
}

func (d *PolicyObjectsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PolicyObjectsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := config.Metalake.ValueString()
	policy := config.Policy.ValueString()

	result, err := d.client.ListObjectsForPolicy(ctx, metalake, policy)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Policy not found",
				fmt.Sprintf("No policy %q exists in metalake %q; cannot list the objects it is attached to.", policy, metalake),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("listing objects of", policy, err)...)
		return
	}

	items := make([]attr.Value, 0, len(result.MetadataObjects))
	for _, object := range result.MetadataObjects {
		item := policyObjectItemModel{
			Type:     types.StringValue(models.CanonicalObjectType(object.Type)),
			FullName: types.StringValue(object.FullName),
		}
		obj, objDiags := types.ObjectValueFrom(ctx, PolicyObjectItemAttrTypes, item)
		resp.Diagnostics.Append(objDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, obj)
	}

	objects, listDiags := types.ListValue(types.ObjectType{AttrTypes: PolicyObjectItemAttrTypes}, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.Objects = objects

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
