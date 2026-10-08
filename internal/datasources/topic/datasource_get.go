package topic

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

var _ datasource.DataSource = &TopicDataSource{}
var _ datasource.DataSourceWithConfigure = &TopicDataSource{}

var dsAuditAttrTypes = models.AuditAttrTypes

type TopicDataSource struct {
	client *client.Client
}

type TopicDataSourceModel struct {
	Metalake   types.String `tfsdk:"metalake"`
	Catalog    types.String `tfsdk:"catalog"`
	Schema     types.String `tfsdk:"schema"`
	Name       types.String `tfsdk:"name"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
}

func NewTopicDataSource() datasource.DataSource {
	return &TopicDataSource{}
}

func (d *TopicDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_topic"
}

func (d *TopicDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves a single Gravitino topic by name.",
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
				Description: "The topic name.",
				Required:    true,
			},
			"comment": schema.StringAttribute{
				Description: "The topic comment.",
				Computed:    true,
			},
			"properties": schema.MapAttribute{
				Description: "Key-value properties for the topic.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"audit": schema.ObjectAttribute{
				Description:    "Audit information for the topic.",
				Computed:       true,
				AttributeTypes: dsAuditAttrTypes,
			},
		},
	}
}

func (ds *TopicDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		ds.client = c
	}
}

func (ds *TopicDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TopicDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	topicResp, err := ds.client.GetTopic(ctx, config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString(), config.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Topic not found",
				fmt.Sprintf("No Gravitino topic %q exists in %s.%s.%s.", config.Name.ValueString(), config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading topic", config.Name.ValueString(), err)...)
		return
	}

	if topicResp.Topic.Comment != "" {
		config.Comment = types.StringValue(topicResp.Topic.Comment)
	} else {
		config.Comment = types.StringNull()
	}

	if len(topicResp.Topic.Properties) > 0 {
		props, d := types.MapValueFrom(ctx, types.StringType, topicResp.Topic.Properties)
		resp.Diagnostics.Append(d...)
		config.Properties = props
	} else {
		config.Properties = types.MapNull(types.StringType)
	}

	auditObj, d := dsAuditToObject(topicResp.Topic.Audit)
	resp.Diagnostics.Append(d...)
	config.Audit = auditObj

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func dsAuditToObject(audit *models.Audit) (types.Object, diag.Diagnostics) {
	return models.AuditToObjectValue(context.Background(), audit)
}
