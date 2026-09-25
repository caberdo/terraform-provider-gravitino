package tag

import (
	"context"
	"fmt"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var _ datasource.DataSource = &TagsDataSource{}
var _ datasource.DataSourceWithConfigure = &TagsDataSource{}

type TagsDataSource struct {
	client *client.Client
}

func NewListDataSource() datasource.DataSource {
	return &TagsDataSource{}
}

func (d *TagsDataSource) SetClient(c *client.Client) {
	d.client = c
}

type TagsDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Tags     types.List   `tfsdk:"tags"`
}

type tagItemModel struct {
	Name       types.String `tfsdk:"name"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
	Inherited  types.Bool   `tfsdk:"inherited"`
}

var TagItemAttrTypes = map[string]attr.Type{
	"name":       types.StringType,
	"comment":    types.StringType,
	"properties": types.MapType{ElemType: types.StringType},
	"audit":      types.ObjectType{AttrTypes: AuditAttrTypes},
	"inherited":  types.BoolType,
}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

func (d *TagsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *TagsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_tags"
}

func (d *TagsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"tags": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
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
				},
			},
		},
	}
}

func (d *TagsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TagsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.ListTagsDetailed(ctx, config.Metalake.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Metalake not found",
				fmt.Sprintf("Cannot list tags of metalake %q: it does not exist.", config.Metalake.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("listing tags", config.Metalake.ValueString(), err)...)
		return
	}

	items := make([]attr.Value, 0, len(result.Tags))
	for _, tag := range result.Tags {
		t := tag
		item := tagToItemModel(ctx, &t, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if item == nil {
			continue
		}
		obj, objDiags := types.ObjectValueFrom(ctx, TagItemAttrTypes, item)
		resp.Diagnostics.Append(objDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, obj)
	}

	tagsList, listDiags := types.ListValue(types.ObjectType{AttrTypes: TagItemAttrTypes}, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Tags = tagsList
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func tagToItemModel(ctx context.Context, t *models.Tag, diags *diag.Diagnostics) *tagItemModel {
	if t == nil {
		return nil
	}

	item := &tagItemModel{
		Name:    types.StringValue(t.Name),
		Comment: commentToValue(t.Comment),
	}

	if len(t.Properties) > 0 {
		props, d := types.MapValueFrom(ctx, types.StringType, t.Properties)
		diags.Append(d...)
		if diags.HasError() {
			return nil
		}
		item.Properties = props
	} else {
		item.Properties = types.MapNull(types.StringType)
	}

	auditObj, d := auditToObjectValueForDS(ctx, t.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return nil
	}
	item.Audit = auditObj

	if t.Inherited != nil {
		item.Inherited = types.BoolValue(*t.Inherited)
	} else {
		item.Inherited = types.BoolNull()
	}

	return item
}

// commentToValue maps an absent/empty server comment to null so the data source
// never invents an empty string where the API reports no value.
func commentToValue(comment string) types.String {
	if comment == "" {
		return types.StringNull()
	}
	return types.StringValue(comment)
}

func auditToObjectValueForDS(ctx context.Context, audit *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(AuditAttrTypes), nil
	}

	creator := types.StringValue(audit.Creator)
	lastModifier := types.StringValue(audit.LastModifier)

	var createTime, lastModifiedTime types.String
	if audit.CreateTime != nil {
		createTime = types.StringValue(audit.CreateTime.Format(time.RFC3339))
	} else {
		createTime = types.StringNull()
	}
	if audit.LastModifiedTime != nil {
		lastModifiedTime = types.StringValue(audit.LastModifiedTime.Format(time.RFC3339))
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
