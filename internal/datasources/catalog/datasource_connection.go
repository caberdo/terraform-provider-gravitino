package catalog

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ datasource.DataSource = &CatalogConnectionTestDataSource{}
var _ datasource.DataSourceWithConfigure = &CatalogConnectionTestDataSource{}
var _ datasource.DataSourceWithConfigValidators = &CatalogConnectionTestDataSource{}

type CatalogConnectionTestDataSource struct {
	client *client.Client
}

func NewConnectionTestDataSource() datasource.DataSource {
	return &CatalogConnectionTestDataSource{}
}

func (d *CatalogConnectionTestDataSource) SetClient(c *client.Client) {
	d.client = c
}

type CatalogConnectionTestDataSourceModel struct {
	Metalake   types.String `tfsdk:"metalake"`
	Catalog    types.String `tfsdk:"catalog"`
	Name       types.String `tfsdk:"name"`
	Type       types.String `tfsdk:"type"`
	Provider   types.String `tfsdk:"catalog_provider"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`

	Success types.Bool   `tfsdk:"success"`
	Message types.String `tfsdk:"message"`
}

func (d *CatalogConnectionTestDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *CatalogConnectionTestDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_catalog_connection_test"
}

func (d *CatalogConnectionTestDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Tests a catalog connection and exposes the result. Set `catalog` to test the stored " +
			"configuration of an existing catalog (requires Gravitino 1.3.1 or newer), or set `name` and `type` to " +
			"test a proposed catalog configuration without creating the catalog (Gravitino 1.3.0 or newer). A test " +
			"that completes with a failure sets `success` to false and explains the failure in `message`; the data " +
			"source only reports a Terraform error when the request itself fails (for example an unreachable server).",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake that owns (or would own) the catalog.",
			},
			"catalog": schema.StringAttribute{
				Optional: true,
				Description: "Name of an existing catalog whose stored configuration and effective credentials are tested " +
					"(`POST /metalakes/{metalake}/catalogs/{catalog}/testConnection`). Requires Gravitino 1.3.1 or newer. " +
					"Cannot be combined with `name`, `type`, `catalog_provider`, `comment` or `properties`.",
			},
			"name": schema.StringAttribute{
				Optional: true,
				Description: "Name of the proposed catalog configuration to test. Required together with `type`; " +
					"cannot be combined with `catalog`.",
			},
			"type": schema.StringAttribute{
				Optional: true,
				Description: "Type of the proposed catalog configuration. Must be one of: relational, fileset, messaging, " +
					"model. Required together with `name`; cannot be combined with `catalog`.",
				Validators: []validator.String{
					stringvalidator.OneOf("relational", "fileset", "messaging", "model"),
				},
			},
			"catalog_provider": schema.StringAttribute{
				Optional: true,
				Description: "Provider of the proposed catalog configuration, for example `hive`, `lakehouse-iceberg`, " +
					"`jdbc-mysql` or `fileset`. Cannot be combined with `catalog`.",
				Validators: []validator.String{
					stringvalidator.OneOf("hive", "lakehouse-iceberg", "lakehouse-paimon", "lakehouse-hudi", "jdbc-mysql", "jdbc-postgresql", "jdbc-doris", "jdbc-oceanbase", "kafka", "fileset", "model"),
				},
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Description: "Comment of the proposed catalog configuration. Cannot be combined with `catalog`.",
			},
			"properties": schema.MapAttribute{
				Optional:    true,
				Sensitive:   true,
				ElementType: types.StringType,
				Description: "Properties of the proposed catalog configuration, for example `metastore.uris`. May contain " +
					"credentials, therefore it is marked sensitive. Cannot be combined with `catalog`.",
			},
			"success": schema.BoolAttribute{
				Computed:    true,
				Description: "True when the connection test succeeded (server application code 0).",
			},
			"message": schema.StringAttribute{
				Computed:    true,
				Description: "Sanitized failure message returned by the server; empty when the test succeeded.",
			},
		},
	}
}

// ConfigValidators validates the parts of the configuration Terraform cannot
// express in the schema: exactly one of the two variants must be configured.
func (d *CatalogConnectionTestDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{catalogConnectionTestVariantValidator{}}
}

func (d *CatalogConnectionTestDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CatalogConnectionTestDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := config.Metalake.ValueString()
	catalogName := config.Catalog.ValueString()

	var (
		result *models.CatalogTestConnectionResponse
		err    error
		target string
	)

	if catalogName != "" {
		target = catalogName
		if !d.existingCatalogSupported(ctx, resp) {
			return
		}

		tflog.Debug(ctx, "Testing existing catalog connection", map[string]interface{}{
			"metalake": metalake,
			"catalog":  catalogName,
		})
		result, err = d.client.TestCatalogConnection(ctx, metalake, catalogName, nil)
	} else {
		target = config.Name.ValueString()

		var properties map[string]string
		if !config.Properties.IsNull() && !config.Properties.IsUnknown() {
			resp.Diagnostics.Append(config.Properties.ElementsAs(ctx, &properties, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
		}

		tflog.Debug(ctx, "Testing proposed catalog configuration", map[string]interface{}{
			"metalake": metalake,
			"name":     target,
			"type":     config.Type.ValueString(),
		})
		result, err = d.client.TestCatalogConfig(ctx, metalake, &models.CatalogCreateRequest{
			Name:       target,
			Type:       config.Type.ValueString(),
			Provider:   config.Provider.ValueString(),
			Comment:    config.Comment.ValueString(),
			Properties: properties,
		})
	}

	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("testing the connection of catalog", target, err)...)
		return
	}

	tflog.Debug(ctx, "Tested catalog connection", map[string]interface{}{
		"metalake": metalake,
		"catalog":  target,
		"code":     result.Code,
		"success":  result.Code == 0,
	})

	config.Success = types.BoolValue(result.Code == 0)
	config.Message = types.StringValue(result.Message)

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

// existingCatalogSupported reports whether the server implements
// POST /metalakes/{metalake}/catalogs/{catalog}/testConnection, which was added
// in Gravitino 1.3.1. It resolves the version through client.ResolveServerVersion
// so a gate normally costs no request of its own. A version the provider cannot
// read counts as unsupported (models.ServerVersionAtLeast): the caller is about
// to use an API that only exists from 1.3.1 on, so an unknown server version
// must not be assumed to support it.
func (d *CatalogConnectionTestDataSource) existingCatalogSupported(ctx context.Context, resp *datasource.ReadResponse) bool {
	reported, err := d.client.ResolveServerVersion(ctx)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading the Gravitino server version", "catalog connection test", err)...)
		return false
	}

	if models.ServerVersionAtLeast(reported, 1, 3, 1) {
		return true
	}

	resp.Diagnostics.AddError(
		"Unsupported Gravitino version",
		fmt.Sprintf("Testing an existing catalog connection (`catalog`) requires Gravitino >= 1.3.1, but the server reports version %q. Set `name` and `type` instead to test a proposed catalog configuration, which works on Gravitino 1.3.0 as well.", reported),
	)
	return false
}

// catalogConnectionTestVariantValidator enforces that exactly one of the two
// connection test variants is configured: an existing catalog (`catalog`), or a
// proposed catalog configuration (`name` and `type` plus the optional
// `catalog_provider`, `comment` and `properties`).
type catalogConnectionTestVariantValidator struct{}

func (v catalogConnectionTestVariantValidator) Description(_ context.Context) string {
	return "either `catalog` (test an existing catalog) or `name` and `type` (test a proposed catalog configuration) must be configured, but not both"
}

func (v catalogConnectionTestVariantValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v catalogConnectionTestVariantValidator) ValidateDataSource(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config CatalogConnectionTestDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// An unknown `catalog` resolves to a concrete value only during apply; the
	// variant cannot be checked yet.
	if config.Catalog.IsUnknown() {
		return
	}

	// An empty string is treated as "not set", matching Read.
	if configured(config.Catalog) && config.Catalog.ValueString() != "" {
		proposed := []struct {
			name string
			set  bool
		}{
			{"name", configured(config.Name)},
			{"type", configured(config.Type)},
			{"catalog_provider", configured(config.Provider)},
			{"comment", configured(config.Comment)},
			{"properties", !config.Properties.IsNull() && !config.Properties.IsUnknown()},
		}
		for _, attr := range proposed {
			if attr.set {
				resp.Diagnostics.AddAttributeError(
					path.Root(attr.name),
					"Conflicting catalog connection test configuration",
					fmt.Sprintf("`catalog` tests an existing catalog and cannot be combined with `%s`, which belongs to a proposed configuration. Remove one of them.", attr.name),
				)
			}
		}
		return
	}

	if config.Name.IsUnknown() || config.Type.IsUnknown() {
		return
	}

	if !configured(config.Name) || !configured(config.Type) {
		resp.Diagnostics.AddAttributeError(
			path.Root("name"),
			"Missing catalog connection test configuration",
			"Set `catalog` to test an existing catalog connection, or set both `name` and `type` to test a proposed catalog configuration.",
		)
	}
}

// configured reports whether an optional string attribute carries a value that
// is known at validation time.
func configured(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown()
}
