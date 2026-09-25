package credential

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ datasource.DataSource = &CredentialsDataSource{}
var _ datasource.DataSourceWithConfigure = &CredentialsDataSource{}

type CredentialsDataSource struct {
	client *client.Client
}

func New() datasource.DataSource {
	return &CredentialsDataSource{}
}

func (d *CredentialsDataSource) SetClient(c *client.Client) {
	d.client = c
}

type CredentialsDataSourceModel struct {
	Metalake     types.String `tfsdk:"metalake"`
	ResourceType types.String `tfsdk:"resource_type"`
	Resource     types.String `tfsdk:"resource"`
	Credentials  types.List   `tfsdk:"credentials"`
}

// CredentialItemModel is a single element of the `credentials` list. It mirrors
// the `Credential` schema of the Gravitino v1.3.0 credentials API.
type CredentialItemModel struct {
	CredentialType types.String `tfsdk:"credential_type"`
	ExpireTimeInMs types.Int64  `tfsdk:"expire_time_in_ms"`
	CredentialInfo types.Map    `tfsdk:"credential_info"`
}

// CredentialItemAttrTypes are the attribute types of CredentialItemModel.
var CredentialItemAttrTypes = map[string]attr.Type{
	"credential_type":   types.StringType,
	"expire_time_in_ms": types.Int64Type,
	"credential_info":   types.MapType{ElemType: types.StringType},
}

func (d *CredentialsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CredentialsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_credentials"
}

func (d *CredentialsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Gets the credentials associated with a metadata object.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"resource_type": schema.StringAttribute{
				Required:    true,
				Description: "The metadata object type (METALAKE, CATALOG, SCHEMA, TABLE, COLUMN, FILESET, TOPIC, MODEL, ROLE).",
				Validators: []validator.String{
					stringvalidator.OneOf(models.CredentialObjectTypes...),
				},
			},
			"resource": schema.StringAttribute{
				Required:    true,
				Description: "The full name of the metadata object (for example hive_catalog.example_schema.users).",
			},
			"credentials": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The credentials associated with the metadata object.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"credential_type": schema.StringAttribute{
							Computed:    true,
							Description: "The type of the credential, for example s3-token, s3-secret-key, oss-token, oss-secret-key, gcs-token, adls-token, azure-account-key.",
						},
						"expire_time_in_ms": schema.Int64Attribute{
							Computed:    true,
							Description: "The expiration time of the credential in milliseconds since the epoch. 0 means the credential does not expire.",
						},
						"credential_info": schema.MapAttribute{
							Computed:    true,
							Sensitive:   true,
							ElementType: types.StringType,
							Description: "The specific information of the credential.",
						},
					},
				},
			},
		},
	}
}

func (d *CredentialsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CredentialsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := config.Metalake.ValueString()
	resourceType := config.ResourceType.ValueString()
	resource := config.Resource.ValueString()

	tflog.Debug(ctx, "Reading credentials", map[string]interface{}{
		"metalake":      metalake,
		"resource_type": resourceType,
		"resource":      resource,
	})

	result, err := d.client.GetCredentials(ctx, metalake, resourceType, resource)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading credentials", resource, err)...)
		return
	}

	credentials, diags := credentialsToItemModels(ctx, result.Credentials)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.Credentials = credentials

	tflog.Debug(ctx, "Read credentials", map[string]interface{}{
		"metalake":      metalake,
		"resource_type": resourceType,
		"resource":      resource,
		"count":         len(result.Credentials),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

// credentialsToItemModels maps the API credentials onto Terraform list
// elements. Every attribute is set to a known value: a credential without
// `credentialInfo` becomes an empty map (the schema default in the spec) and an
// empty response becomes an empty list, never an unknown value.
func credentialsToItemModels(ctx context.Context, creds []models.Credential) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics

	items := make([]attr.Value, 0, len(creds))
	for _, cred := range creds {
		item, itemDiags := types.ObjectValueFrom(ctx, CredentialItemAttrTypes, CredentialItemModel{
			CredentialType: types.StringValue(cred.CredentialType),
			ExpireTimeInMs: types.Int64Value(cred.ExpireTimeInMs),
			CredentialInfo: credentialInfoValue(cred.CredentialInfo),
		})
		diags.Append(itemDiags...)
		if diags.HasError() {
			return types.ListNull(types.ObjectType{AttrTypes: CredentialItemAttrTypes}), diags
		}
		items = append(items, item)
	}

	list, listDiags := types.ListValue(types.ObjectType{AttrTypes: CredentialItemAttrTypes}, items)
	diags.Append(listDiags...)
	return list, diags
}

// credentialInfoValue converts the credential info map to a known map value;
// nil becomes an empty map.
func credentialInfoValue(info map[string]string) types.Map {
	elems := make(map[string]attr.Value, len(info))
	for k, v := range info {
		elems[k] = types.StringValue(v)
	}
	return types.MapValueMust(types.StringType, elems)
}
