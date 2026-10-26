package policy

import (
	"context"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/tfutil"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ObjectPoliciesDataSource{}
var _ datasource.DataSourceWithConfigure = &ObjectPoliciesDataSource{}

// ObjectPoliciesDataSource lists the policies attached to one metadata object.
// The listing includes policies inherited from parent objects; the
// gravitino_policy_association resource manages direct associations only.
type ObjectPoliciesDataSource struct {
	client *client.Client
}

func NewObjectPoliciesDataSource() datasource.DataSource {
	return &ObjectPoliciesDataSource{}
}

func (d *ObjectPoliciesDataSource) SetClient(c *client.Client) {
	d.client = c
}

type ObjectPoliciesDataSourceModel struct {
	Metalake       types.String `tfsdk:"metalake"`
	ObjectType     types.String `tfsdk:"object_type"`
	ObjectFullName types.String `tfsdk:"object_full_name"`
	Policies       types.List   `tfsdk:"policies"`
}

func (d *ObjectPoliciesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *ObjectPoliciesDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_object_policies"
}

func (d *ObjectPoliciesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the policies attached to one metadata object. The result includes policies inherited from the object's ancestors.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"object_type": schema.StringAttribute{
				Required: true,
				Description: fmt.Sprintf("The metadata object type. One of: %s. VIEW and FUNCTION require Gravitino >= 1.3.1 and are rejected on an older server with an explicit diagnostic. "+
					"Columns cannot carry a policy.", strings.Join(models.PolicyObjectTypes, ", ")),
				Validators: []validator.String{
					stringvalidator.OneOf(models.PolicyObjectTypes...),
				},
			},
			"object_full_name": schema.StringAttribute{
				Required: true,
				Description: "The full name of the metadata object relative to the metalake, for example 'my_catalog' for a CATALOG " +
					"or 'my_catalog.my_schema.my_table' for a TABLE. Do not prefix it with the metalake name.",
			},
			"policies": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "The names of the policies attached to the object, including inherited policies.",
			},
		},
	}
}

func (d *ObjectPoliciesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ObjectPoliciesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := config.Metalake.ValueString()
	objectType := config.ObjectType.ValueString()
	objectFullName := config.ObjectFullName.ValueString()

	if err := d.client.CheckMetadataObjectTypeSupported(ctx, objectType); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("object_type"), "Unsupported object_type value", err.Error())
		return
	}

	result, err := d.client.ListPoliciesForObject(ctx, metalake, objectType, objectFullName)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Metadata object not found",
				fmt.Sprintf("Cannot list the policies of %s %q in metalake %q: the object does not exist.", objectType, objectFullName, metalake),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("listing policies of", objectFullName, err)...)
		return
	}

	names := make([]string, 0, len(result.Policies))
	for _, policy := range result.Policies {
		names = append(names, policy.Name)
	}

	policies, listDiags := tfutil.StringsToList(names)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.Policies = policies

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
