package job_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/job"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// jobResponseExample is the exact `JobResponse` example of the Gravitino v1.3.0
// jobs spec (docs/open-api/jobs.yaml, components/examples/JobResponse).
const jobResponseExample = `{
  "code": 0,
  "job": {
    "jobId": "job-12345",
    "jobTemplateName": "test_run_get",
    "status": "succeeded",
    "audit": {
      "createTime": "2025-08-12T02:14:28.205023Z",
      "creator": "anonymous"
    }
  }
}`

// jobListResponseExample is the exact `JobListResponse` example of the spec.
const jobListResponseExample = `{
  "code": 0,
  "jobs": [
    {
      "jobId": "job-12345",
      "jobTemplateName": "test_run_get",
      "status": "succeeded",
      "audit": {
        "createTime": "2025-08-12T02:14:28.205023Z",
        "creator": "anonymous"
      }
    },
    {
      "jobId": "job-67890",
      "jobTemplateName": "test_run_get_spark",
      "status": "failed",
      "audit": {
        "createTime": "2025-08-12T02:14:28.205023Z",
        "creator": "anonymous"
      }
    }
  ]
}`

// noSuchJobExceptionExample is the spec's `NoSuchJobException` 404 payload.
const noSuchJobExceptionExample = `{
  "code": 1003,
  "type": "NoSuchJobException",
  "message": "Failed to operate job(s) [job-12345] operation [GET] under metalake [test_metalake], reason [NoSuchJobException]",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchJobException: Job xxx does not exist",
    "..."
  ]
}`

const jobsRunsPath = "/api/metalakes/test_metalake/jobs/runs"

// jobItem mirrors ds.JobItemAttrTypes so a nested list value can be decoded into
// a struct in assertions.
type jobItem struct {
	JobID       types.String `tfsdk:"job_id"`
	JobTemplate types.String `tfsdk:"job_template"`
	Status      types.String `tfsdk:"status"`
	QueuedAt    types.String `tfsdk:"queued_at"`
	StartedAt   types.String `tfsdk:"started_at"`
	FinishedAt  types.String `tfsdk:"finished_at"`
	Audit       types.Object `tfsdk:"audit"`
}

func jobSchema(t *testing.T, d datasource.DataSource) schema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func jobRaw(t *testing.T, s schema.Schema, model interface{}) tftypes.Value {
	t.Helper()
	obj, diags := types.ObjectValueFrom(context.Background(), s.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("failed to build object from model: %v", diags)
	}
	raw, err := obj.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("failed to convert object to terraform value: %v", err)
	}
	return raw
}

func newGetClient(t *testing.T, serverURL string) *client.Client {
	t.Helper()
	c, err := client.New(serverURL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	return c
}

func TestJobDataSource_Metadata(t *testing.T) {
	d := ds.NewGetDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_job" {
		t.Fatalf("expected gravitino_job, got %s", resp.TypeName)
	}
}

func TestJobsDataSource_Metadata(t *testing.T) {
	d := ds.NewListDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_jobs" {
		t.Fatalf("expected gravitino_jobs, got %s", resp.TypeName)
	}
}

func TestJobDataSource_Read(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(jobResponseExample))
	}))
	defer server.Close()

	d := ds.NewGetDataSource()
	c := newGetClient(t, server.URL)
	d.(*ds.JobDataSource).SetClient(c)

	ctx := context.Background()
	s := jobSchema(t, d)

	config := ds.JobDataSourceModel{
		Metalake:    types.StringValue("test_metalake"),
		JobID:       types.StringValue("job-12345"),
		JobTemplate: types.StringNull(),
		Status:      types.StringNull(),
		QueuedAt:    types.StringNull(),
		StartedAt:   types.StringNull(),
		FinishedAt:  types.StringNull(),
		Audit:       types.ObjectNull(ds.AuditAttrTypes),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: jobRaw(t, s, config)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotPath != jobsRunsPath+"/job-12345" {
		t.Fatalf("expected GET %s/job-12345, got %s", jobsRunsPath, gotPath)
	}

	var state ds.JobDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
	if got := state.JobID.ValueString(); got != "job-12345" {
		t.Errorf("expected job_id job-12345, got %s", got)
	}
	if got := state.JobTemplate.ValueString(); got != "test_run_get" {
		t.Errorf("expected job_template test_run_get, got %s", got)
	}
	if got := state.Status.ValueString(); got != "succeeded" {
		t.Errorf("expected status succeeded, got %s", got)
	}
	if !state.QueuedAt.IsNull() {
		t.Errorf("expected queued_at to be null when the API omits it, got %s", state.QueuedAt.ValueString())
	}
	if !state.StartedAt.IsNull() {
		t.Errorf("expected started_at to be null when the API omits it, got %s", state.StartedAt.ValueString())
	}
	if !state.FinishedAt.IsNull() {
		t.Errorf("expected finished_at to be null when the API omits it, got %s", state.FinishedAt.ValueString())
	}

	var audit models.AuditTFSDK
	resp.Diagnostics.Append(state.Audit.As(ctx, &audit, basetypes.ObjectAsOptions{})...)
	if got := audit.Creator.ValueString(); got != "anonymous" {
		t.Errorf("expected audit.creator anonymous, got %s", got)
	}
	if got := audit.CreateTime.ValueString(); got != "2025-08-12T02:14:28Z" {
		t.Errorf("expected audit.create_time 2025-08-12T02:14:28Z, got %s", got)
	}
}

func TestJobDataSource_Read_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(noSuchJobExceptionExample))
	}))
	defer server.Close()

	d := ds.NewGetDataSource()
	c := newGetClient(t, server.URL)
	d.(*ds.JobDataSource).SetClient(c)

	ctx := context.Background()
	s := jobSchema(t, d)

	config := ds.JobDataSourceModel{
		Metalake:    types.StringValue("test_metalake"),
		JobID:       types.StringValue("job-12345"),
		JobTemplate: types.StringNull(),
		Status:      types.StringNull(),
		QueuedAt:    types.StringNull(),
		StartedAt:   types.StringNull(),
		FinishedAt:  types.StringNull(),
		Audit:       types.ObjectNull(ds.AuditAttrTypes),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: jobRaw(t, s, config)}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for a missing job")
	}
	if len(resp.Diagnostics.Errors()) != 1 {
		t.Fatalf("expected exactly one error diagnostic, got %v", resp.Diagnostics.Errors())
	}
	errDiag := resp.Diagnostics.Errors()[0]
	if !strings.Contains(errDiag.Summary(), "Job \"job-12345\" not found") {
		t.Errorf("expected a not-found summary, got %q", errDiag.Summary())
	}
	if !strings.Contains(errDiag.Detail(), "test_metalake") {
		t.Errorf("expected the metalake to be named in the detail, got %q", errDiag.Detail())
	}
}

func TestJobsDataSource_Read(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(jobListResponseExample))
	}))
	defer server.Close()

	d := ds.NewListDataSource()
	c := newGetClient(t, server.URL)
	d.(*ds.JobsDataSource).SetClient(c)

	ctx := context.Background()
	s := jobSchema(t, d)

	config := ds.JobsDataSourceModel{
		Metalake:    types.StringValue("test_metalake"),
		Jobs:        types.ListNull(types.ObjectType{AttrTypes: ds.JobItemAttrTypes}),
		JobTemplate: types.StringNull(),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: jobRaw(t, s, config)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotPath != jobsRunsPath {
		t.Fatalf("expected GET %s, got %s", jobsRunsPath, gotPath)
	}

	items := jobItemsFromState(t, ctx, resp)
	if len(items) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(items))
	}
	if items[0].JobID.ValueString() != "job-12345" || items[0].JobTemplate.ValueString() != "test_run_get" {
		t.Errorf("unexpected first job: %+v", items[0])
	}
	if items[0].Status.ValueString() != "succeeded" {
		t.Errorf("expected the first job to have succeeded, got %s", items[0].Status.ValueString())
	}
	if items[1].JobID.ValueString() != "job-67890" || items[1].JobTemplate.ValueString() != "test_run_get_spark" {
		t.Errorf("unexpected second job: %+v", items[1])
	}
	if items[1].Status.ValueString() != "failed" {
		t.Errorf("expected the second job to have failed, got %s", items[1].Status.ValueString())
	}
	if !items[0].QueuedAt.IsNull() || !items[0].StartedAt.IsNull() || !items[0].FinishedAt.IsNull() {
		t.Error("expected the timestamps the API omits to stay null")
	}

	var audit models.AuditTFSDK
	resp.Diagnostics.Append(items[0].Audit.As(ctx, &audit, basetypes.ObjectAsOptions{})...)
	if got := audit.Creator.ValueString(); got != "anonymous" {
		t.Errorf("expected audit.creator anonymous, got %s", got)
	}
}

func TestJobsDataSource_Read_FilterByTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(jobListResponseExample))
	}))
	defer server.Close()

	d := ds.NewListDataSource()
	c := newGetClient(t, server.URL)
	d.(*ds.JobsDataSource).SetClient(c)

	ctx := context.Background()
	s := jobSchema(t, d)

	config := ds.JobsDataSourceModel{
		Metalake:    types.StringValue("test_metalake"),
		Jobs:        types.ListNull(types.ObjectType{AttrTypes: ds.JobItemAttrTypes}),
		JobTemplate: types.StringValue("test_run_get_spark"),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: jobRaw(t, s, config)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	items := jobItemsFromState(t, ctx, resp)
	if len(items) != 1 {
		t.Fatalf("expected the filter to leave 1 job, got %d", len(items))
	}
	if got := items[0].JobID.ValueString(); got != "job-67890" {
		t.Errorf("expected job-67890, got %s", got)
	}
}

func jobItemsFromState(t *testing.T, ctx context.Context, resp *datasource.ReadResponse) []jobItem {
	t.Helper()

	var state ds.JobsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed to read state: %v", resp.Diagnostics)
	}

	var items []jobItem
	resp.Diagnostics.Append(state.Jobs.ElementsAs(ctx, &items, false)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed to decode jobs list: %v", resp.Diagnostics)
	}
	return items
}
