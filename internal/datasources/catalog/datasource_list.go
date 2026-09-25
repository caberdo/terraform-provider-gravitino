package catalog

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

var _ datasource.DataSource = &CatalogsDataSource{}
var _ datasource.DataSourceWithConfigure = &CatalogsDataSource{}

type CatalogsDataSource struct {
	client *client.Client
}

func NewListDataSource() datasource.DataSource {
	return &CatalogsDataSource{}
}

func (d *CatalogsDataSource) SetClient(c *client.Client) {
	d.client = c
}

type CatalogsDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Catalogs types.List   `tfsdk:"catalogs"`
}

type catalogItemModel struct {
	Name       types.String `tfsdk:"name"`
	Type       types.String `tfsdk:"type"`
	Provider   types.String `tfsdk:"catalog_provider"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
}

var CatalogItemAttrTypes = map[string]attr.Type{
	"name":             types.StringType,
	"type":             types.StringType,
	"catalog_provider": types.StringType,
	"comment":          types.StringType,
	"properties":       types.MapType{ElemType: types.StringType},
	"audit":            types.ObjectType{AttrTypes: AuditAttrTypes},
}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

func (d *CatalogsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CatalogsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_catalogs"
}

func (d *CatalogsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"catalogs": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "The catalog name.",
						},
						"type": schema.StringAttribute{
							Computed:    true,
							Description: "The catalog type.",
						},
						"catalog_provider": schema.StringAttribute{
							Computed:    true,
							Description: "The catalog provider.",
						},
						"comment": schema.StringAttribute{
							Computed:    true,
							Description: "The catalog comment.",
						},
						"properties": schema.MapAttribute{
							Computed:    true,
							Sensitive:   true,
							ElementType: types.StringType,
							Description: "The catalog properties.",
						},
						"audit": schema.ObjectAttribute{
							Computed:       true,
							AttributeTypes: AuditAttrTypes,
							Description:    "Audit information for the catalog.",
						},
					},
				},
			},
		},
	}
}

func (d *CatalogsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CatalogsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.ListCatalogsDetails(ctx, config.Metalake.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Metalake not found",
				fmt.Sprintf("No metalake %q exists; cannot list its catalogs.", config.Metalake.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("listing catalogs", config.Metalake.ValueString(), err)...)
		return
	}

	items := make([]attr.Value, 0, len(result.Catalogs))
	for _, catalog := range result.Catalogs {
		cat := catalog
		item := catalogToItemModel(ctx, &cat, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if item == nil {
			continue
		}
		obj, objDiags := types.ObjectValueFrom(ctx, CatalogItemAttrTypes, item)
		resp.Diagnostics.Append(objDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, obj)
	}

	catalogsList, listDiags := types.ListValue(types.ObjectType{AttrTypes: CatalogItemAttrTypes}, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Catalogs = catalogsList
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func catalogToItemModel(ctx context.Context, c *models.Catalog, diags *diag.Diagnostics) *catalogItemModel {
	if c == nil {
		return nil
	}

	item := &catalogItemModel{
		Name:     types.StringValue(c.Name),
		Type:     types.StringValue(c.Type),
		Provider: types.StringValue(c.Provider),
		Comment:  types.StringValue(c.Comment),
	}

	props, d := types.MapValueFrom(ctx, types.StringType, c.Properties)
	diags.Append(d...)
	if diags.HasError() {
		return nil
	}
	item.Properties = props

	auditObj, d := auditToObjectValueForDS(ctx, c.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return nil
	}
	item.Audit = auditObj

	return item
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
