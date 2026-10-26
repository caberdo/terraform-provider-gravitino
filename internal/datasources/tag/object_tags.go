package tag

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

var _ datasource.DataSource = &ObjectTagsDataSource{}
var _ datasource.DataSourceWithConfigure = &ObjectTagsDataSource{}

// ObjectTagsDataSource lists the tags attached to one metadata object. The
// listing includes tags inherited from parent objects; the
// gravitino_tag_association resource manages direct associations only.
type ObjectTagsDataSource struct {
	client *client.Client
}

func NewObjectTagsDataSource() datasource.DataSource {
	return &ObjectTagsDataSource{}
}

func (d *ObjectTagsDataSource) SetClient(c *client.Client) {
	d.client = c
}

type ObjectTagsDataSourceModel struct {
	Metalake       types.String `tfsdk:"metalake"`
	ObjectType     types.String `tfsdk:"object_type"`
	ObjectFullName types.String `tfsdk:"object_full_name"`
	Tags           types.List   `tfsdk:"tags"`
}

func (d *ObjectTagsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *ObjectTagsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_object_tags"
}

func (d *ObjectTagsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the tags attached to one metadata object. The result includes tags inherited from the object's ancestors.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"object_type": schema.StringAttribute{
				Required: true,
				Description: fmt.Sprintf("The metadata object type. One of: %s. VIEW requires Gravitino >= 1.3.1 and is rejected on an older server with an explicit diagnostic.",
					strings.Join(models.TagObjectTypes, ", ")),
				Validators: []validator.String{
					stringvalidator.OneOf(models.TagObjectTypes...),
				},
			},
			"object_full_name": schema.StringAttribute{
				Required: true,
				Description: "The full name of the metadata object relative to the metalake, for example 'my_catalog' for a CATALOG " +
					"or 'my_catalog.my_schema.my_table' for a TABLE. Do not prefix it with the metalake name.",
			},
			"tags": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "The names of the tags attached to the object, including inherited tags.",
			},
		},
	}
}

func (d *ObjectTagsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ObjectTagsDataSourceModel
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

	result, err := d.client.ListTagsForObject(ctx, metalake, objectType, objectFullName)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Metadata object not found",
				fmt.Sprintf("Cannot list the tags of %s %q in metalake %q: the object does not exist.", objectType, objectFullName, metalake),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("listing tags of", objectFullName, err)...)
		return
	}

	names := make([]string, 0, len(result.Tags))
	for _, tag := range result.Tags {
		names = append(names, tag.Name)
	}

	tags, listDiags := tfutil.StringsToList(names)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.Tags = tags

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
