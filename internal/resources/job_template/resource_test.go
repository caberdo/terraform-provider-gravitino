package job_template_test

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
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/job_template"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Paths of the Gravitino v1.3.0 jobs spec examples
// (docs/open-api/jobs.yaml, components/examples).
const (
	shellExecutable   = "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/test-job.sh"
	shellCommonScript = "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/common.sh"
	updatedExecutable = "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/updated-test-job.sh"
	updatedScript     = "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/updated-common.sh"
	sparkExecutable   = "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/spark-demo.jar"
	sparkJar          = "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/spark-job.jar"
)

// jobTemplateResponseExample is the exact `JobTemplateResponse` example of the
// Gravitino v1.3.0 jobs spec (components/examples/JobTemplateResponse).
const jobTemplateResponseExample = `{
  "code": 0,
  "jobTemplate": {
    "name": "test_run_get",
    "jobType": "shell",
    "comment": "Test shell job template",
    "executable": "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/test-job.sh",
    "arguments": ["{{arg1}}", "{{arg2}}"],
    "environments": {
      "ENV_VAR": "{{env_var}}"
    },
    "customFields": { },
    "scripts": [
      "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/common.sh"
    ],
    "audit": {
      "createTime": "2025-08-12T02:14:28.205023Z",
      "creator": "anonymous"
    }
  }
}`

// noSuchJobTemplateExceptionExample is the spec's `NoSuchJobTemplateException`
// 404 payload.
const noSuchJobTemplateExceptionExample = `{
  "code": 1003,
  "type": "NoSuchJobTemplateException",
  "message": "Failed to operate job template(s) [my_job] operation [GET] under metalake [my_test_metalake], reason [NoSuchJobTemplateException]",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchJobTemplateException: Job template xxx does not exist",
    "..."
  ]
}`

const jobTemplatesPath = "/api/metalakes/test_metalake/jobs/templates"

// wantRegisterBody is the wire body expected by POST /jobs/templates: the
// `JobTemplateRegisterRequest` example of the spec, wrapped in `jobTemplate`.
var wantRegisterBody = `{"jobTemplate":{"name":"test_run_get","jobType":"shell","comment":"Test shell job template","executable":"` +
	shellExecutable + `","arguments":["{{arg1}}","{{arg2}}"],"environments":{"ENV_VAR":"{{env_var}}"},"scripts":["` + shellCommonScript + `"]}}`

// wantUpdatesBody is the wire body expected by PUT /jobs/templates/{name}: the
// `JobTemplateUpdatesRequest` example of the spec. The rename must come first
// and updateTemplate may only carry changed fields.
var wantUpdatesBody = `{"updates":[{"@type":"rename","newName":"new_test_run_get"},{"@type":"updateComment","newComment":"Updated comment for the job template"},{"@type":"updateTemplate","newTemplate":{"@type":"shell","newExecutable":"` +
	updatedExecutable + `","newArguments":["{{new_arg1}}","{{new_arg2}}"],"newEnvironments":{"NEW_ENV_VAR":"{{new_env_var}}"},"newCustomFields":{"field1":"value1"},"newScripts":["` + updatedScript + `"]}}]}`

func newJobTemplateResource(t *testing.T, serverURL string) *res.JobTemplateResource {
	t.Helper()
	c, err := client.New(serverURL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	r := res.NewJobTemplateResource().(*res.JobTemplateResource)
	r.SetClient(c)
	return r
}

func jobTemplateSchema(t *testing.T, r resource.Resource) schema.Schema {
	t.Helper()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func jobTemplateRaw(t *testing.T, s schema.Schema, model res.JobTemplateResourceModel) tftypes.Value {
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

func shellConfigModel() res.JobTemplateResourceModel {
	return res.JobTemplateResourceModel{
		ID:           types.StringNull(),
		Metalake:     types.StringValue("test_metalake"),
		Name:         types.StringValue("test_run_get"),
		JobType:      types.StringValue("shell"),
		Comment:      types.StringValue("Test shell job template"),
		Executable:   types.StringValue(shellExecutable),
		Arguments:    types.ListValueMust(types.StringType, []attr.Value{types.StringValue("{{arg1}}"), types.StringValue("{{arg2}}")}),
		Environments: types.MapValueMust(types.StringType, map[string]attr.Value{"ENV_VAR": types.StringValue("{{env_var}}")}),
		CustomFields: types.MapNull(types.StringType),
		Scripts:      types.ListValueMust(types.StringType, []attr.Value{types.StringValue(shellCommonScript)}),
		ClassName:    types.StringNull(),
		Jars:         types.ListNull(types.StringType),
		Files:        types.ListNull(types.StringType),
		Archives:     types.ListNull(types.StringType),
		Configs:      types.MapNull(types.StringType),
		Audit:        types.ObjectNull(res.AuditAttrTypes),
	}
}

// jobTemplateStateModel mirrors jobTemplateResponseExample: the state after a
// create that read the template back for its audit information.
func jobTemplateStateModel() res.JobTemplateResourceModel {
	m := shellConfigModel()
	m.ID = types.StringValue("test_metalake.test_run_get")
	createTime := "2025-08-12T02:14:28Z"
	m.Audit = types.ObjectValueMust(res.AuditAttrTypes, map[string]attr.Value{
		"creator":            types.StringValue("anonymous"),
		"create_time":        types.StringValue(createTime),
		"last_modifier":      types.StringValue(""),
		"last_modified_time": types.StringNull(),
	})
	return m
}

func jobTemplateNullModel() res.JobTemplateResourceModel {
	return res.JobTemplateResourceModel{
		ID:           types.StringNull(),
		Metalake:     types.StringNull(),
		Name:         types.StringNull(),
		JobType:      types.StringNull(),
		Comment:      types.StringNull(),
		Executable:   types.StringNull(),
		Arguments:    types.ListNull(types.StringType),
		Environments: types.MapNull(types.StringType),
		CustomFields: types.MapNull(types.StringType),
		Scripts:      types.ListNull(types.StringType),
		ClassName:    types.StringNull(),
		Jars:         types.ListNull(types.StringType),
		Files:        types.ListNull(types.StringType),
		Archives:     types.ListNull(types.StringType),
		Configs:      types.MapNull(types.StringType),
		Audit:        types.ObjectNull(res.AuditAttrTypes),
	}
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

func listValues(t *testing.T, ctx context.Context, l types.List) []string {
	t.Helper()
	if l.IsNull() {
		return nil
	}
	var out []string
	if d := l.ElementsAs(ctx, &out, false); d.HasError() {
		t.Fatalf("failed to decode list: %v", d)
	}
	return out
}

func mapValues(t *testing.T, ctx context.Context, m types.Map) map[string]string {
	t.Helper()
	if m.IsNull() {
		return nil
	}
	out := map[string]string{}
	if d := m.ElementsAs(ctx, &out, false); d.HasError() {
		t.Fatalf("failed to decode map: %v", d)
	}
	return out
}

func TestJobTemplateResource_Schema(t *testing.T) {
	r := res.NewJobTemplateResource()

	metaResp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, metaResp)
	if metaResp.TypeName != "gravitino_job_template" {
		t.Fatalf("expected gravitino_job_template, got %s", metaResp.TypeName)
	}

	s := jobTemplateSchema(t, r)
	for _, name := range []string{
		"id", "metalake", "name", "job_type", "comment", "executable", "arguments",
		"environments", "custom_fields", "scripts", "class_name", "jars", "files",
		"archives", "configs", "audit",
	} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %s", name)
		}
	}

	// The pre-v1.3.0 job template attributes must be gone.
	for _, gone := range []string{"template", "parameters", "properties"} {
		if _, ok := s.Attributes[gone]; ok {
			t.Errorf("attribute %s must no longer exist", gone)
		}
	}
}

// TestJobTemplateResource_Create asserts the exact register body and that the
// template is read back afterwards, because registering only answers {code: 0}.
func TestJobTemplateResource_Create(t *testing.T) {
	var (
		postBody  []byte
		postPath  string
		postCount int
		gets      []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == jobTemplatesPath:
			postCount++
			postPath = r.URL.Path
			postBody, _ = io.ReadAll(r.Body)
			_, _ = w.Write([]byte(`{"code":0}`))
		case r.Method == http.MethodGet && r.URL.Path == jobTemplatesPath+"/test_run_get":
			gets = append(gets, r.URL.Path)
			_, _ = w.Write([]byte(jobTemplateResponseExample))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	r := newJobTemplateResource(t, server.URL)
	ctx := context.Background()
	s := jobTemplateSchema(t, r)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: jobTemplateRaw(t, s, shellConfigModel())}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if postCount != 1 || postPath != jobTemplatesPath {
		t.Fatalf("expected exactly one POST %s, got %d at %s", jobTemplatesPath, postCount, postPath)
	}
	assertJSONEqual(t, wantRegisterBody, postBody)
	if len(gets) != 1 {
		t.Fatalf("expected the template to be read back once, got %v", gets)
	}

	var state res.JobTemplateResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed to read created state: %v", resp.Diagnostics)
	}

	if got := state.ID.ValueString(); got != "test_metalake.test_run_get" {
		t.Errorf("expected id test_metalake.test_run_get, got %s", got)
	}
	if got := state.Name.ValueString(); got != "test_run_get" {
		t.Errorf("expected name test_run_get, got %s", got)
	}
	if got := state.JobType.ValueString(); got != "shell" {
		t.Errorf("expected job_type shell, got %s", got)
	}
	if got := state.Comment.ValueString(); got != "Test shell job template" {
		t.Errorf("expected the comment from the read-back, got %s", got)
	}
	if got := listValues(t, ctx, state.Arguments); !reflect.DeepEqual(got, []string{"{{arg1}}", "{{arg2}}"}) {
		t.Errorf("expected arguments [{{arg1}} {{arg2}}], got %v", got)
	}
	if got := mapValues(t, ctx, state.Environments); !reflect.DeepEqual(got, map[string]string{"ENV_VAR": "{{env_var}}"}) {
		t.Errorf("expected environments from the API, got %v", got)
	}
	if !state.CustomFields.IsNull() {
		t.Errorf("expected an empty customFields to stay null, got %v", state.CustomFields)
	}
	if got := listValues(t, ctx, state.Scripts); !reflect.DeepEqual(got, []string{shellCommonScript}) {
		t.Errorf("expected scripts [%s], got %v", shellCommonScript, got)
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

// TestJobTemplateResource_Update asserts the exact updates array and that the
// rename is PUT to the old name, because every other update resolves the
// template by its current name.
func TestJobTemplateResource_Update(t *testing.T) {
	var (
		putBody []byte
		putPath string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.Method != http.MethodPut {
			t.Errorf("expected only a PUT during update, got %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		putPath = r.URL.Path
		putBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"code":0,"jobTemplate":{"name":"new_test_run_get","jobType":"shell","comment":"Updated comment for the job template","executable":"` +
			updatedExecutable + `","arguments":["{{new_arg1}}","{{new_arg2}}"],"environments":{"NEW_ENV_VAR":"{{new_env_var}}"},"customFields":{"field1":"value1"},"scripts":["` +
			updatedScript + `"],"audit":{"createTime":"2025-08-12T02:14:28.205023Z","creator":"anonymous","lastModifier":"anonymous","lastModifiedTime":"2025-08-13T09:00:00.000000Z"}}}`))
	}))
	defer server.Close()

	r := newJobTemplateResource(t, server.URL)
	ctx := context.Background()
	s := jobTemplateSchema(t, r)

	plan := shellConfigModel()
	plan.Name = types.StringValue("new_test_run_get")
	plan.Comment = types.StringValue("Updated comment for the job template")
	plan.Executable = types.StringValue(updatedExecutable)
	plan.Arguments = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("{{new_arg1}}"), types.StringValue("{{new_arg2}}")})
	plan.Environments = types.MapValueMust(types.StringType, map[string]attr.Value{"NEW_ENV_VAR": types.StringValue("{{new_env_var}}")})
	plan.CustomFields = types.MapValueMust(types.StringType, map[string]attr.Value{"field1": types.StringValue("value1")})
	plan.Scripts = types.ListValueMust(types.StringType, []attr.Value{types.StringValue(updatedScript)})

	state := jobTemplateStateModel()
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: jobTemplateRaw(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: jobTemplateRaw(t, s, state)},
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if putPath != jobTemplatesPath+"/test_run_get" {
		t.Fatalf("expected the PUT to target the old name %s/test_run_get, got %s", jobTemplatesPath, putPath)
	}
	assertJSONEqual(t, wantUpdatesBody, putBody)

	var newState res.JobTemplateResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &newState)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed to read updated state: %v", resp.Diagnostics)
	}
	if got := newState.Name.ValueString(); got != "new_test_run_get" {
		t.Errorf("expected the renamed template, got %s", got)
	}
	if got := newState.ID.ValueString(); got != "test_metalake.new_test_run_get" {
		t.Errorf("expected id test_metalake.new_test_run_get, got %s", got)
	}
	if got := newState.Comment.ValueString(); got != "Updated comment for the job template" {
		t.Errorf("expected the updated comment, got %s", got)
	}
	if got := newState.Executable.ValueString(); got != updatedExecutable {
		t.Errorf("expected the updated executable, got %s", got)
	}
	if got := listValues(t, ctx, newState.Scripts); !reflect.DeepEqual(got, []string{updatedScript}) {
		t.Errorf("expected the updated scripts, got %v", got)
	}
}

// TestJobTemplateResource_Update_NoChangeDoesNotCallTheAPI proves an empty diff
// leaves the template alone instead of issuing a PUT with no updates.
func TestJobTemplateResource_Update_NoChangeDoesNotCallTheAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()

	r := newJobTemplateResource(t, server.URL)
	ctx := context.Background()
	s := jobTemplateSchema(t, r)

	model := jobTemplateStateModel()
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: jobTemplateRaw(t, s, model)},
		State: tfsdk.State{Schema: s, Raw: jobTemplateRaw(t, s, model)},
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsKnown() || resp.State.Raw.IsNull() {
		t.Fatal("expected the state to be re-established from the plan")
	}
}

func TestJobTemplateResource_Delete(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		body       string
	}{
		{name: "success", statusCode: http.StatusOK, body: `{"code":0,"dropped":true}`},
		// A real Gravitino 1.3.0 answers a missing template with 200 and
		// dropped:false, so that must not be treated as a failure either.
		{name: "already gone", statusCode: http.StatusOK, body: `{"code":0,"dropped":false}`},
		{name: "gone with a 404", statusCode: http.StatusNotFound, body: noSuchJobTemplateExceptionExample},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				deleteCalled bool
				deletePath   string
			)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
				if r.Method != http.MethodDelete {
					t.Errorf("expected DELETE, got %s", r.Method)
				}
				deleteCalled = true
				deletePath = r.URL.Path
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			r := newJobTemplateResource(t, server.URL)
			ctx := context.Background()
			s := jobTemplateSchema(t, r)

			state := jobTemplateStateModel()
			resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: jobTemplateRaw(t, s, state)}}
			r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: jobTemplateRaw(t, s, state)}}, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("deleting a job template must tolerate a 404: %v", resp.Diagnostics)
			}
			if !deleteCalled || deletePath != jobTemplatesPath+"/test_run_get" {
				t.Fatalf("expected DELETE %s/test_run_get, got %s", jobTemplatesPath, deletePath)
			}
		})
	}
}

// TestJobTemplateResource_ValidateConfig guards the shell/spark split: the API
// models them as distinct object types and rejects the foreign attributes.
func TestJobTemplateResource_ValidateConfig(t *testing.T) {
	shell := func() res.JobTemplateResourceModel {
		m := shellConfigModel()
		m.Comment = types.StringNull()
		m.Arguments = types.ListNull(types.StringType)
		m.Environments = types.MapNull(types.StringType)
		m.Scripts = types.ListNull(types.StringType)
		return m
	}
	spark := func() res.JobTemplateResourceModel {
		return res.JobTemplateResourceModel{
			ID:           types.StringNull(),
			Metalake:     types.StringValue("test_metalake"),
			Name:         types.StringValue("test_run_get_spark"),
			JobType:      types.StringValue("spark"),
			Comment:      types.StringNull(),
			Executable:   types.StringValue(shellExecutable),
			Arguments:    types.ListNull(types.StringType),
			Environments: types.MapNull(types.StringType),
			CustomFields: types.MapNull(types.StringType),
			Scripts:      types.ListNull(types.StringType),
			ClassName:    types.StringNull(),
			Jars:         types.ListNull(types.StringType),
			Files:        types.ListNull(types.StringType),
			Archives:     types.ListNull(types.StringType),
			Configs:      types.MapNull(types.StringType),
			Audit:        types.ObjectNull(res.AuditAttrTypes),
		}
	}

	cases := []struct {
		name     string
		model    res.JobTemplateResourceModel
		wantPath string
	}{
		{
			name: "shell rejects jars",
			model: func() res.JobTemplateResourceModel {
				m := shell()
				m.Jars = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("spark-job.jar")})
				return m
			}(),
			wantPath: "jars",
		},
		{
			name: "shell rejects class_name",
			model: func() res.JobTemplateResourceModel {
				m := shell()
				m.ClassName = types.StringValue("org.apache.spark.examples.SparkPi")
				return m
			}(),
			wantPath: "class_name",
		},
		{
			name: "shell rejects configs",
			model: func() res.JobTemplateResourceModel {
				m := shell()
				m.Configs = types.MapValueMust(types.StringType, map[string]attr.Value{"spark.executor.memory": types.StringValue("1g")})
				return m
			}(),
			wantPath: "configs",
		},
		{
			name: "spark rejects scripts",
			model: func() res.JobTemplateResourceModel {
				m := spark()
				m.Scripts = types.ListValueMust(types.StringType, []attr.Value{types.StringValue(shellCommonScript)})
				return m
			}(),
			wantPath: "scripts",
		},
		{
			name: "valid shell",
			model: func() res.JobTemplateResourceModel {
				m := shell()
				m.Scripts = types.ListValueMust(types.StringType, []attr.Value{types.StringValue(shellCommonScript)})
				return m
			}(),
		},
		{
			name: "valid spark",
			model: func() res.JobTemplateResourceModel {
				m := spark()
				m.ClassName = types.StringValue("org.apache.spark.examples.SparkPi")
				m.Jars = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("spark-job.jar")})
				m.Configs = types.MapValueMust(types.StringType, map[string]attr.Value{"spark.executor.memory": types.StringValue("1g")})
				return m
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := res.NewJobTemplateResource().(resource.ResourceWithValidateConfig)
			ctx := context.Background()
			s := jobTemplateSchema(t, r.(resource.Resource))

			resp := &resource.ValidateConfigResponse{}
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{
				Config: tfsdk.Config{Schema: s, Raw: jobTemplateRaw(t, s, tc.model)},
			}, resp)

			if tc.wantPath == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("expected the config to be valid, got %v", resp.Diagnostics)
				}
				return
			}

			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected a diagnostic on %s", tc.wantPath)
			}
			diagWithPath, ok := resp.Diagnostics.Errors()[0].(diag.DiagnosticWithPath)
			if !ok {
				t.Fatalf("expected the diagnostic to carry an attribute path, got %T", resp.Diagnostics.Errors()[0])
			}
			if !diagWithPath.Path().Equal(path.Root(tc.wantPath)) {
				t.Errorf("expected the diagnostic on %s, got %s", tc.wantPath, diagWithPath.Path())
			}
		})
	}
}

func TestJobTemplateResource_ImportState(t *testing.T) {
	r := res.NewJobTemplateResource().(resource.ResourceWithImportState)

	ctx := context.Background()
	s := jobTemplateSchema(t, r.(resource.Resource))

	resp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: s, Raw: jobTemplateRaw(t, s, jobTemplateNullModel())},
	}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "my_metalake.my_template"}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	for attr, want := range map[string]string{
		"metalake": "my_metalake",
		"name":     "my_template",
		"id":       "my_metalake.my_template",
	} {
		var got types.String
		resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root(attr), &got)...)
		if got.ValueString() != want {
			t.Errorf("expected %s %q, got %q", attr, want, got.ValueString())
		}
	}
}

func TestJobTemplateResource_ImportState_Invalid(t *testing.T) {
	r := res.NewJobTemplateResource().(resource.ResourceWithImportState)

	ctx := context.Background()
	s := jobTemplateSchema(t, r.(resource.Resource))

	for _, id := range []string{"no_dot_here", ".x", "x."} {
		t.Run(id, func(t *testing.T) {
			resp := &resource.ImportStateResponse{
				State: tfsdk.State{Schema: s, Raw: jobTemplateRaw(t, s, jobTemplateNullModel())},
			}
			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)

			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error for import ID %q", id)
			}
		})
	}
}
