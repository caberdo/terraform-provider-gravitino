package job

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &JobResource{}
var _ resource.ResourceWithImportState = &JobResource{}
var _ resource.ResourceWithConfigure = &JobResource{}

// JobResource manages a single job run. Gravitino models jobs as job *runs* of a
// job template: POST /jobs/runs starts one and the server assigns the jobId.
// Therefore every configured attribute forces a replacement (a new run) and
// Delete cancels the run rather than deleting it, because the API has no delete.
type JobResource struct {
	client *client.Client
}

func New() resource.Resource {
	return &JobResource{}
}

func (r *JobResource) SetClient(c *client.Client) {
	r.client = c
}

type JobResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Metalake    types.String `tfsdk:"metalake"`
	JobTemplate types.String `tfsdk:"job_template"`
	JobConf     types.Map    `tfsdk:"job_conf"`
	JobID       types.String `tfsdk:"job_id"`
	Status      types.String `tfsdk:"status"`
	QueuedAt    types.String `tfsdk:"queued_at"`
	StartedAt   types.String `tfsdk:"started_at"`
	FinishedAt  types.String `tfsdk:"finished_at"`
	Audit       types.Object `tfsdk:"audit"`
}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

// jobStatuses are the status values defined by the Gravitino Job schema.
var jobStatuses = []string{"queued", "started", "failed", "succeeded", "cancelling", "canceled"}

const (
	cancelPollInterval = 2 * time.Second
	// cancelPollTimeout bounds how long destroy waits for a cancelled job to reach a
	// terminal status. Gravitino only leaves "cancelling" once a job executor collects
	// the cancellation, and without a running executor it never does, so the wait is
	// deliberately short and best effort.
	cancelPollTimeout = 10 * time.Second
)

func (r *JobResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *JobResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_job"
}

func (r *JobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a job run in a Gravitino metalake. A job is started by running a job " +
			"template and is identified by the server-generated job id, so changing any configured " +
			"attribute starts a new run. Destroying the resource cancels the run; Gravitino keeps no " +
			"deletable job records.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "Composite identifier in the format 'metalake.job_id'.",
			},
			"metalake": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "The metalake name.",
			},
			"job_template": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "The name of the job template to run.",
			},
			"job_conf": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
				Description: "The job configuration passed to the job run.",
			},
			"job_id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "The server-generated unique identifier of the job run.",
			},
			"status": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: fmt.Sprintf("The status of the job run. One of: %s.", strings.Join(jobStatuses, ", ")),
			},
			"queued_at": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "The time the job was queued (RFC3339).",
			},
			"started_at": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "The time the job started (RFC3339).",
			},
			"finished_at": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "The time the job finished (RFC3339).",
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Description: "Audit information for the job run.",
			},
		},
	}
}

func (r *JobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan JobResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Running job", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"template": plan.JobTemplate.ValueString(),
	})

	runReq := &models.JobRunRequest{
		JobTemplateName: plan.JobTemplate.ValueString(),
		JobConf:         mapFromTF(plan.JobConf),
	}

	result, err := r.client.RunJob(ctx, plan.Metalake.ValueString(), runReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("running job", plan.JobTemplate.ValueString(), err)...)
		return
	}

	setStateFromJob(ctx, &resp.Diagnostics, plan.Metalake.ValueString(), &result.Job, &plan)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Ran job", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"job_id":   plan.JobID.ValueString(),
	})
}

func (r *JobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state JobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading job", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"job_id":   state.JobID.ValueString(),
	})

	result, err := r.client.GetJob(ctx, state.Metalake.ValueString(), state.JobID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading job", state.JobID.ValueString(), err)...)
		return
	}

	setStateFromJob(ctx, &resp.Diagnostics, state.Metalake.ValueString(), &result.Job, &state)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)

	tflog.Debug(ctx, "Read job", map[string]interface{}{"job_id": state.JobID.ValueString()})
}

// Update is unreachable: every configurable attribute forces a replacement. It
// exists only to satisfy the resource interface.
func (r *JobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Job update not supported",
		"A Gravitino job run is immutable. Changing job_template, job_conf or metalake starts a new job run.",
	)
}

func (r *JobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state JobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := state.Metalake.ValueString()
	jobID := state.JobID.ValueString()

	tflog.Debug(ctx, "Cancelling job", map[string]interface{}{"metalake": metalake, "job_id": jobID})

	current, err := r.client.GetJob(ctx, metalake, jobID)
	if err != nil {
		if client.IsNotFoundError(err) {
			// Already gone: nothing to cancel.
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting job", jobID, err)...)
		return
	}

	// Cancelling a finished job is rejected by the server, so only interrupt
	// jobs that can still make progress.
	if !jobIsTerminal(current.Job.Status) {
		if _, err := r.client.CancelJob(ctx, metalake, jobID); err != nil && !client.IsNotFoundError(err) {
			resp.Diagnostics.Append(client.NewResourceError("cancelling job", jobID, err)...)
			return
		}
		// Gravitino refuses to delete a job template while it still has active
		// jobs, and a cancelled job only becomes terminal once the job executor
		// has stopped it. Wait (bounded) so that destroying a template after its
		// jobs is deterministic whenever an executor is doing its work; if the
		// job stays active we continue anyway and let the template delete report
		// the server's own InUseException.
		waitForTerminalJob(ctx, r.client, metalake, jobID)
	}

	tflog.Debug(ctx, "Cancelled job", map[string]interface{}{"metalake": metalake, "job_id": jobID})
}

// waitForTerminalJob polls until the job reaches a terminal status, the context is
// cancelled, or cancelPollTimeout elapses.
func waitForTerminalJob(ctx context.Context, c *client.Client, metalake, jobID string) {
	deadline := time.Now().Add(cancelPollTimeout)

	ticker := time.NewTicker(cancelPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Now().After(deadline) {
				return
			}
			current, err := c.GetJob(ctx, metalake, jobID)
			if err != nil {
				return
			}
			if jobIsTerminal(current.Job.Status) {
				return
			}
		}
	}
}

func (r *JobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idx := strings.LastIndex(req.ID, ".")
	if idx == -1 || idx == 0 || idx == len(req.ID)-1 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected 'metalake.job_id', got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), req.ID[:idx])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("job_id"), req.ID[idx+1:])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// jobIsTerminal reports whether a status is final, so cancelling is meaningless.
func jobIsTerminal(status string) bool {
	switch status {
	case "succeeded", "failed", "canceled":
		return true
	default:
		return false
	}
}

// setStateFromJob writes every schema attribute from an API response so no
// attribute is ever left unknown after apply.
func setStateFromJob(ctx context.Context, diags *diag.Diagnostics, metalake string, job *models.Job, model *JobResourceModel) {
	model.Metalake = types.StringValue(metalake)
	model.JobID = types.StringValue(job.JobID)
	model.JobTemplate = types.StringValue(job.JobTemplateName)
	model.Status = types.StringValue(job.Status)
	model.ID = types.StringValue(metalake + "." + job.JobID)
	model.QueuedAt = timeToString(job.QueuedAt)
	model.StartedAt = timeToString(job.StartedAt)
	model.FinishedAt = timeToString(job.FinishedAt)

	auditObj, d := auditToObjectValue(ctx, job.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Audit = auditObj
}

func timeToString(t *time.Time) types.String {
	if t == nil {
		return types.StringNull()
	}
	return types.StringValue(t.Format(time.RFC3339))
}

func auditToObjectValue(_ context.Context, audit *models.Audit) (types.Object, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(AuditAttrTypes), nil
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

func mapFromTF(m types.Map) map[string]string {
	result := make(map[string]string)
	if m.IsNull() || m.IsUnknown() {
		return result
	}
	for k, v := range m.Elements() {
		if strVal, ok := v.(types.String); ok {
			result[k] = strVal.ValueString()
		}
	}
	return result
}
