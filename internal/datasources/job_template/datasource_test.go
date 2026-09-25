package job_template_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/job_template"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The paths below come from the Gravitino v1.3.0 jobs spec examples
// (docs/open-api/jobs.yaml, components/examples).
const (
	shellExecutable   = "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/test-job.sh"
	shellCommonScript = "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/common.sh"
	sparkExecutable   = "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/spark-demo.jar"
	sparkJar          = "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/spark-job.jar"
)

// jobTemplateResponseExample is the exact `JobTemplateResponse` example of the
// spec: a shell template.
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

// jobTemplateListResponseExample is the exact `JobTemplateListResponse` example
// of the spec: one shell and one spark template.
const jobTemplateListResponseExample = `{
  "code": 0,
  "jobTemplates": [
    {
      "arguments": ["{{arg1}}", "{{arg2}}"],
      "audit": {
        "createTime": "2025-08-12T02:14:28.205023Z",
        "creator": "anonymous"
      },
      "comment": "Test shell job template",
      "customFields": { },
      "environments": {
        "ENV_VAR": "{{env_var}}"
      },
      "executable": "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/test-job.sh",
      "jobType": "shell",
      "name": "test_run_get",
      "scripts": [
        "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/common.sh"
      ]
    },
    {
      "arguments": ["--arg1", "{{arg2}}"],
      "audit": {
        "createTime": "2025-08-12T02:14:28.205023Z",
        "creator": "anonymous"
      },
      "comment": "Test spark job template",
      "customFields": { },
      "environments": {
        "ENV_VAR": "{{env_var}}"
      },
      "executable": "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/spark-demo.jar",
      "jobType": "spark",
      "name": "test_run_get_spark",
      "className": "org.apache.spark.examples.SparkPi",
      "jars": [
        "/var/folders/90/v1d9hxsd6pj8m0jnn6f22tkr0000gn/T/tmpy65fiugc/spark-job.jar"
      ]
    }
  ]
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

// jobTemplateItem mirrors ds.JobTemplateItemAttrTypes so a nested list value can
// be decoded into a struct in assertions.
type jobTemplateItem struct {
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

func newClient(t *testing.T, serverURL string) *client.Client {
	t.Helper()
	c, err := client.New(serverURL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	return c
}

func jobTemplateSchema(t *testing.T, d datasource.DataSource) schema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func jobTemplateRaw(t *testing.T, s schema.Schema, model interface{}) tftypes.Value {
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

func TestJobTemplateDataSource_Metadata(t *testing.T) {
	d := ds.NewJobTemplateDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_job_template" {
		t.Fatalf("expected gravitino_job_template, got %s", resp.TypeName)
	}
}

func TestJobTemplatesDataSource_Metadata(t *testing.T) {
	d := ds.NewJobTemplatesDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_job_templates" {
		t.Fatalf("expected gravitino_job_templates, got %s", resp.TypeName)
	}
}

func TestJobTemplateDataSource_Read(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(jobTemplateResponseExample))
	}))
	defer server.Close()

	d := ds.NewJobTemplateDataSource()
	d.(*ds.JobTemplateDataSource).SetClient(newClient(t, server.URL))

	ctx := context.Background()
	s := jobTemplateSchema(t, d)

	config := emptyJobTemplateConfig()

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: jobTemplateRaw(t, s, config)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotPath != jobTemplatesPath+"/test_run_get" {
		t.Fatalf("expected GET %s/test_run_get, got %s", jobTemplatesPath, gotPath)
	}

	var state ds.JobTemplateDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)

	if got := state.Name.ValueString(); got != "test_run_get" {
		t.Errorf("expected name test_run_get, got %s", got)
	}
	if got := state.JobType.ValueString(); got != "shell" {
		t.Errorf("expected job_type shell, got %s", got)
	}
	if got := state.Comment.ValueString(); got != "Test shell job template" {
		t.Errorf("expected the spec comment, got %s", got)
	}
	if got := listValues(t, ctx, state.Arguments); !reflect.DeepEqual(got, []string{"{{arg1}}", "{{arg2}}"}) {
		t.Errorf("expected the spec arguments, got %v", got)
	}
	if got := mapValues(t, ctx, state.Environments); !reflect.DeepEqual(got, map[string]string{"ENV_VAR": "{{env_var}}"}) {
		t.Errorf("expected the spec environments, got %v", got)
	}
	if got := listValues(t, ctx, state.Scripts); !reflect.DeepEqual(got, []string{shellCommonScript}) {
		t.Errorf("expected the spec scripts, got %v", got)
	}
	if !state.CustomFields.IsNull() {
		t.Errorf("expected the spec's empty customFields to be null, got %v", state.CustomFields)
	}
	if !state.ClassName.IsNull() {
		t.Errorf("expected class_name to be null for a shell template, got %q", state.ClassName.ValueString())
	}
	if !state.Configs.IsNull() {
		t.Errorf("expected configs to be null for a shell template, got %v", state.Configs)
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

func TestJobTemplateDataSource_Read_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(noSuchJobTemplateExceptionExample))
	}))
	defer server.Close()

	d := ds.NewJobTemplateDataSource()
	d.(*ds.JobTemplateDataSource).SetClient(newClient(t, server.URL))

	ctx := context.Background()
	s := jobTemplateSchema(t, d)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: s, Raw: jobTemplateRaw(t, s, emptyJobTemplateConfig())},
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for a missing job template")
	}
	if len(resp.Diagnostics.Errors()) != 1 {
		t.Fatalf("expected exactly one error diagnostic, got %v", resp.Diagnostics.Errors())
	}
	errDiag := resp.Diagnostics.Errors()[0]
	if !strings.Contains(errDiag.Summary(), "Job template \"test_run_get\" not found") {
		t.Errorf("expected a not-found summary, got %q", errDiag.Summary())
	}
	if !strings.Contains(errDiag.Detail(), "test_metalake") {
		t.Errorf("expected the metalake to be named in the detail, got %q", errDiag.Detail())
	}
}

func TestJobTemplatesDataSource_Read(t *testing.T) {
	var gotPath, gotQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(jobTemplateListResponseExample))
	}))
	defer server.Close()

	d := ds.NewJobTemplatesDataSource()
	d.(*ds.JobTemplatesDataSource).SetClient(newClient(t, server.URL))

	ctx := context.Background()
	s := jobTemplateSchema(t, d)

	config := ds.JobTemplatesDataSourceModel{
		Metalake:     types.StringValue("test_metalake"),
		JobTemplates: types.ListNull(types.ObjectType{AttrTypes: ds.JobTemplateItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: jobTemplateRaw(t, s, config)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotPath != jobTemplatesPath || gotQuery != "details=true" {
		t.Fatalf("expected GET %s?details=true, got %s?%s", jobTemplatesPath, gotPath, gotQuery)
	}

	items := jobTemplateItemsFromState(t, ctx, resp)
	if len(items) != 2 {
		t.Fatalf("expected 2 job templates, got %d", len(items))
	}

	shell := items[0]
	if shell.Name.ValueString() != "test_run_get" || shell.JobType.ValueString() != "shell" {
		t.Errorf("unexpected shell template: %+v", shell)
	}
	if got := listValues(t, ctx, shell.Scripts); !reflect.DeepEqual(got, []string{shellCommonScript}) {
		t.Errorf("expected the shell scripts, got %v", got)
	}
	if !shell.Jars.IsNull() || !shell.ClassName.IsNull() {
		t.Error("expected the shell template to have no spark-only attributes")
	}

	spark := items[1]
	if spark.Name.ValueString() != "test_run_get_spark" || spark.JobType.ValueString() != "spark" {
		t.Errorf("unexpected spark template: %+v", spark)
	}
	if got := spark.ClassName.ValueString(); got != "org.apache.spark.examples.SparkPi" {
		t.Errorf("expected the spec class name, got %s", got)
	}
	if got := spark.Executable.ValueString(); got != sparkExecutable {
		t.Errorf("expected the spec spark executable, got %s", got)
	}
	if got := listValues(t, ctx, spark.Jars); !reflect.DeepEqual(got, []string{sparkJar}) {
		t.Errorf("expected the spec jars, got %v", got)
	}
	if got := listValues(t, ctx, spark.Arguments); !reflect.DeepEqual(got, []string{"--arg1", "{{arg2}}"}) {
		t.Errorf("expected the spec spark arguments, got %v", got)
	}

	for i, item := range items {
		var audit models.AuditTFSDK
		resp.Diagnostics.Append(item.Audit.As(ctx, &audit, basetypes.ObjectAsOptions{})...)
		if got := audit.Creator.ValueString(); got != "anonymous" {
			t.Errorf("template %d: expected audit.creator anonymous, got %s", i, got)
		}
	}
}

func TestJobTemplatesDataSource_Read_EmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code":0,"jobTemplates":[]}`))
	}))
	defer server.Close()

	d := ds.NewJobTemplatesDataSource()
	d.(*ds.JobTemplatesDataSource).SetClient(newClient(t, server.URL))

	ctx := context.Background()
	s := jobTemplateSchema(t, d)

	config := ds.JobTemplatesDataSourceModel{
		Metalake:     types.StringValue("test_metalake"),
		JobTemplates: types.ListNull(types.ObjectType{AttrTypes: ds.JobTemplateItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: jobTemplateRaw(t, s, config)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state ds.JobTemplatesDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
	if !state.JobTemplates.IsNull() && len(state.JobTemplates.Elements()) != 0 {
		t.Fatalf("expected an empty job_templates list, got %v", state.JobTemplates)
	}
}

func emptyJobTemplateConfig() ds.JobTemplateDataSourceModel {
	return ds.JobTemplateDataSourceModel{
		Metalake:     types.StringValue("test_metalake"),
		Name:         types.StringValue("test_run_get"),
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
		Audit:        types.ObjectNull(ds.AuditAttrTypes),
	}
}

func jobTemplateItemsFromState(t *testing.T, ctx context.Context, resp *datasource.ReadResponse) []jobTemplateItem {
	t.Helper()

	var state ds.JobTemplatesDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed to read state: %v", resp.Diagnostics)
	}

	var items []jobTemplateItem
	resp.Diagnostics.Append(state.JobTemplates.ElementsAs(ctx, &items, false)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed to decode job_templates list: %v", resp.Diagnostics)
	}
	return items
}
