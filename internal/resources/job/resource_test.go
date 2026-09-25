package job_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/job"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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

// noSuchMetalakeExceptionExample is a real Gravitino 404 payload: an HTTP 404
// carrying application code 1003. It guards against detecting "not found" from
// the payload's `code` field instead of the HTTP status.
const noSuchMetalakeExceptionExample = `{
  "code": 1003,
  "type": "NoSuchMetalakeException",
  "message": "Failed to operate job(s) [job-12345] operation [GET] under metalake [test_metalake], reason [NoSuchMetalakeException]",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchMetalakeException: Metalake test_metalake does not exist",
    "..."
  ]
}`

const jobsRunsPath = "/api/metalakes/test_metalake/jobs/runs"

func newJobResource(t *testing.T, serverURL string) *res.JobResource {
	t.Helper()
	c, err := client.New(serverURL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	r := res.New().(*res.JobResource)
	r.SetClient(c)
	return r
}

func jobSchema(t *testing.T, r resource.Resource) schema.Schema {
	t.Helper()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func jobRaw(t *testing.T, s schema.Schema, model res.JobResourceModel) tftypes.Value {
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

// jobPlanModel is the plan Terraform hands to Create: the configured attributes
// are known, every computed attribute is unknown.
func jobPlanModel(jobConf types.Map) res.JobResourceModel {
	return res.JobResourceModel{
		ID:          types.StringUnknown(),
		Metalake:    types.StringValue("test_metalake"),
		JobTemplate: types.StringValue("test_run_get"),
		JobConf:     jobConf,
		JobID:       types.StringUnknown(),
		Status:      types.StringUnknown(),
		QueuedAt:    types.StringUnknown(),
		StartedAt:   types.StringUnknown(),
		FinishedAt:  types.StringUnknown(),
		Audit:       types.ObjectUnknown(res.AuditAttrTypes),
	}
}

func jobStateModel(status string) res.JobResourceModel {
	return res.JobResourceModel{
		ID:          types.StringValue("test_metalake.job-12345"),
		Metalake:    types.StringValue("test_metalake"),
		JobTemplate: types.StringValue("test_run_get"),
		JobConf:     types.MapNull(types.StringType),
		JobID:       types.StringValue("job-12345"),
		Status:      types.StringValue(status),
		QueuedAt:    types.StringNull(),
		StartedAt:   types.StringNull(),
		FinishedAt:  types.StringNull(),
		Audit:       types.ObjectNull(res.AuditAttrTypes),
	}
}

func jobNullModel() res.JobResourceModel {
	return res.JobResourceModel{
		ID:          types.StringNull(),
		Metalake:    types.StringNull(),
		JobTemplate: types.StringNull(),
		JobConf:     types.MapNull(types.StringType),
		JobID:       types.StringNull(),
		Status:      types.StringNull(),
		QueuedAt:    types.StringNull(),
		StartedAt:   types.StringNull(),
		FinishedAt:  types.StringNull(),
		Audit:       types.ObjectNull(res.AuditAttrTypes),
	}
}

// jobResponseWithStatus builds a spec-shaped JobResponse whose status can be
// varied, so terminal and in-flight jobs can be told apart.
func jobResponseWithStatus(status string) string {
	return `{"code":0,"job":{"jobId":"job-12345","jobTemplateName":"test_run_get","status":"` + status +
		`","audit":{"createTime":"2025-08-12T02:14:28.205023Z","creator":"anonymous"}}}`
}

func assertJSONEqual(t *testing.T, want string, got []byte) {
	t.Helper()
	var wantVal, gotVal interface{}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("invalid expected JSON: %v", err)
	}
	if err := json.Unmarshal(got, &gotVal); err != nil {
		t.Fatalf("request body is not valid JSON: %q (%v)", string(got), err)
	}
	if !reflect.DeepEqual(wantVal, gotVal) {
		t.Errorf("unexpected request body\n got: %s\nwant: %s", string(got), want)
	}
}

func TestJobResource_Schema(t *testing.T) {
	r := res.New()

	metaResp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, metaResp)
	if metaResp.TypeName != "gravitino_job" {
		t.Fatalf("expected gravitino_job, got %s", metaResp.TypeName)
	}

	s := jobSchema(t, r)
	for _, name := range []string{
		"id", "metalake", "job_template", "job_conf", "job_id",
		"status", "queued_at", "started_at", "finished_at", "audit",
	} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %s", name)
		}
	}

	// The pre-v1.3.0 job attributes must be gone: a job is a run of a template,
	// not a user-named record with parameters/schedule.
	for _, gone := range []string{"name", "template", "schedule", "parameters", "properties"} {
		if _, ok := s.Attributes[gone]; ok {
			t.Errorf("attribute %s must no longer exist", gone)
		}
	}
}

func TestJobResource_Create(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(jobResponseExample))
	}))
	defer server.Close()

	r := newJobResource(t, server.URL)
	ctx := context.Background()
	s := jobSchema(t, r)

	plan := jobPlanModel(types.MapValueMust(types.StringType, map[string]attr.Value{
		"arg1": types.StringValue("value1"),
		"arg2": types.StringValue("value2"),
	}))

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: jobRaw(t, s, plan)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if gotMethod != http.MethodPost || gotPath != jobsRunsPath {
		t.Fatalf("expected POST %s, got %s %s", jobsRunsPath, gotMethod, gotPath)
	}
	assertJSONEqual(t, `{"jobTemplateName":"test_run_get","jobConf":{"arg1":"value1","arg2":"value2"}}`, gotBody)

	var state res.JobResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed to read created state: %v", resp.Diagnostics)
	}

	if got := state.ID.ValueString(); got != "test_metalake.job-12345" {
		t.Errorf("expected id test_metalake.job-12345, got %s", got)
	}
	if got := state.JobID.ValueString(); got != "job-12345" {
		t.Errorf("expected job_id job-12345, got %s", got)
	}
	if got := state.Status.ValueString(); got != "succeeded" {
		t.Errorf("expected status succeeded, got %s", got)
	}
	if got := state.JobTemplate.ValueString(); got != "test_run_get" {
		t.Errorf("expected job_template test_run_get, got %s", got)
	}
	if got := state.Metalake.ValueString(); got != "test_metalake" {
		t.Errorf("expected metalake test_metalake, got %s", got)
	}
	for _, name := range []string{"queued_at", "started_at", "finished_at"} {
		var v types.String
		resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root(name), &v)...)
		if !v.IsNull() {
			t.Errorf("expected %s to be null when the API omits it, got %s", name, v.ValueString())
		}
	}

	var jobConf map[string]string
	resp.Diagnostics.Append(state.JobConf.ElementsAs(ctx, &jobConf, false)...)
	if want := map[string]string{"arg1": "value1", "arg2": "value2"}; !reflect.DeepEqual(jobConf, want) {
		t.Errorf("expected job_conf %v, got %v", want, jobConf)
	}

	var audit models.AuditTFSDK
	resp.Diagnostics.Append(state.Audit.As(ctx, &audit, basetypes.ObjectAsOptions{})...)
	if got := audit.Creator.ValueString(); got != "anonymous" {
		t.Errorf("expected audit.creator anonymous, got %s", got)
	}
	if got := audit.CreateTime.ValueString(); got != "2025-08-12T02:14:28Z" {
		t.Errorf("expected audit.create_time 2025-08-12T02:14:28Z, got %s", got)
	}
	if !audit.LastModifiedTime.IsNull() {
		t.Errorf("expected audit.last_modified_time to be null, got %s", audit.LastModifiedTime.ValueString())
	}
}

// TestJobResource_Create_WithoutJobConf guards the omitempty contract of
// JobRunRequest: no job_conf must produce no jobConf key at all.
func TestJobResource_Create_WithoutJobConf(t *testing.T) {
	var gotBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(jobResponseExample))
	}))
	defer server.Close()

	r := newJobResource(t, server.URL)
	ctx := context.Background()
	s := jobSchema(t, r)

	plan := jobPlanModel(types.MapNull(types.StringType))
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: jobRaw(t, s, plan)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	assertJSONEqual(t, `{"jobTemplateName":"test_run_get"}`, gotBody)
}

func TestJobResource_Read_NotFoundRemovesResource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(noSuchMetalakeExceptionExample))
	}))
	defer server.Close()

	r := newJobResource(t, server.URL)
	ctx := context.Background()
	s := jobSchema(t, r)

	state := jobStateModel("started")
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: jobRaw(t, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: jobRaw(t, s, state)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 must remove the resource from state, not fail: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("expected the job to be removed from state, got %s", resp.State.Raw)
	}
}

func TestJobResource_Read(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(jobResponseWithStatus("succeeded")))
	}))
	defer server.Close()

	r := newJobResource(t, server.URL)
	ctx := context.Background()
	s := jobSchema(t, r)

	state := jobStateModel("started")
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: jobRaw(t, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: jobRaw(t, s, state)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotPath != jobsRunsPath+"/job-12345" {
		t.Fatalf("expected GET %s/job-12345, got %s", jobsRunsPath, gotPath)
	}

	var refreshed res.JobResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &refreshed)...)
	if got := refreshed.Status.ValueString(); got != "succeeded" {
		t.Errorf("expected the refreshed status succeeded, got %s", got)
	}
	if got := refreshed.ID.ValueString(); got != "test_metalake.job-12345" {
		t.Errorf("expected id test_metalake.job-12345, got %s", got)
	}
}

// TestJobResource_Delete_CancelsRunningJob proves Delete interrupts a job that
// can still make progress, since the API offers no way to delete a job record.
func TestJobResource_Delete_CancelsRunningJob(t *testing.T) {
	var (
		cancelCalled bool
		cancelPath   string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == jobsRunsPath+"/job-12345":
			_, _ = w.Write([]byte(jobResponseWithStatus("started")))
		case r.Method == http.MethodPost && r.URL.Path == jobsRunsPath+"/job-12345":
			cancelCalled = true
			cancelPath = r.URL.Path
			_, _ = w.Write([]byte(jobResponseWithStatus("canceled")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	r := newJobResource(t, server.URL)
	ctx := context.Background()
	s := jobSchema(t, r)

	state := jobStateModel("started")
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: jobRaw(t, s, state)}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: jobRaw(t, s, state)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !cancelCalled {
		t.Fatal("expected a running job to be cancelled with POST /jobs/runs/{jobId}")
	}
	if cancelPath != jobsRunsPath+"/job-12345" {
		t.Fatalf("expected cancel POST to %s/job-12345, got %s", jobsRunsPath, cancelPath)
	}
}

// TestJobResource_Delete_TerminalStatusDoesNotCancel proves Delete leaves a
// finished job alone: the server rejects cancelling a terminal job.
func TestJobResource_Delete_TerminalStatusDoesNotCancel(t *testing.T) {
	var cancelCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.Method == http.MethodPost {
			cancelCalled = true
			_, _ = w.Write([]byte(jobResponseWithStatus("succeeded")))
			return
		}
		_, _ = w.Write([]byte(jobResponseWithStatus("succeeded")))
	}))
	defer server.Close()

	for _, status := range []string{"succeeded", "failed", "canceled"} {
		t.Run(status, func(t *testing.T) {
			cancelCalled = false
			r := newJobResource(t, server.URL)
			ctx := context.Background()
			s := jobSchema(t, r)

			state := jobStateModel(status)
			resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: jobRaw(t, s, state)}}
			r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: jobRaw(t, s, state)}}, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
			if cancelCalled {
				t.Fatalf("a %s job is terminal and must not be cancelled", status)
			}
		})
	}
}

func TestJobResource_ImportState(t *testing.T) {
	r := res.New().(resource.ResourceWithImportState)

	ctx := context.Background()
	s := jobSchema(t, r.(resource.Resource))

	resp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: s, Raw: jobRaw(t, s, jobNullModel())},
	}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "my_metalake.job-12345"}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	for attr, want := range map[string]string{
		"metalake": "my_metalake",
		"job_id":   "job-12345",
		"id":       "my_metalake.job-12345",
	} {
		var got types.String
		resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root(attr), &got)...)
		if got.ValueString() != want {
			t.Errorf("expected %s %q, got %q", attr, want, got.ValueString())
		}
	}
}

func TestJobResource_ImportState_Invalid(t *testing.T) {
	r := res.New().(resource.ResourceWithImportState)

	ctx := context.Background()
	s := jobSchema(t, r.(resource.Resource))

	for _, id := range []string{"no_dot_here", ".x", "x."} {
		t.Run(id, func(t *testing.T) {
			resp := &resource.ImportStateResponse{
				State: tfsdk.State{Schema: s, Raw: jobRaw(t, s, jobNullModel())},
			}
			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)

			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error for import ID %q", id)
			}
		})
	}
}
