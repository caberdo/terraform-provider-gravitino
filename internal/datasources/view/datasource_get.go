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

var _ datasource.DataSource = &ViewDataSource{}
var _ datasource.DataSourceWithConfigure = &ViewDataSource{}

type ViewDataSource struct {
	client *client.Client
}

type ViewDataSourceModel struct {
	Metalake        types.String                     `tfsdk:"metalake"`
	Catalog         types.String                     `tfsdk:"catalog"`
	Schema          types.String                     `tfsdk:"schema"`
	Name            types.String                     `tfsdk:"name"`
	Comment         types.String                     `tfsdk:"comment"`
	Columns         []models.ColumnTFSDK             `tfsdk:"column"`
	Representations []models.ViewRepresentationTFSDK `tfsdk:"representation"`
	DefaultCatalog  types.String                     `tfsdk:"default_catalog"`
	DefaultSchema   types.String                     `tfsdk:"default_schema"`
	Properties      types.Map                        `tfsdk:"properties"`
	Audit           types.Object                     `tfsdk:"audit"`
}

func NewViewDataSource() datasource.DataSource {
	return &ViewDataSource{}
}

func (d *ViewDataSource) SetClient(c *client.Client) {
	d.client = c
}

func (d *ViewDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_view"
}

func (d *ViewDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves a single Gravitino view by name.",
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
			"name": schema.StringAttribute{
				Description: "The view name.",
				Required:    true,
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
	}
}

func (ds *ViewDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (ds *ViewDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ViewDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	viewResp, err := ds.client.GetView(ctx, config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString(), config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading view", config.Name.ValueString(), err)...)
		return
	}

	view := viewResp.View

	config.Name = types.StringValue(view.Name)
	config.Comment = types.StringValue(view.Comment)
	var colDiags diag.Diagnostics
	config.Columns = models.TableColumnsToModel(ctx, view.Columns, &colDiags)
	resp.Diagnostics.Append(colDiags...)
	config.Representations = models.ViewRepresentationsToState(view.Representations)
	config.DefaultCatalog = types.StringValue(view.DefaultCatalog)
	config.DefaultSchema = types.StringValue(view.DefaultSchema)

	if len(view.Properties) > 0 {
		props, d := types.MapValueFrom(ctx, types.StringType, view.Properties)
		resp.Diagnostics.Append(d...)
		config.Properties = props
	} else {
		config.Properties = types.MapNull(types.StringType)
	}

	auditObj, d := models.AuditToObjectValue(ctx, view.Audit)
	resp.Diagnostics.Append(d...)
	config.Audit = auditObj

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
