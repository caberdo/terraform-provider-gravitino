package user

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/providerbase"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &UserDataSource{}
var _ datasource.DataSourceWithConfigure = &UserDataSource{}

type UserDataSource struct {
	providerbase.DataSource
}

func NewGetDataSource() datasource.DataSource {
	return &UserDataSource{}
}

type UserDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Name     types.String `tfsdk:"name"`
	Roles    types.Set    `tfsdk:"roles"`
	Audit    types.Object `tfsdk:"audit"`
}

var AuditAttrTypes = models.AuditAttrTypes

func (d *UserDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_user"
}

func (d *UserDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The user name.",
			},
			"roles": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "The roles assigned to the user.",
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
				Description:    "Audit information for the user.",
			},
		},
	}
}

func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config UserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.Client.GetUser(ctx, config.Metalake.ValueString(), config.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"User not found",
				fmt.Sprintf("No user %q exists in metalake %q.", config.Name.ValueString(), config.Metalake.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading user", config.Name.ValueString(), err)...)
		return
	}

	setDataSourceStateFromUser(ctx, &resp.Diagnostics, &result.User, &config)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func setDataSourceStateFromUser(ctx context.Context, diags *diag.Diagnostics, user *models.User, model *UserDataSourceModel) {
	roles, d := types.SetValueFrom(ctx, types.StringType, user.Roles)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Roles = roles

	auditObj, d := auditToObjectValueForDS(ctx, user.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Audit = auditObj
}
