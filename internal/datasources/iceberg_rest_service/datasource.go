package iceberg_rest_service

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	minMajor = 1
	minMinor = 3
	minPatch = 1

	versionRequirement = "requires Gravitino >= 1.3.1"
)

var _ datasource.DataSource = (*IcebergRESTServiceDataSource)(nil)
var _ datasource.DataSourceWithConfigure = (*IcebergRESTServiceDataSource)(nil)

type IcebergRESTServiceDataSource struct {
	client *client.Client
}

func New() datasource.DataSource {
	return &IcebergRESTServiceDataSource{}
}

func (d *IcebergRESTServiceDataSource) SetClient(c *client.Client) {
	d.client = c
}

type IcebergRESTServiceDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	URI      types.String `tfsdk:"uri"`
}

func (d *IcebergRESTServiceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *IcebergRESTServiceDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_iceberg_rest_service"
}

func (d *IcebergRESTServiceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Discovers the Iceberg REST service endpoint advertised by the Gravitino server " +
			"(GET /api/system/iceberg-rest). The endpoint " + versionRequirement + ".",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Optional: true,
				Description: "The metalake the caller intends to route through the Iceberg REST service. " +
					"When omitted, the server reports the endpoint regardless of which metalake it serves.",
			},
			"uri": schema.StringAttribute{
				Computed: true,
				Description: "The Iceberg REST service endpoint, or null when the server advertises none for the " +
					"requested metalake. Requires Gravitino >= 1.3.1.",
			},
		},
	}
}

func (d *IcebergRESTServiceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config IcebergRESTServiceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := ""
	if !config.Metalake.IsNull() {
		metalake = config.Metalake.ValueString()
	}

	tflog.Debug(ctx, "Discovering the Iceberg REST service endpoint", map[string]interface{}{
		"metalake": metalake,
	})

	if v, err := d.client.ResolveServerVersion(ctx); err != nil {
		// The endpoint call below fails with the same transport error, so a
		// failed version lookup must not be reported as a version requirement.
		tflog.Warn(ctx, "Could not read the Gravitino server version; relying on the endpoint response", map[string]interface{}{
			"error": err.Error(),
		})
	} else if !models.ServerVersionAtLeast(v, minMajor, minMinor, minPatch) {
		resp.Diagnostics.AddError(
			"Iceberg REST service discovery is not supported by this server",
			fmt.Sprintf("The data source %s, but the connected server reports version %s.", versionRequirement, v),
		)
		return
	}

	result, err := d.client.GetIcebergRestServiceURI(ctx, metalake)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Iceberg REST service discovery is not supported by this server",
				"GET /api/system/iceberg-rest answered 404; the data source "+versionRequirement+".",
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("discovering the Iceberg REST service endpoint", "/system/iceberg-rest", err)...)
		return
	}

	if result.URI == nil {
		config.URI = types.StringNull()
	} else {
		config.URI = types.StringValue(*result.URI)
	}

	tflog.Debug(ctx, "Discovered the Iceberg REST service endpoint", map[string]interface{}{
		"metalake":  metalake,
		"available": result.URI != nil,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
