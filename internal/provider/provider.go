package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/client/auth"
	dsauthentication "github.com/gravitino/terraform-provider-gravitino/internal/datasources/authentication"
	dscatalog "github.com/gravitino/terraform-provider-gravitino/internal/datasources/catalog"
	dscredential "github.com/gravitino/terraform-provider-gravitino/internal/datasources/credential"
	dsfileset "github.com/gravitino/terraform-provider-gravitino/internal/datasources/fileset"
	dsfunction "github.com/gravitino/terraform-provider-gravitino/internal/datasources/function"
	dsgroup "github.com/gravitino/terraform-provider-gravitino/internal/datasources/group"
	dshealth "github.com/gravitino/terraform-provider-gravitino/internal/datasources/health"
	dsidpgroup "github.com/gravitino/terraform-provider-gravitino/internal/datasources/idp_group"
	dsidpuser "github.com/gravitino/terraform-provider-gravitino/internal/datasources/idp_user"
	dsjob "github.com/gravitino/terraform-provider-gravitino/internal/datasources/job"
	dsjobtemplate "github.com/gravitino/terraform-provider-gravitino/internal/datasources/job_template"
	dsmetalake "github.com/gravitino/terraform-provider-gravitino/internal/datasources/metalake"
	dsmodel "github.com/gravitino/terraform-provider-gravitino/internal/datasources/model"
	dsmodelversion "github.com/gravitino/terraform-provider-gravitino/internal/datasources/model_version"
	dsowner "github.com/gravitino/terraform-provider-gravitino/internal/datasources/owner"
	dspartition "github.com/gravitino/terraform-provider-gravitino/internal/datasources/partition"
	dspolicy "github.com/gravitino/terraform-provider-gravitino/internal/datasources/policy"
	dsrole "github.com/gravitino/terraform-provider-gravitino/internal/datasources/role"
	dsschema "github.com/gravitino/terraform-provider-gravitino/internal/datasources/schema"
	dssecrets "github.com/gravitino/terraform-provider-gravitino/internal/datasources/secrets"
	dsstatistics "github.com/gravitino/terraform-provider-gravitino/internal/datasources/statistics"
	dstable "github.com/gravitino/terraform-provider-gravitino/internal/datasources/table"
	dstag "github.com/gravitino/terraform-provider-gravitino/internal/datasources/tag"
	dstopic "github.com/gravitino/terraform-provider-gravitino/internal/datasources/topic"
	dsuser "github.com/gravitino/terraform-provider-gravitino/internal/datasources/user"
	dsview "github.com/gravitino/terraform-provider-gravitino/internal/datasources/view"
	rscatalog "github.com/gravitino/terraform-provider-gravitino/internal/resources/catalog"
	rsfileset "github.com/gravitino/terraform-provider-gravitino/internal/resources/fileset"
	rsfunction "github.com/gravitino/terraform-provider-gravitino/internal/resources/function"
	rsgroup "github.com/gravitino/terraform-provider-gravitino/internal/resources/group"
	rsidpgroup "github.com/gravitino/terraform-provider-gravitino/internal/resources/idp_group"
	rsidpuser "github.com/gravitino/terraform-provider-gravitino/internal/resources/idp_user"
	rsjob "github.com/gravitino/terraform-provider-gravitino/internal/resources/job"
	rsjobtemplate "github.com/gravitino/terraform-provider-gravitino/internal/resources/job_template"
	rsmetalake "github.com/gravitino/terraform-provider-gravitino/internal/resources/metalake"
	rsmodel "github.com/gravitino/terraform-provider-gravitino/internal/resources/model"
	rsmodelversion "github.com/gravitino/terraform-provider-gravitino/internal/resources/model_version"
	rsowner "github.com/gravitino/terraform-provider-gravitino/internal/resources/owner"
	rspartition "github.com/gravitino/terraform-provider-gravitino/internal/resources/partition"
	rspolicy "github.com/gravitino/terraform-provider-gravitino/internal/resources/policy"
	rsrole "github.com/gravitino/terraform-provider-gravitino/internal/resources/role"
	rsschema "github.com/gravitino/terraform-provider-gravitino/internal/resources/schema"
	rstable "github.com/gravitino/terraform-provider-gravitino/internal/resources/table"
	rstag "github.com/gravitino/terraform-provider-gravitino/internal/resources/tag"
	rstopic "github.com/gravitino/terraform-provider-gravitino/internal/resources/topic"
	rsuser "github.com/gravitino/terraform-provider-gravitino/internal/resources/user"
	rsview "github.com/gravitino/terraform-provider-gravitino/internal/resources/view"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = (*GravitinoProvider)(nil)

type GravitinoProvider struct {
	version string
}

type GravitinoProviderModel struct {
	URI  types.String `tfsdk:"uri"`
	Auth types.String `tfsdk:"auth"`

	// Simple / Basic
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`

	// OAuth static + client credentials
	OAuthToken        types.String `tfsdk:"oauth_token"`
	OAuthClientID     types.String `tfsdk:"oauth_client_id"`
	OAuthClientSecret types.String `tfsdk:"oauth_client_secret"`
	OAuthServerURI    types.String `tfsdk:"oauth_server_uri"`
	OAuthTokenPath    types.String `tfsdk:"oauth_token_path"`
	OAuthScope        types.String `tfsdk:"oauth_scope"`

	// Kerberos
	KerberosPrincipal      types.String `tfsdk:"kerberos_principal"`
	KerberosKeytab         types.String `tfsdk:"kerberos_keytab"`
	KerberosUseTicketCache types.Bool   `tfsdk:"kerberos_use_ticket_cache"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &GravitinoProvider{
			version: version,
		}
	}
}

func (p *GravitinoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "gravitino"
	resp.Version = p.version
}

func (p *GravitinoProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"uri": schema.StringAttribute{
				Optional:    true,
				Description: "The URI of the Gravitino server. Can also be set via GRAVITINO_URI environment variable.",
			},
			"auth": schema.StringAttribute{
				Optional:    true,
				Description: "Authentication method: 'simple', 'basic', 'oauth', or 'kerberos'. Can also be set via GRAVITINO_AUTH environment variable.",
				Validators: []validator.String{
					stringvalidator.OneOf("simple", "basic", "oauth", "kerberos", "none"),
				},
			},
			"username": schema.StringAttribute{
				Optional:    true,
				Description: "Username for simple/basic authentication. Can also be set via GRAVITINO_USERNAME environment variable.",
			},
			"password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Password for basic authentication. Can also be set via GRAVITINO_PASSWORD environment variable.",
			},
			"oauth_token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "OAuth2 bearer token. Can also be set via GRAVITINO_OAUTH_TOKEN environment variable.",
			},
			"oauth_client_id": schema.StringAttribute{
				Optional:    true,
				Description: "OAuth2 client ID for client credentials flow. Can also be set via GRAVITINO_OAUTH_CLIENT_ID environment variable.",
			},
			"oauth_client_secret": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "OAuth2 client secret for client credentials flow. Can also be set via GRAVITINO_OAUTH_CLIENT_SECRET environment variable.",
			},
			"oauth_server_uri": schema.StringAttribute{
				Optional:    true,
				Description: "OAuth2 server URI for client credentials flow. Can also be set via GRAVITINO_OAUTH_SERVER_URI environment variable.",
			},
			"oauth_token_path": schema.StringAttribute{
				Optional:    true,
				Description: "OAuth2 token endpoint path (e.g. /oauth2/token). Can also be set via GRAVITINO_OAUTH_TOKEN_PATH environment variable.",
			},
			"oauth_scope": schema.StringAttribute{
				Optional:    true,
				Description: "OAuth2 scope for client credentials flow. Can also be set via GRAVITINO_OAUTH_SCOPE environment variable.",
			},
			"kerberos_principal": schema.StringAttribute{
				Optional:    true,
				Description: "Kerberos principal (e.g. HTTP/server@REALM). Can also be set via GRAVITINO_KERBEROS_PRINCIPAL environment variable.",
			},
			"kerberos_keytab": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Path to Kerberos keytab file. Can also be set via GRAVITINO_KERBEROS_KEYTAB environment variable.",
			},
			"kerberos_use_ticket_cache": schema.BoolAttribute{
				Optional:    true,
				Description: "Use Kerberos ticket cache instead of keytab. Can also be set via GRAVITINO_KERBEROS_USE_TICKET_CACHE environment variable.",
			},
		},
	}
}

func (p *GravitinoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config GravitinoProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	uri := os.Getenv("GRAVITINO_URI")
	if !config.URI.IsNull() {
		uri = config.URI.ValueString()
	}
	if uri == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("uri"),
			"Missing Gravitino URI",
			"Set the uri attribute in the provider block or the GRAVITINO_URI environment variable.",
		)
		return
	}

	authMethod := readConfigString(config.Auth, "GRAVITINO_AUTH")
	username := readConfigString(config.Username, "GRAVITINO_USERNAME")
	password := readConfigString(config.Password, "GRAVITINO_PASSWORD")
	oauthToken := readConfigString(config.OAuthToken, "GRAVITINO_OAUTH_TOKEN")
	oauthClientID := readConfigString(config.OAuthClientID, "GRAVITINO_OAUTH_CLIENT_ID")
	oauthClientSecret := readConfigString(config.OAuthClientSecret, "GRAVITINO_OAUTH_CLIENT_SECRET")
	oauthServerURI := readConfigString(config.OAuthServerURI, "GRAVITINO_OAUTH_SERVER_URI")
	oauthTokenPath := readConfigString(config.OAuthTokenPath, "GRAVITINO_OAUTH_TOKEN_PATH")
	oauthScope := readConfigString(config.OAuthScope, "GRAVITINO_OAUTH_SCOPE")
	kerberosPrincipal := readConfigString(config.KerberosPrincipal, "GRAVITINO_KERBEROS_PRINCIPAL")
	kerberosKeytab := readConfigString(config.KerberosKeytab, "GRAVITINO_KERBEROS_KEYTAB")

	kerberosUseTicketCache := false
	if envVal := os.Getenv("GRAVITINO_KERBEROS_USE_TICKET_CACHE"); envVal != "" {
		parsed, err := strconv.ParseBool(envVal)
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("kerberos_use_ticket_cache"),
				"Invalid GRAVITINO_KERBEROS_USE_TICKET_CACHE value",
				fmt.Sprintf("Expected a boolean, got %q.", envVal),
			)
			return
		}
		kerberosUseTicketCache = parsed
	}
	if !config.KerberosUseTicketCache.IsNull() {
		kerberosUseTicketCache = config.KerberosUseTicketCache.ValueBool()
	}

	ap, err := buildAuthProvider(authMethod, username, password, oauthToken,
		oauthClientID, oauthClientSecret, oauthServerURI, oauthTokenPath, oauthScope,
		kerberosPrincipal, kerberosKeytab, kerberosUseTicketCache)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("auth"),
			"Invalid authentication configuration",
			err.Error(),
		)
		return
	}

	c, err := client.New(uri, ap)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create client", err.Error())
		return
	}

	resp.DataSourceData = c
	resp.ResourceData = c
}

func readConfigString(val types.String, envVar string) string {
	if !val.IsNull() {
		return val.ValueString()
	}
	return os.Getenv(envVar)
}

func buildAuthProvider(authMethod, username, password, oauthToken, oauthClientID, oauthClientSecret, oauthServerURI, oauthTokenPath, oauthScope, kerberosPrincipal, kerberosKeytab string, kerberosUseTicketCache bool) (auth.AuthProvider, error) {
	switch authMethod {
	case "", "none":
		return nil, nil
	case "simple":
		return auth.NewSimpleProvider(username), nil
	case "basic":
		if username == "" {
			return nil, fmt.Errorf("username is required for basic authentication")
		}
		return auth.NewBasicProvider(username, password), nil
	case "oauth":
		if oauthToken != "" {
			return auth.NewOAuthStaticProvider(oauthToken), nil
		}
		if oauthClientID != "" && oauthClientSecret != "" && oauthServerURI != "" && oauthTokenPath != "" {
			return auth.NewOAuthCredentialsProvider(oauthClientID, oauthClientSecret, oauthServerURI, oauthTokenPath, oauthScope), nil
		}
		return nil, fmt.Errorf("oauth requires either oauth_token (static) or oauth_client_id + oauth_client_secret + oauth_server_uri + oauth_token_path (client credentials)")
	case "kerberos":
		if kerberosPrincipal == "" {
			return nil, fmt.Errorf("kerberos_principal is required for kerberos authentication")
		}
		return auth.NewKerberosProvider(kerberosPrincipal, kerberosKeytab, kerberosUseTicketCache)
	default:
		return nil, fmt.Errorf("unknown authentication method: %q", authMethod)
	}
}

func (p *GravitinoProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		dsmetalake.NewMetalakesDataSource,
		dsmetalake.NewMetalakeDataSource,
		dscatalog.NewListDataSource,
		dscatalog.NewGetDataSource,
		dsjob.NewListDataSource,
		dsjob.NewGetDataSource,
		dsjobtemplate.NewJobTemplateDataSource,
		dsjobtemplate.NewJobTemplatesDataSource,
		dsschema.NewSchemaDataSource,
		dsschema.NewSchemasDataSource,
		dsmodel.NewModelDataSource,
		dsmodel.NewModelsDataSource,
		dsmodelversion.NewModelVersionDataSource,
		dsmodelversion.NewModelVersionsDataSource,
		dstag.NewListDataSource,
		dstag.NewGetDataSource,
		dsview.NewViewsDataSource,
		dsview.NewViewDataSource,
		dsfunction.NewFunctionsDataSource,
		dsfunction.NewFunctionDataSource,
		dsfileset.NewListDataSource,
		dsfileset.NewGetDataSource,
		dstopic.NewTopicsDataSource,
		dstopic.NewTopicDataSource,
		dsgroup.NewListDataSource,
		dsgroup.NewGetDataSource,
		dspartition.NewPartitionsDataSource,
		dspartition.NewPartitionDataSource,
		dstable.NewTablesDataSource,
		dstable.NewTableDataSource,
		dshealth.NewHealthDataSource,
		dshealth.NewLivenessDataSource,
		dshealth.NewReadinessDataSource,
		dspolicy.NewListDataSource,
		dscredential.New,
		dssecrets.New,
		dsrole.New,
		dsrole.NewRoleDataSource,
		dsrole.NewRolesListDataSource,
		dsstatistics.New,
		dsstatistics.NewPartition,
		dsauthentication.New,
		dsowner.NewOwnerDataSource,
		dsuser.NewListDataSource,
		dsuser.NewGetDataSource,
		dsidpuser.NewDataSource,
		dsidpgroup.NewDataSource,
	}
}

func (p *GravitinoProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		rsmetalake.NewMetalakeResource,
		rscatalog.New,
		rsfileset.New,
		rsjob.New,
		rsjobtemplate.NewJobTemplateResource,
		rsschema.NewSchemaResource,
		rsmodel.New,
		rsmodelversion.NewModelVersionResource,
		rsowner.NewOwnerResource,
		rstable.NewTableResource,
		rstag.New,
		rstopic.NewTopicResource,
		rsview.NewViewResource,
		rsfunction.New,
		rsgroup.New,
		rspartition.NewPartitionResource,
		rspolicy.New,
		rsrole.NewRoleResource,
		rsuser.New,
		rsidpuser.New,
		rsidpgroup.New,
	}
}
