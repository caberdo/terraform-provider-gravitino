package tag

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

var _ datasource.DataSource = &TagDataSource{}
var _ datasource.DataSourceWithConfigure = &TagDataSource{}

type TagDataSource struct {
	client *client.Client
}

func NewGetDataSource() datasource.DataSource {
	return &TagDataSource{}
}

func (d *TagDataSource) SetClient(c *client.Client) {
	d.client = c
}

type TagDataSourceModel struct {
	Metalake   types.String `tfsdk:"metalake"`
	Name       types.String `tfsdk:"name"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
	Inherited  types.Bool   `tfsdk:"inherited"`
}

func (d *TagDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *TagDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_tag"
}

func (d *TagDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The tag name.",
			},
			"comment": schema.StringAttribute{
				Computed:    true,
				Description: "The tag comment.",
			},
			"properties": schema.MapAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "The tag properties.",
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
				Description:    "Audit information for the tag.",
			},
			"inherited": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the tag is inherited from a parent metadata object. Null when the server does not report it.",
			},
		},
	}
}

func (d *TagDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TagDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.GetTag(ctx, config.Metalake.ValueString(), config.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Tag not found",
				fmt.Sprintf("No Gravitino tag %q exists in metalake %q.", config.Name.ValueString(), config.Metalake.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading tag", config.Name.ValueString(), err)...)
		return
	}

	setDataSourceStateFromTag(ctx, &resp.Diagnostics, &result.Tag, &config)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func setDataSourceStateFromTag(ctx context.Context, diags *diag.Diagnostics, tag *models.Tag, model *TagDataSourceModel) {
	if tag.Comment != "" {
		model.Comment = types.StringValue(tag.Comment)
	} else {
		model.Comment = types.StringNull()
	}

	if len(tag.Properties) > 0 {
		props, d := types.MapValueFrom(ctx, types.StringType, tag.Properties)
		diags.Append(d...)
		if diags.HasError() {
			return
		}
		model.Properties = props
	} else {
		model.Properties = types.MapNull(types.StringType)
	}

	auditObj, d := auditToObjectValueForDS(ctx, tag.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Audit = auditObj

	if tag.Inherited != nil {
		model.Inherited = types.BoolValue(*tag.Inherited)
	} else {
		model.Inherited = types.BoolNull()
	}
}
