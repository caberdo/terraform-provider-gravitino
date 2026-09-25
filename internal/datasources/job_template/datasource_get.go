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

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

var _ datasource.DataSource = &JobTemplateDataSource{}
var _ datasource.DataSourceWithConfigure = &JobTemplateDataSource{}

type JobTemplateDataSource struct {
	client *client.Client
}

func NewJobTemplateDataSource() datasource.DataSource {
	return &JobTemplateDataSource{}
}

func (d *JobTemplateDataSource) SetClient(c *client.Client) {
	d.client = c
}

type JobTemplateDataSourceModel struct {
	Metalake     types.String `tfsdk:"metalake"`
	Name         types.String `tfsdk:"name"`
	JobType      types.String `tfsdk:"job_type"`
	Comment      types.String `tfsdk:"comment"`
	Executable   types.String `tfsdk:"executable"`
	Arguments    types.List   `tfsdk:"arguments"`
	Environments types.Map    `tfsdk:"environments"`
	CustomFields types.Map    `tfsdk:"custom_fields"`
	Scripts      types.List   `tfsdk:"scripts"`
	ClassName    types.String `tfsdk:"class_name"`
	Jars         types.List   `tfsdk:"jars"`
	Files        types.List   `tfsdk:"files"`
	Archives     types.List   `tfsdk:"archives"`
	Configs      types.Map    `tfsdk:"configs"`
	Audit        types.Object `tfsdk:"audit"`
}

func (d *JobTemplateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *JobTemplateDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_job_template"
}

func jobTemplateAttributes() map[string]schema.Attribute {
	attributes := map[string]schema.Attribute{
		"job_type": schema.StringAttribute{
			Computed:    true,
			Description: "The job template type (shell or spark).",
		},
		"comment": schema.StringAttribute{
			Computed:    true,
			Description: "A comment about the job template.",
		},
		"executable": schema.StringAttribute{
			Computed:    true,
			Description: "The executable command of the job template.",
		},
		"arguments": schema.ListAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "The arguments of the job template.",
		},
		"environments": schema.MapAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "Environment variables for the job template.",
		},
		"custom_fields": schema.MapAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "Custom fields for the job template.",
		},
		"scripts": schema.ListAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "The scripts of a shell job template.",
		},
		"class_name": schema.StringAttribute{
			Computed:    true,
			Description: "The main class of a spark job template.",
		},
		"jars": schema.ListAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "The jars of a spark job template.",
		},
		"files": schema.ListAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "The files of a spark job template.",
		},
		"archives": schema.ListAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "The archives of a spark job template.",
		},
		"configs": schema.MapAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "The spark configurations.",
		},
		"audit": schema.ObjectAttribute{
			Computed:       true,
			AttributeTypes: AuditAttrTypes,
			Description:    "Audit information for the job template.",
		},
	}
	return attributes
}

func (d *JobTemplateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"metalake": schema.StringAttribute{
			Required:    true,
			Description: "The metalake name.",
		},
		"name": schema.StringAttribute{
			Required:    true,
			Description: "The job template name.",
		},
	}
	for k, v := range jobTemplateAttributes() {
		attributes[k] = v
	}

	resp.Schema = schema.Schema{
		Description: "Looks up a single Gravitino job template.",
		Attributes:  attributes,
	}
}

func (d *JobTemplateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config JobTemplateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := config.Metalake.ValueString()
	name := config.Name.ValueString()

	result, err := d.client.GetJobTemplate(ctx, metalake, name)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Job template %q not found", name),
				fmt.Sprintf("No job template named %q exists in metalake %q.", name, metalake),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading job template", name, err)...)
		return
	}

	resp.Diagnostics.Append(setJobTemplateState(ctx, &result.JobTemplate, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

// setJobTemplateState maps an API job template onto the flat attribute set shared
// by the get and list data sources.
func setJobTemplateState(ctx context.Context, t *models.JobTemplate, model *JobTemplateDataSourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	arguments, d := jobTemplateList(ctx, t.Arguments)
	diags.Append(d...)
	scripts, d := jobTemplateList(ctx, t.Scripts)
	diags.Append(d...)
	jars, d := jobTemplateList(ctx, t.Jars)
	diags.Append(d...)
	files, d := jobTemplateList(ctx, t.Files)
	diags.Append(d...)
	archives, d := jobTemplateList(ctx, t.Archives)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	model.Name = types.StringValue(t.Name)
	model.JobType = types.StringValue(t.JobType)
	// The API omits optional fields entirely (a shell template has no class_name, a
	// template without a comment returns no comment field): expose those as null rather
	// than as an empty string.
	model.Comment = optionalStringValue(t.Comment)
	model.Executable = types.StringValue(t.Executable)
	model.Arguments = arguments
	model.Environments = jobTemplateMap(t.Environments)
	model.CustomFields = jobTemplateMap(t.CustomFields)
	model.Scripts = scripts
	model.ClassName = optionalStringValue(t.ClassName)
	model.Jars = jars
	model.Files = files
	model.Archives = archives
	model.Configs = jobTemplateMap(t.Configs)

	auditObj, d := auditToObjectValueForDS(t.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	model.Audit = auditObj

	return diags
}

// optionalStringValue renders an absent server value as null.
func optionalStringValue(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func jobTemplateList(ctx context.Context, s []string) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	if len(s) == 0 {
		return types.ListNull(types.StringType), diags
	}
	attrs := make([]attr.Value, 0, len(s))
	for _, v := range s {
		attrs = append(attrs, types.StringValue(v))
	}
	list, d := types.ListValue(types.StringType, attrs)
	diags.Append(d...)
	return list, diags
}

func jobTemplateMap(m map[string]string) types.Map {
	if len(m) == 0 {
		return types.MapNull(types.StringType)
	}
	attrs := make(map[string]attr.Value, len(m))
	for k, v := range m {
		attrs[k] = types.StringValue(v)
	}
	return types.MapValueMust(types.StringType, attrs)
}

func auditToObjectValueForDS(audit *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	var diags diag.Diagnostics

	if audit == nil {
		return types.ObjectNull(AuditAttrTypes), diags
	}

	var createTime, lastModifiedTime types.String
	if audit.CreateTime != nil {
		createTime = types.StringValue(audit.CreateTime.Format(timeFormat))
	} else {
		createTime = types.StringNull()
	}
	if audit.LastModifiedTime != nil {
		lastModifiedTime = types.StringValue(audit.LastModifiedTime.Format(timeFormat))
	} else {
		lastModifiedTime = types.StringNull()
	}

	attrs := map[string]attr.Value{
		"creator":            types.StringValue(audit.Creator),
		"create_time":        createTime,
		"last_modifier":      types.StringValue(audit.LastModifier),
		"last_modified_time": lastModifiedTime,
	}

	return types.ObjectValue(AuditAttrTypes, attrs)
}
