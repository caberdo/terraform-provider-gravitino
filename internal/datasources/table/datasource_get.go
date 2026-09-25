package table

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

var _ datasource.DataSource = (*tableDataSource)(nil)
var _ datasource.DataSourceWithConfigure = (*tableDataSource)(nil)

type tableDataSource struct {
	client *client.Client
}

// NewTableDataSource returns the gravitino_table data source.
func NewTableDataSource() datasource.DataSource {
	return &tableDataSource{}
}

func (d *tableDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *tableDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_table"
}

func (d *tableDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves a single Gravitino table, including the columns, sort orders, distribution, " +
			"partitioning and indexes reported by Gravitino.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Description: "The metalake the table belongs to.",
				Required:    true,
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog the table belongs to.",
				Required:    true,
			},
			"schema": schema.StringAttribute{
				Description: "The schema the table belongs to.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the table.",
				Required:    true,
			},
			"comment": schema.StringAttribute{
				Description: "The comment of the table.",
				Computed:    true,
			},
			"properties": schema.MapAttribute{
				Description: "The properties of the table as reported by Gravitino.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"audit": schema.ObjectAttribute{
				Description:    "Audit information of the table.",
				Computed:       true,
				AttributeTypes: models.AuditAttrTypes,
			},
		},
		Blocks: map[string]schema.Block{
			"column": schema.ListNestedBlock{
				Description: "A column of the table. The type is a Gravitino primitive type name such as " +
					"\"varchar(255)\", or a JSON object for the structured types.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name":           schema.StringAttribute{Computed: true},
						"type":           schema.StringAttribute{Computed: true},
						"comment":        schema.StringAttribute{Computed: true},
						"nullable":       schema.BoolAttribute{Computed: true},
						"auto_increment": schema.BoolAttribute{Computed: true},
						"default_value": schema.StringAttribute{
							Description: "The value of the column default value literal.",
							Computed:    true,
						},
					},
				},
			},
			"sort_order": schema.ListNestedBlock{
				Description: "A sort order of the table.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"field_name": schema.ListAttribute{
							Computed:    true,
							ElementType: types.StringType,
						},
						"direction":     schema.StringAttribute{Computed: true},
						"null_ordering": schema.StringAttribute{Computed: true},
					},
				},
			},
			"distribution": schema.SingleNestedBlock{
				Description: "How the data of the table is distributed. Absent when the catalog reports no distribution.",
				Attributes: map[string]schema.Attribute{
					"strategy": schema.StringAttribute{Computed: true},
					"number":   schema.Int64Attribute{Computed: true},
					"func_args": schema.ListAttribute{
						Description: "The distribution arguments as dotted field paths.",
						Computed:    true,
						ElementType: types.StringType,
					},
				},
			},
			"partitioning": schema.ListNestedBlock{
				Description: "A partitioning strategy of the table.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"strategy": schema.StringAttribute{Computed: true},
						"field_name": schema.ListAttribute{
							Computed:    true,
							ElementType: types.StringType,
						},
						"field_names": schema.ListAttribute{
							Computed:    true,
							ElementType: types.ListType{ElemType: types.StringType},
						},
						"num_buckets": schema.Int64Attribute{Computed: true},
						"width":       schema.Int64Attribute{Computed: true},
						"func_name":   schema.StringAttribute{Computed: true},
						"func_args": schema.ListAttribute{
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
			"index": schema.ListNestedBlock{
				Description: "An index of the table.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"index_type": schema.StringAttribute{Computed: true},
						"name":       schema.StringAttribute{Computed: true},
						"field_names": schema.ListAttribute{
							Computed:    true,
							ElementType: types.ListType{ElemType: types.StringType},
						},
					},
				},
			},
		},
	}
}

func (d *tableDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config models.TableDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tableResp, err := d.client.GetTable(ctx,
		config.Metalake.ValueString(),
		config.Catalog.ValueString(),
		config.Schema.ValueString(),
		config.Name.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading table", config.Name.ValueString(), err)...)
		return
	}

	state := models.TableDataSourceModel{
		Metalake: config.Metalake,
		Catalog:  config.Catalog,
		Schema:   config.Schema,
		Name:     config.Name,
	}

	mapTableToDataSourceState(ctx, &tableResp.Table, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// mapTableToDataSourceState maps a table of tables.yaml into the data source
// model. The values Gravitino reports back are normalised so that they match
// the representation of the resource: a distribution reported with the strategy
// "none" means no distribution, the index types are lower cased, and a column
// default value reported as the null literal means no default value.
func mapTableToDataSourceState(ctx context.Context, table *models.Table, state *models.TableDataSourceModel, diags *diag.Diagnostics) {
	state.Name = types.StringValue(table.Name)
	if table.Comment == "" {
		state.Comment = types.StringNull()
	} else {
		state.Comment = types.StringValue(table.Comment)
	}

	if table.Properties == nil {
		state.Properties = types.MapNull(types.StringType)
	} else {
		properties, propertyDiags := types.MapValueFrom(ctx, types.StringType, table.Properties)
		diags.Append(propertyDiags...)
		state.Properties = properties
	}

	audit, auditDiags := models.AuditToObjectValue(ctx, table.Audit)
	diags.Append(auditDiags...)
	state.Audit = audit

	state.Columns = models.TableColumnsToModel(ctx, table.Columns, diags)
	state.SortOrders = models.TableSortOrdersToModel(ctx, table.SortOrders, diags)
	state.Distribution = models.TableDistributionToModel(ctx, table.Distribution, diags)
	state.Partitioning = models.TablePartitioningToModel(ctx, table.Partitioning, diags)
	state.Indexes = models.TableIndexesToModel(ctx, table.Indexes, diags)
}
