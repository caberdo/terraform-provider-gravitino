package role

import (
	"context"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var _ datasource.DataSource = &RoleDataSource{}
var _ datasource.DataSourceWithConfigure = &RoleDataSource{}

type RoleDataSource struct {
	client *client.Client
}

func NewRoleDataSource() datasource.DataSource {
	return &RoleDataSource{}
}

func (d *RoleDataSource) SetClient(c *client.Client) {
	d.client = c
}

type RoleDataSourceModel struct {
	Metalake         types.String `tfsdk:"metalake"`
	Name             types.String `tfsdk:"name"`
	Properties       types.Map    `tfsdk:"properties"`
	SecurableObjects types.Set    `tfsdk:"securable_objects"`
	Audit            types.Object `tfsdk:"audit"`
}

var RolePrivilegeAttrTypes = map[string]attr.Type{
	"name":      types.StringType,
	"condition": types.StringType,
}

var RoleSecurableObjectAttrTypes = map[string]attr.Type{
	"full_name":  types.StringType,
	"type":       types.StringType,
	"privileges": types.SetType{ElemType: types.ObjectType{AttrTypes: RolePrivilegeAttrTypes}},
}

var RoleAuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

func (d *RoleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RoleDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_role"
}

func (d *RoleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The role name.",
			},
			"properties": schema.MapAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "A map of key-value properties for the role.",
			},
			"securable_objects": schema.SetNestedAttribute{
				Computed:    true,
				Description: "The securable objects and their privileges assigned to the role.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"full_name": schema.StringAttribute{
							Computed:    true,
							Description: "The full name of the securable object.",
						},
						"type": schema.StringAttribute{
							Computed:    true,
							Description: "The type of the securable object.",
						},
						"privileges": schema.SetNestedAttribute{
							Computed:    true,
							Description: "The privileges for the securable object.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Computed:    true,
										Description: "The privilege name.",
									},
									"condition": schema.StringAttribute{
										Computed:    true,
										Description: "The privilege condition.",
									},
								},
							},
						},
					},
				},
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: RoleAuditAttrTypes,
				Description:    "Audit information for the role.",
			},
		},
	}
}

func (d *RoleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config RoleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.GetRole(config.Metalake.ValueString(), config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to get role", err.Error())
		return
	}

	setDataSourceStateFromRole(ctx, &resp.Diagnostics, &result.Role, &config)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func setDataSourceStateFromRole(ctx context.Context, diags *diag.Diagnostics, role *models.RoleDetail, model *RoleDataSourceModel) {
	props, d := types.MapValueFrom(ctx, types.StringType, role.Properties)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Properties = props

	secObjs, d := securableObjectsToTFForDS(ctx, role.SecurableObjects)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.SecurableObjects = secObjs

	auditObj, d := auditToObjectValueForDS(ctx, role.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Audit = auditObj
}

func securableObjectsToTFForDS(ctx context.Context, objects []models.SecurableObject) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics

	if len(objects) == 0 {
		return types.SetNull(types.ObjectType{AttrTypes: RoleSecurableObjectAttrTypes}), diags
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
			privObj, d := types.ObjectValue(RolePrivilegeAttrTypes, privAttrs)
			diags.Append(d...)
			if diags.HasError() {
				return types.SetNull(types.ObjectType{AttrTypes: RoleSecurableObjectAttrTypes}), diags
			}
			privItems = append(privItems, privObj)
		}

		privSet, d := types.SetValue(types.ObjectType{AttrTypes: RolePrivilegeAttrTypes}, privItems)
		diags.Append(d...)
		if diags.HasError() {
			return types.SetNull(types.ObjectType{AttrTypes: RoleSecurableObjectAttrTypes}), diags
		}

		soAttrs := map[string]attr.Value{
			"full_name":  types.StringValue(o.FullName),
			"type":       types.StringValue(strings.ToUpper(o.Type)),
			"privileges": privSet,
		}
		soObj, d := types.ObjectValue(RoleSecurableObjectAttrTypes, soAttrs)
		diags.Append(d...)
		if diags.HasError() {
			return types.SetNull(types.ObjectType{AttrTypes: RoleSecurableObjectAttrTypes}), diags
		}
		items = append(items, soObj)
	}

	return types.SetValue(types.ObjectType{AttrTypes: RoleSecurableObjectAttrTypes}, items)
}

func auditToObjectValueForDS(ctx context.Context, audit *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(RoleAuditAttrTypes), nil
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

	return types.ObjectValue(RoleAuditAttrTypes, attrs)
}
