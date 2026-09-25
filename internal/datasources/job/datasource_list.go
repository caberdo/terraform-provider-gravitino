package job

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

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

var _ datasource.DataSource = &JobsDataSource{}
var _ datasource.DataSourceWithConfigure = &JobsDataSource{}

type JobsDataSource struct {
	client *client.Client
}

func NewListDataSource() datasource.DataSource {
	return &JobsDataSource{}
}

func (d *JobsDataSource) SetClient(c *client.Client) {
	d.client = c
}

type JobsDataSourceModel struct {
	Metalake    types.String `tfsdk:"metalake"`
	Jobs        types.List   `tfsdk:"jobs"`
	JobTemplate types.String `tfsdk:"job_template"`
}

var JobItemAttrTypes = map[string]attr.Type{
	"job_id":       types.StringType,
	"job_template": types.StringType,
	"status":       types.StringType,
	"queued_at":    types.StringType,
	"started_at":   types.StringType,
	"finished_at":  types.StringType,
	"audit":        types.ObjectType{AttrTypes: AuditAttrTypes},
}

func (d *JobsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *JobsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_jobs"
}

func (d *JobsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the job runs of a metalake, optionally filtered by job template name.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"job_template": schema.StringAttribute{
				Optional:    true,
				Description: "Only return job runs of this job template.",
			},
			"jobs": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"job_id": schema.StringAttribute{
							Computed:    true,
							Description: "The unique identifier of the job run.",
						},
						"job_template": schema.StringAttribute{
							Computed:    true,
							Description: "The name of the job template the job runs.",
						},
						"status": schema.StringAttribute{
							Computed:    true,
							Description: "The current status of the job run.",
						},
						"queued_at": schema.StringAttribute{
							Computed:    true,
							Description: "The time the job was queued (RFC3339).",
						},
						"started_at": schema.StringAttribute{
							Computed:    true,
							Description: "The time the job started (RFC3339).",
						},
						"finished_at": schema.StringAttribute{
							Computed:    true,
							Description: "The time the job finished (RFC3339).",
						},
						"audit": schema.ObjectAttribute{
							Computed:       true,
							AttributeTypes: AuditAttrTypes,
							Description:    "Audit information for the job run.",
						},
					},
				},
			},
		},
	}
}

func (d *JobsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config JobsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := config.Metalake.ValueString()

	result, err := d.client.ListJobs(ctx, metalake)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("listing jobs", metalake, err)...)
		return
	}

	filter := ""
	if !config.JobTemplate.IsNull() && !config.JobTemplate.IsUnknown() {
		filter = config.JobTemplate.ValueString()
	}

	items := make([]attr.Value, 0, len(result.Jobs))
	for i := range result.Jobs {
		job := &result.Jobs[i]
		if filter != "" && job.JobTemplateName != filter {
			continue
		}

		item, itemDiags := jobToItemModel(ctx, job)
		resp.Diagnostics.Append(itemDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		items = append(items, item)
	}

	jobsList, listDiags := types.ListValue(types.ObjectType{AttrTypes: JobItemAttrTypes}, items)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.Jobs = jobsList

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func jobToItemModel(ctx context.Context, job *models.Job) (basetypes.ObjectValue, diag.Diagnostics) {
	var diags diag.Diagnostics

	auditObj, d := auditToObjectValueForDS(ctx, job.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return basetypes.ObjectValue{}, diags
	}

	attrs := map[string]attr.Value{
		"job_id":       types.StringValue(job.JobID),
		"job_template": types.StringValue(job.JobTemplateName),
		"status":       types.StringValue(job.Status),
		"queued_at":    dataSourceTimeToString(job.QueuedAt),
		"started_at":   dataSourceTimeToString(job.StartedAt),
		"finished_at":  dataSourceTimeToString(job.FinishedAt),
		"audit":        auditObj,
	}

	obj, d := types.ObjectValue(JobItemAttrTypes, attrs)
	diags.Append(d...)
	return obj, diags
}

func auditToObjectValueForDS(_ context.Context, audit *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	var diags diag.Diagnostics

	if audit == nil {
		return types.ObjectNull(AuditAttrTypes), diags
	}

	var createTime, lastModifiedTime types.String
	if audit.CreateTime != nil {
		createTime = types.StringValue(audit.CreateTime.Format(time.RFC3339))
	} else {
		createTime = types.StringNull()
	}
	if audit.LastModifiedTime != nil {
		lastModifiedTime = types.StringValue(audit.LastModifiedTime.Format(time.RFC3339))
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
