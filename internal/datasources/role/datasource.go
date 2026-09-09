package role

import (
	"context"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RolesDataSource{}
var _ datasource.DataSourceWithConfigure = &RolesDataSource{}

type RolesDataSource struct {
	client *client.Client
}

func New() datasource.DataSource {
	return &RolesDataSource{}
}

func (d *RolesDataSource) SetClient(c *client.Client) {
	d.client = c
}

type RolesDataSourceModel struct {
	Metalake     types.String `tfsdk:"metalake"`
	ResourceType types.String `tfsdk:"resource_type"`
	Resource     types.String `tfsdk:"resource"`
	Roles        types.List   `tfsdk:"roles"`
}

type roleItemModel struct {
	Name            types.String `tfsdk:"name"`
	Privileges      types.List   `tfsdk:"privileges"`
	SecurableObject types.String `tfsdk:"securable_object"`
}

var RoleItemAttrTypes = map[string]attr.Type{
	"name":             types.StringType,
	"privileges":       types.ListType{ElemType: types.StringType},
	"securable_object": types.StringType,
}

func (d *RolesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected DataSource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	d.client = c
}

func (d *RolesDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_roles"
}

func (d *RolesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"resource_type": schema.StringAttribute{
				Required:    true,
				Description: "The resource type (e.g. catalogs, schemas, tables).",
				Validators: []validator.String{
					stringvalidator.OneOf(models.AllObjectTypes...),
				},
			},
			"resource": schema.StringAttribute{
				Required:    true,
				Description: "The resource name.",
			},
			"roles": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The roles for the resource.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "The role name.",
						},
						"privileges": schema.ListAttribute{
							Computed:    true,
							ElementType: types.StringType,
							Description: "The privileges assigned to the role.",
						},
						"securable_object": schema.StringAttribute{
							Computed:    true,
							Description: "The securable object associated with the role.",
						},
					},
				},
			},
		},
	}
}

func (d *RolesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config RolesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.ListRoles(
		config.Metalake.ValueString(),
		config.ResourceType.ValueString(),
		config.Resource.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list roles", err.Error())
		return
	}

	items := make([]attr.Value, 0, len(result.Roles))
	for _, role := range result.Roles {
		r := role
		item := roleToItemModel(ctx, &r)
		if item == nil {
			continue
		}
		obj, objDiags := types.ObjectValueFrom(ctx, RoleItemAttrTypes, item)
		resp.Diagnostics.Append(objDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, obj)
	}

	rolesList, listDiags := types.ListValue(types.ObjectType{AttrTypes: RoleItemAttrTypes}, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Roles = rolesList
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func roleToItemModel(ctx context.Context, r *models.Role) *roleItemModel {
	if r == nil {
		return nil
	}

	item := &roleItemModel{
		Name:            types.StringValue(r.Name),
		SecurableObject: types.StringValue(r.SecurableObject),
	}

	if r.Privileges != nil {
		privileges := make([]attr.Value, 0, len(r.Privileges))
		for _, p := range r.Privileges {
			privileges = append(privileges, types.StringValue(strings.ToUpper(p)))
		}
		privList, d := types.ListValue(types.StringType, privileges)
		if d.HasError() {
			return nil
		}
		item.Privileges = privList
	} else {
		item.Privileges = types.ListNull(types.StringType)
	}

	return item
}
