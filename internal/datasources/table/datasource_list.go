package table

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = (*tablesDataSource)(nil)
var _ datasource.DataSourceWithConfigure = (*tablesDataSource)(nil)

type tablesDataSource struct {
	client *client.Client
}

func NewTablesDataSource() datasource.DataSource {
	return &tablesDataSource{}
}

type tablesDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Catalog  types.String `tfsdk:"catalog"`
	Schema   types.String `tfsdk:"schema"`
	Tables   types.List   `tfsdk:"tables"`
}

func (d *tablesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *tablesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tables"
}

func (d *tablesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required: true,
			},
			"catalog": schema.StringAttribute{
				Required: true,
			},
			"schema": schema.StringAttribute{
				Required: true,
			},
			"tables": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *tablesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config tablesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	listResp, err := d.client.ListTables(ctx,
		config.Metalake.ValueString(),
		config.Catalog.ValueString(),
		config.Schema.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("listing tables", config.Schema.ValueString(), err)...)
		return
	}

	tableNames := make([]attr.Value, 0, len(listResp.Identifiers))
	for _, ident := range listResp.Identifiers {
		tableNames = append(tableNames, types.StringValue(ident.Name))
	}

	tablesList, diags := types.ListValue(types.StringType, tableNames)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Tables = tablesList
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
