package tag

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

var _ datasource.DataSource = &TagObjectsDataSource{}
var _ datasource.DataSourceWithConfigure = &TagObjectsDataSource{}

// TagObjectsDataSource lists the metadata objects a tag is directly attached
// to. Inherited reach is not included: the API lists direct attachments only.
type TagObjectsDataSource struct {
	client *client.Client
}

func NewTagObjectsDataSource() datasource.DataSource {
	return &TagObjectsDataSource{}
}

func (d *TagObjectsDataSource) SetClient(c *client.Client) {
	d.client = c
}

type TagObjectsDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Tag      types.String `tfsdk:"tag"`
	Objects  types.List   `tfsdk:"objects"`
}

type tagObjectItemModel struct {
	Type     types.String `tfsdk:"type"`
	FullName types.String `tfsdk:"full_name"`
}

var TagObjectItemAttrTypes = map[string]attr.Type{
	"type":      types.StringType,
	"full_name": types.StringType,
}

func (d *TagObjectsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *TagObjectsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_tag_objects"
}

func (d *TagObjectsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the metadata objects a tag is directly attached to. Objects that only inherit the tag are not included.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"tag": schema.StringAttribute{
				Required:    true,
				Description: "The tag name.",
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

func (d *TagObjectsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TagObjectsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := config.Metalake.ValueString()
	tag := config.Tag.ValueString()

	result, err := d.client.ListObjectsForTag(ctx, metalake, tag)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Tag not found",
				fmt.Sprintf("No tag %q exists in metalake %q; cannot list the objects it is attached to.", tag, metalake),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("listing objects of", tag, err)...)
		return
	}

	items := make([]attr.Value, 0, len(result.MetadataObjects))
	for _, object := range result.MetadataObjects {
		item := tagObjectItemModel{
			Type:     types.StringValue(models.CanonicalObjectType(object.Type)),
			FullName: types.StringValue(object.FullName),
		}
		obj, objDiags := types.ObjectValueFrom(ctx, TagObjectItemAttrTypes, item)
		resp.Diagnostics.Append(objDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, obj)
	}

	objects, listDiags := types.ListValue(types.ObjectType{AttrTypes: TagObjectItemAttrTypes}, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.Objects = objects

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
