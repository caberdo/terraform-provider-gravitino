package job

import (
	"context"
	"fmt"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &JobDataSource{}
var _ datasource.DataSourceWithConfigure = &JobDataSource{}

type JobDataSource struct {
	client *client.Client
}

func NewGetDataSource() datasource.DataSource {
	return &JobDataSource{}
}

func (d *JobDataSource) SetClient(c *client.Client) {
	d.client = c
}

type JobDataSourceModel struct {
	Metalake    types.String `tfsdk:"metalake"`
	JobID       types.String `tfsdk:"job_id"`
	JobTemplate types.String `tfsdk:"job_template"`
	Status      types.String `tfsdk:"status"`
	QueuedAt    types.String `tfsdk:"queued_at"`
	StartedAt   types.String `tfsdk:"started_at"`
	FinishedAt  types.String `tfsdk:"finished_at"`
	Audit       types.Object `tfsdk:"audit"`
}

func (d *JobDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *JobDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "gravitino_job"
}

func (d *JobDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a single job run by its server-generated job id.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"job_id": schema.StringAttribute{
				Required:    true,
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
	}
}

func (d *JobDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config JobDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	jobID := config.JobID.ValueString()

	result, err := d.client.GetJob(ctx, config.Metalake.ValueString(), jobID)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Job %q not found", jobID),
				fmt.Sprintf("No job run with id %q exists in metalake %q.", jobID, config.Metalake.ValueString()),
			)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading job", jobID, err)...)
		return
	}

	setDataSourceStateFromJob(ctx, &resp.Diagnostics, &result.Job, &config)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

func setDataSourceStateFromJob(ctx context.Context, diags *diag.Diagnostics, job *models.Job, model *JobDataSourceModel) {
	model.JobID = types.StringValue(job.JobID)
	model.JobTemplate = types.StringValue(job.JobTemplateName)
	model.Status = types.StringValue(job.Status)
	model.QueuedAt = dataSourceTimeToString(job.QueuedAt)
	model.StartedAt = dataSourceTimeToString(job.StartedAt)
	model.FinishedAt = dataSourceTimeToString(job.FinishedAt)

	auditObj, d := auditToObjectValueForDS(ctx, job.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Audit = auditObj
}

func dataSourceTimeToString(t *time.Time) types.String {
	if t == nil {
		return types.StringNull()
	}
	return types.StringValue(t.Format(time.RFC3339))
}
