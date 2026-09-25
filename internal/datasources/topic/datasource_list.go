package topic

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

var _ datasource.DataSource = &TopicsDataSource{}
var _ datasource.DataSourceWithConfigure = &TopicsDataSource{}

var dslAuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

type TopicsDataSource struct {
	client *client.Client
}

type TopicsDataSourceModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Catalog  types.String `tfsdk:"catalog"`
	Schema   types.String `tfsdk:"schema"`
	Topics   types.List   `tfsdk:"topics"`
}

func NewTopicsDataSource() datasource.DataSource {
	return &TopicsDataSource{}
}

func (d *TopicsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_topics"
}

func (d *TopicsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all topics within a Gravitino metalake, catalog, and schema.",
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
			"topics": schema.ListNestedAttribute{
				Description: "List of topics with their details.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The topic name.",
							Computed:    true,
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
							AttributeTypes: dslAuditAttrTypes,
						},
					},
				},
			},
		},
	}
}

func (ds *TopicsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (ds *TopicsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TopicsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	topics, err := ds.client.ListTopicsDetails(ctx, config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				"Metalake, catalog or schema not found",
				fmt.Sprintf("Cannot list topics of %s.%s.%s: it does not exist.", config.Metalake.ValueString(), config.Catalog.ValueString(), config.Schema.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("listing topics", config.Schema.ValueString(), err)...)
		return
	}

	items := make([]attr.Value, 0, len(topics))
	for i := range topics {
		item, d := dslTopicListItemToObject(ctx, &topics[i])
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		items = []attr.Value{}
	}

	listVal, d := types.ListValue(
		types.ObjectType{AttrTypes: dslTopicListItemAttrTypes()},
		items,
	)
	resp.Diagnostics.Append(d...)
	config.Topics = listVal

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func dslTopicListItemAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":       types.StringType,
		"comment":    types.StringType,
		"properties": types.MapType{ElemType: types.StringType},
		"audit":      types.ObjectType{AttrTypes: dslAuditAttrTypes},
	}
}

func dslTopicListItemToObject(ctx context.Context, t *models.Topic) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	var props types.Map
	if len(t.Properties) > 0 {
		p, d := types.MapValueFrom(ctx, types.StringType, t.Properties)
		diags.Append(d...)
		props = p
	} else {
		props = types.MapNull(types.StringType)
	}

	auditObj, d := dslAuditToObject(t.Audit)
	diags.Append(d...)

	obj, d := types.ObjectValue(dslTopicListItemAttrTypes(), map[string]attr.Value{
		"name":       types.StringValue(t.Name),
		"comment":    commentToValue(t.Comment),
		"properties": props,
		"audit":      auditObj,
	})
	diags.Append(d...)

	return obj, diags
}

// commentToValue maps an absent/empty server comment to null so the data source
// never invents an empty string where the API reports no value.
func commentToValue(comment string) types.String {
	if comment == "" {
		return types.StringNull()
	}
	return types.StringValue(comment)
}

func dslAuditToObject(audit *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(dslAuditAttrTypes), nil
	}

	creator := types.StringNull()
	if audit.Creator != "" {
		creator = types.StringValue(audit.Creator)
	}

	createTime := types.StringNull()
	if audit.CreateTime != nil {
		createTime = types.StringValue(audit.CreateTime.Format(time.RFC3339))
	}

	lastModifier := types.StringNull()
	if audit.LastModifier != "" {
		lastModifier = types.StringValue(audit.LastModifier)
	}

	lastModifiedTime := types.StringNull()
	if audit.LastModifiedTime != nil {
		lastModifiedTime = types.StringValue(audit.LastModifiedTime.Format(time.RFC3339))
	}

	return types.ObjectValue(dslAuditAttrTypes, map[string]attr.Value{
		"creator":            creator,
		"create_time":        createTime,
		"last_modifier":      lastModifier,
		"last_modified_time": lastModifiedTime,
	})
}
