package model_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/model"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// modelResponseExample is the `ModelResponse` example of the Gravitino v1.3.0
// OpenAPI spec (docs/open-api/models.yaml), served verbatim by the mocks. Note
// that a model has no uri: only its versions carry artifact locations.
const modelResponseExample = `{
  "code": 0,
  "model": {
    "name": "model1",
    "latestVersion": 0,
    "comment": "This is a comment",
    "properties": {
      "key1": "value1",
      "key2": "value2"
    },
    "audit": {
      "creator": "user1",
      "createTime": "2021-01-01T00:00:00Z",
      "lastModifier": "user1",
      "lastModifiedTime": "2021-01-01T00:00:00Z"
    }
  }
}`

// noSuchModelExample is the `NoSuchModelException` example of the spec.
const noSuchModelExample = `{
  "code": 1003,
  "type": "NoSuchModelException",
  "message": "Model does not exist",
  "stack": ["org.apache.gravitino.exceptions.NoSuchModelException: Model does not exist"]
}`

func newModelDataSource(server *httptest.Server) *ds.ModelDataSource {
	c, _ := client.New(server.URL, nil)
	d := ds.NewModelDataSource().(*ds.ModelDataSource)
	d.SetClient(c)
	return d
}

func dataSourceSchema(t *testing.T, d datasource.DataSource) (datasource.SchemaResponse, types.ObjectType) {
	t.Helper()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	return *schemaResp, schemaResp.Schema.Type().(types.ObjectType)
}

func configFrom(t *testing.T, sch datasource.SchemaResponse, attrTypes map[string]attr.Type, model interface{}) tfsdk.Config {
	t.Helper()
	obj, diags := types.ObjectValueFrom(context.Background(), attrTypes, model)
	if diags.HasError() {
		t.Fatalf("failed to create config object: %v", diags)
	}
	tfVal, err := obj.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}
	return tfsdk.Config{Schema: sch.Schema, Raw: tfVal}
}

func TestModelDataSource_Metadata(t *testing.T) {
	d := ds.NewModelDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_model" {
		t.Fatalf("expected gravitino_model, got %s", resp.TypeName)
	}

	modelsDataSource := ds.NewModelsDataSource()
	resp = &datasource.MetadataResponse{}
	modelsDataSource.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_models" {
		t.Fatalf("expected gravitino_models, got %s", resp.TypeName)
	}
}

func TestModelDataSource_Schema(t *testing.T) {
	sch, _ := dataSourceSchema(t, ds.NewModelDataSource())
	for _, name := range []string{"metalake", "catalog", "schema", "name", "comment", "latest_version", "properties", "audit"} {
		if _, ok := sch.Schema.Attributes[name]; !ok {
			t.Errorf("expected attribute %q in the schema", name)
		}
	}
	if _, ok := sch.Schema.Attributes["model_uri"]; ok {
		t.Error("model_uri must not exist: a Model has no modelUri field in the v1.3.0 spec")
	}
	if got := sch.Schema.Attributes["latest_version"].GetType(); got != types.Int64Type {
		t.Errorf("latest_version must be an int64, got %s", got)
	}
}

func TestModelDataSource_Read_SpecExample(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metalakes/ml/catalogs/cat/schemas/sch/models/model1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(modelResponseExample))
	}))
	defer server.Close()

	d := newModelDataSource(server)
	ctx := context.Background()
	sch, objType := dataSourceSchema(t, d)

	config := ds.ModelDataSourceModel{
		Metalake:      types.StringValue("ml"),
		Catalog:       types.StringValue("cat"),
		Schema:        types.StringValue("sch"),
		Name:          types.StringValue("model1"),
		Comment:       types.StringNull(),
		LatestVersion: types.Int64Null(),
		Properties:    types.MapNull(types.StringType),
		Audit:         types.ObjectNull(ds.AuditAttrTypes),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: configFrom(t, sch, objType.AttributeTypes(), config)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got ds.ModelDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Comment.ValueString() != "This is a comment" {
		t.Errorf("unexpected comment: %q", got.Comment.ValueString())
	}
	if got.LatestVersion.ValueInt64() != 0 {
		t.Errorf("unexpected latest_version: %d", got.LatestVersion.ValueInt64())
	}
	if got.Properties.Elements()["key2"].(types.String).ValueString() != "value2" {
		t.Errorf("unexpected properties: %v", got.Properties)
	}
	if got.Audit.IsNull() {
		t.Fatal("expected the audit in state")
	}
}

func TestModelDataSource_Read_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(noSuchModelExample))
	}))
	defer server.Close()

	d := newModelDataSource(server)
	ctx := context.Background()
	sch, objType := dataSourceSchema(t, d)

	config := ds.ModelDataSourceModel{
		Metalake:      types.StringValue("ml"),
		Catalog:       types.StringValue("cat"),
		Schema:        types.StringValue("sch"),
		Name:          types.StringValue("model1"),
		Comment:       types.StringNull(),
		LatestVersion: types.Int64Null(),
		Properties:    types.MapNull(types.StringType),
		Audit:         types.ObjectNull(ds.AuditAttrTypes),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: configFrom(t, sch, objType.AttributeTypes(), config)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("a 404 must be reported as an error diagnostic")
	}
}

func TestModelsDataSource_Read(t *testing.T) {
	var gotRequests []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequests = append(gotRequests, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case "/api/metalakes/ml/catalogs/cat/schemas/sch/models":
			// The list endpoint returns identifiers only, which is why every
			// model needs an extra GET.
			_, _ = w.Write([]byte(`{"code": 0, "identifiers": [{"namespace": ["ml", "cat", "sch"], "name": "model1"}, {"namespace": ["ml", "cat", "sch"], "name": "model2"}]}`))
		default:
			_, _ = w.Write([]byte(modelResponseExample))
		}
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewModelsDataSource().(*ds.ModelsDataSource)
	d.SetClient(c)

	ctx := context.Background()
	sch, objType := dataSourceSchema(t, d)

	config := ds.ModelsDataSourceModel{
		Metalake: types.StringValue("ml"),
		Catalog:  types.StringValue("cat"),
		Schema:   types.StringValue("sch"),
		Models:   types.ListNull(types.ObjectType{AttrTypes: ds.ModelItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: configFrom(t, sch, objType.AttributeTypes(), config)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	wantRequests := []string{
		"GET /api/metalakes/ml/catalogs/cat/schemas/sch/models",
		"GET /api/metalakes/ml/catalogs/cat/schemas/sch/models/model1",
		"GET /api/metalakes/ml/catalogs/cat/schemas/sch/models/model2",
	}
	if !reflect.DeepEqual(gotRequests, wantRequests) {
		t.Fatalf("unexpected requests:\ngot  %v\nwant %v", gotRequests, wantRequests)
	}

	var got ds.ModelsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if len(got.Models.Elements()) != 2 {
		t.Fatalf("expected 2 models in state, got %v", got.Models)
	}
	if got.Models.IsUnknown() {
		t.Fatal("models must be known after apply")
	}
}

func TestModelsDataSource_Read_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code": 0, "identifiers": []}`))
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewModelsDataSource().(*ds.ModelsDataSource)
	d.SetClient(c)

	ctx := context.Background()
	sch, objType := dataSourceSchema(t, d)

	config := ds.ModelsDataSourceModel{
		Metalake: types.StringValue("ml"),
		Catalog:  types.StringValue("cat"),
		Schema:   types.StringValue("sch"),
		Models:   types.ListNull(types.ObjectType{AttrTypes: ds.ModelItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: configFrom(t, sch, objType.AttributeTypes(), config)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got ds.ModelsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Models.IsNull() || got.Models.IsUnknown() || len(got.Models.Elements()) != 0 {
		t.Fatalf("expected a known empty list, got %v", got.Models)
	}
}
