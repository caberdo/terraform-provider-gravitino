package fileset

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

var _ datasource.DataSource = &FilesetsDataSource{}
var _ datasource.DataSourceWithConfigure = &FilesetsDataSource{}

type FilesetsDataSource struct {
	client *client.Client
}

func NewListDataSource() datasource.DataSource {
	return &FilesetsDataSource{}
}

func (d *FilesetsDataSource) SetClient(c *client.Client) {
	d.client = c
}

type FilesetsDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Catalog  types.String `tfsdk:"catalog"`
	Schema   types.String `tfsdk:"schema"`
	Filesets types.List   `tfsdk:"filesets"`
}

type filesetItemModel struct {
	Name            types.String `tfsdk:"name"`
	Comment         types.String `tfsdk:"comment"`
	Type            types.String `tfsdk:"type"`
	StorageLocation types.String `tfsdk:"storage_location"`
	Properties      types.Map    `tfsdk:"properties"`
	Audit           types.Object `tfsdk:"audit"`
}

var FilesetItemAttrTypes = map[string]attr.Type{
	"name":             types.StringType,
	"comment":          types.StringType,
	"type":             types.StringType,
	"storage_location": types.StringType,
	"properties":       types.MapType{ElemType: types.StringType},
	"audit":            types.ObjectType{AttrTypes: AuditAttrTypes},
}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
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

func (d *FilesetsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *FilesetsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_filesets"
}

func (d *FilesetsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"catalog": schema.StringAttribute{
				Required:    true,
				Description: "The catalog name.",
			},
			"schema": schema.StringAttribute{
				Required:    true,
				Description: "The schema name.",
			},
			"filesets": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "The fileset name.",
						},
						"comment": schema.StringAttribute{
							Computed:    true,
							Description: "The fileset comment.",
						},
						"type": schema.StringAttribute{
							Computed:    true,
							Description: "The fileset type.",
						},
						"storage_location": schema.StringAttribute{
							Computed:    true,
							Description: "The fileset storage location.",
						},
						"properties": schema.MapAttribute{
							Computed:    true,
							ElementType: types.StringType,
							Description: "The fileset properties.",
						},
						"audit": schema.ObjectAttribute{
							Computed:       true,
							AttributeTypes: AuditAttrTypes,
							Description:    "Audit information for the fileset.",
						},
					},
				},
			},
		},
	}
}

func (d *FilesetsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config FilesetsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.ListFilesetsDetails(ctx, config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Metalake, catalog or schema not found",
				fmt.Sprintf("No metalake %q, catalog %q or schema %q exists.", config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("listing filesets", config.Schema.ValueString(), err)...)
		return
	}

	items := make([]attr.Value, 0, len(result))
	for _, fs := range result {
		fsCopy := fs
		item := filesetToItemModel(ctx, &fsCopy, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if item == nil {
			continue
		}
		obj, objDiags := types.ObjectValueFrom(ctx, FilesetItemAttrTypes, item)
		resp.Diagnostics.Append(objDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, obj)
	}

	filesetsList, listDiags := types.ListValue(types.ObjectType{AttrTypes: FilesetItemAttrTypes}, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Filesets = filesetsList
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func filesetToItemModel(ctx context.Context, fs *models.Fileset, diags *diag.Diagnostics) *filesetItemModel {
	if fs == nil {
		return nil
	}

	item := &filesetItemModel{
		Name:            types.StringValue(fs.Name),
		Comment:         types.StringValue(fs.Comment),
		Type:            types.StringValue(fs.Type),
		StorageLocation: types.StringValue(fs.StorageLocation),
	}

	props, d := types.MapValueFrom(ctx, types.StringType, fs.Properties)
	diags.Append(d...)
	if diags.HasError() {
		return nil
	}
	item.Properties = props

	auditObj, d := auditToObjectValueForDS(ctx, fs.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return nil
	}
	item.Audit = auditObj

	return item
}
