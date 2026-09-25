package job_template

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// timeFormat matches the timestamp format used by the other resources.
const timeFormat = "2006-01-02T15:04:05Z07:00"

var _ datasource.DataSource = &JobTemplatesDataSource{}
var _ datasource.DataSourceWithConfigure = &JobTemplatesDataSource{}

type JobTemplatesDataSource struct {
	client *client.Client
}

func NewJobTemplatesDataSource() datasource.DataSource {
	return &JobTemplatesDataSource{}
}

func (d *JobTemplatesDataSource) SetClient(c *client.Client) {
	d.client = c
}

type JobTemplatesDataSourceModel struct {
	Metalake     types.String `tfsdk:"metalake"`
	JobTemplates types.List   `tfsdk:"job_templates"`
}

var JobTemplateItemAttrTypes = map[string]attr.Type{
	"name":          types.StringType,
	"job_type":      types.StringType,
	"comment":       types.StringType,
	"executable":    types.StringType,
	"arguments":     types.ListType{ElemType: types.StringType},
	"environments":  types.MapType{ElemType: types.StringType},
	"custom_fields": types.MapType{ElemType: types.StringType},
	"scripts":       types.ListType{ElemType: types.StringType},
	"class_name":    types.StringType,
	"jars":          types.ListType{ElemType: types.StringType},
	"files":         types.ListType{ElemType: types.StringType},
	"archives":      types.ListType{ElemType: types.StringType},
	"configs":       types.MapType{ElemType: types.StringType},
	"audit":         types.ObjectType{AttrTypes: AuditAttrTypes},
}

func (d *JobTemplatesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	d.client = c
}

func (d *JobTemplatesDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_job_templates"
}

func (d *JobTemplatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	itemAttributes := jobTemplateAttributes()
	itemAttributes["name"] = schema.StringAttribute{
		Computed:    true,
		Description: "The job template name.",
	}

	resp.Schema = schema.Schema{
		Description: "Lists the job templates of a metalake.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"job_templates": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: itemAttributes,
				},
			},
		},
	}
}

func (d *JobTemplatesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config JobTemplatesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := config.Metalake.ValueString()

	result, err := d.client.ListJobTemplates(ctx, metalake)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("listing job templates", metalake, err)...)
		return
	}

	items := make([]attr.Value, 0, len(result.JobTemplates))
	for i := range result.JobTemplates {
		item, itemDiags := jobTemplateItem(ctx, &result.JobTemplates[i])
		resp.Diagnostics.Append(itemDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, item)
	}

	list, listDiags := types.ListValue(types.ObjectType{AttrTypes: JobTemplateItemAttrTypes}, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.JobTemplates = list

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func jobTemplateItem(ctx context.Context, t *models.JobTemplate) (basetypes.ObjectValue, diag.Diagnostics) {
	var diags diag.Diagnostics

	flat := JobTemplateDataSourceModel{}
	setDiags := setJobTemplateState(ctx, t, &flat)
	diags.Append(setDiags...)
	if diags.HasError() {
		return basetypes.ObjectValue{}, diags
	}

	attrs := map[string]attr.Value{
		"name":          flat.Name,
		"job_type":      flat.JobType,
		"comment":       flat.Comment,
		"executable":    flat.Executable,
		"arguments":     flat.Arguments,
		"environments":  flat.Environments,
		"custom_fields": flat.CustomFields,
		"scripts":       flat.Scripts,
		"class_name":    flat.ClassName,
		"jars":          flat.Jars,
		"files":         flat.Files,
		"archives":      flat.Archives,
		"configs":       flat.Configs,
		"audit":         flat.Audit,
	}

	obj, d := types.ObjectValue(JobTemplateItemAttrTypes, attrs)
	diags.Append(d...)
	return obj, diags
}
