package view

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ViewsDataSource{}
var _ datasource.DataSourceWithConfigure = &ViewsDataSource{}

type ViewsDataSource struct {
	client *client.Client
}

// ViewListEntryModel is one element of the gravitino_views data source.
type ViewListEntryModel struct {
	Name            types.String                     `tfsdk:"name"`
	Comment         types.String                     `tfsdk:"comment"`
	Columns         []models.ColumnTFSDK             `tfsdk:"column"`
	Representations []models.ViewRepresentationTFSDK `tfsdk:"representation"`
	DefaultCatalog  types.String                     `tfsdk:"default_catalog"`
	DefaultSchema   types.String                     `tfsdk:"default_schema"`
	Properties      types.Map                        `tfsdk:"properties"`
	Audit           types.Object                     `tfsdk:"audit"`
}

type ViewsDataSourceModel struct {
	Metalake types.String         `tfsdk:"metalake"`
	Catalog  types.String         `tfsdk:"catalog"`
	Schema   types.String         `tfsdk:"schema"`
	Views    []ViewListEntryModel `tfsdk:"views"`
}

func NewViewsDataSource() datasource.DataSource {
	return &ViewsDataSource{}
}

func (d *ViewsDataSource) SetClient(c *client.Client) {
	d.client = c
}

func (d *ViewsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_views"
}

func (d *ViewsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all views within a Gravitino metalake, catalog, and schema.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Description: "The metalake name.",
				Required:    true,
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog name.",
				Required:    true,
			},
			"schema": schema.StringAttribute{
				Description: "The schema name.",
				Required:    true,
			},
			"views": schema.ListNestedAttribute{
				Description: "List of views with their details.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The view name.",
							Computed:    true,
						},
						"comment": schema.StringAttribute{
							Description: "The view comment.",
							Computed:    true,
						},
						"column": schema.ListNestedAttribute{
							Description: "The output columns of the view.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Description: "The column name.",
										Computed:    true,
									},
									"type": schema.StringAttribute{
										Description: "The column data type.",
										Computed:    true,
									},
									"comment": schema.StringAttribute{
										Description: "The column comment.",
										Computed:    true,
									},
									"nullable": schema.BoolAttribute{
										Description: "Whether the column is nullable.",
										Computed:    true,
									},
									"auto_increment": schema.BoolAttribute{
										Description: "Whether the column is auto increment.",
										Computed:    true,
									},
									"default_value": schema.StringAttribute{
										Description: "The default value of the column, using the data type of the column.",
										Computed:    true,
									},
								},
							},
						},
						"representation": schema.ListNestedAttribute{
							Description: "The representations of the view body, keyed by dialect.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"type": schema.StringAttribute{
										Description: "The representation type discriminator.",
										Computed:    true,
									},
									"dialect": schema.StringAttribute{
										Description: "The SQL dialect of this representation.",
										Computed:    true,
									},
									"sql": schema.StringAttribute{
										Description: "The SQL text of the view.",
										Computed:    true,
									},
								},
							},
						},
						"default_catalog": schema.StringAttribute{
							Description: "The default catalog used to resolve unqualified identifiers in the view representations.",
							Computed:    true,
						},
						"default_schema": schema.StringAttribute{
							Description: "The default schema used to resolve unqualified identifiers in the view representations.",
							Computed:    true,
						},
						"properties": schema.MapAttribute{
							Description: "Key-value properties for the view.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"audit": schema.ObjectAttribute{
							Description:    "Audit information for the view.",
							Computed:       true,
							AttributeTypes: models.AuditAttrTypes,
						},
					},
				},
			},
		},
	}
}

func (ds *ViewsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider data", "Expected *client.Client, got unexpected type.")
		return
	}
	ds.client = c
}

func (ds *ViewsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ViewsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	views, err := ds.client.ListViewsDetails(ctx, config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("listing views", config.Schema.ValueString(), err)...)
		return
	}

	entries := make([]ViewListEntryModel, 0, len(views))
	for i := range views {
		entry, diags := viewListEntryToModel(ctx, &views[i])
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		entries = append(entries, entry)
	}
	config.Views = entries

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func viewListEntryToModel(ctx context.Context, view *models.View) (ViewListEntryModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	entry := ViewListEntryModel{
		Name:            types.StringValue(view.Name),
		Comment:         types.StringValue(view.Comment),
		Columns:         models.TableColumnsToModel(ctx, view.Columns, &diags),
		Representations: models.ViewRepresentationsToState(view.Representations),
		DefaultCatalog:  types.StringValue(view.DefaultCatalog),
		DefaultSchema:   types.StringValue(view.DefaultSchema),
	}

	if len(view.Properties) > 0 {
		props, d := types.MapValueFrom(ctx, types.StringType, view.Properties)
		diags.Append(d...)
		entry.Properties = props
	} else {
		entry.Properties = types.MapNull(types.StringType)
	}

	auditObj, d := models.AuditToObjectValue(ctx, view.Audit)
	diags.Append(d...)
	entry.Audit = auditObj

	return entry, diags
}
