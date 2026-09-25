package model_version_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"reflect"
	"strconv"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/model_version"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// modelVersionResponseExample is the `ModelVersionResponse` example of the
// Gravitino v1.3.0 OpenAPI spec (docs/open-api/models.yaml), served verbatim.
const modelVersionResponseExample = `{
  "code": 0,
  "modelVersion": {
    "uris": {
      "hdfs": "hdfs://path/to/model",
      "s3": "s3://path/to/model"
    },
    "version": 0,
    "aliases": ["alias1", "alias2"],
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

// modelVersionInfoListResponseExample is the `ModelVersionInfoListResponse`
// example of the spec, returned when details=true is honored.
const modelVersionInfoListResponseExample = `{
  "code": 0,
  "infos": [
    {
      "uri": "hdfs://path/to/model",
      "version": 0,
      "aliases": ["alias1", "alias2"],
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
  ]
}`

// modelVersionResponseFor is the spec's `ModelVersionResponse` example with the
// version number of the requested model version.
func modelVersionResponseFor(version int) string {
	return fmt.Sprintf(`{
  "code": 0,
  "modelVersion": {
    "uris": {
      "hdfs": "hdfs://path/to/model",
      "s3": "s3://path/to/model"
    },
    "version": %d,
    "aliases": ["alias1", "alias2"],
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
}`, version)
}

func newModelVersionDataSource(server *httptest.Server) *ds.ModelVersionDataSource {
	c, _ := client.New(server.URL, nil)
	d := ds.NewModelVersionDataSource().(*ds.ModelVersionDataSource)
	d.SetClient(c)
	return d
}

func newModelVersionsDataSource(server *httptest.Server) *ds.ModelVersionsDataSource {
	c, _ := client.New(server.URL, nil)
	d := ds.NewModelVersionsDataSource().(*ds.ModelVersionsDataSource)
	d.SetClient(c)
	return d
}

func dsSchema(t *testing.T, d datasource.DataSource) (datasource.SchemaResponse, types.ObjectType) {
	t.Helper()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	return *schemaResp, schemaResp.Schema.Type().(types.ObjectType)
}

func dsConfig(t *testing.T, sch datasource.SchemaResponse, attrTypes map[string]attr.Type, model interface{}) tfsdk.Config {
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

func nullModelVersionConfig() ds.ModelVersionDataSourceModel {
	return ds.ModelVersionDataSourceModel{
		Metalake:   types.StringValue("ml"),
		Catalog:    types.StringValue("cat"),
		Schema:     types.StringValue("sch"),
		Model:      types.StringValue("model1"),
		Version:    types.Int64Null(),
		Alias:      types.StringNull(),
		URI:        types.StringNull(),
		URIs:       types.MapNull(types.StringType),
		Aliases:    types.SetNull(types.StringType),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(ds.AuditAttrTypes),
	}
}

func TestModelVersionDataSources_Metadata(t *testing.T) {
	d := ds.NewModelVersionDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_model_version" {
		t.Fatalf("expected gravitino_model_version, got %s", resp.TypeName)
	}

	versions := ds.NewModelVersionsDataSource()
	resp = &datasource.MetadataResponse{}
	versions.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_model_versions" {
		t.Fatalf("expected gravitino_model_versions, got %s", resp.TypeName)
	}
}

func TestModelVersionDataSource_Schema(t *testing.T) {
	sch, _ := dsSchema(t, ds.NewModelVersionDataSource())
	for _, name := range []string{"metalake", "catalog", "schema", "model", "version", "alias", "uri", "uris", "aliases", "comment", "properties", "audit"} {
		if _, ok := sch.Schema.Attributes[name]; !ok {
			t.Errorf("expected attribute %q in the schema", name)
		}
	}
	if got := sch.Schema.Attributes["version"].GetType(); got != types.Int64Type {
		t.Errorf("version must be an int64, got %s", got)
	}
	if got := sch.Schema.Attributes["uris"].GetType(); got != (types.MapType{ElemType: types.StringType}) {
		t.Errorf("uris must be a map of uri names to uris, got %s", got)
	}
}

func TestModelVersionDataSource_ReadByVersion(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(modelVersionResponseExample))
	}))
	defer server.Close()

	d := newModelVersionDataSource(server)
	ctx := context.Background()
	sch, objType := dsSchema(t, d)

	config := nullModelVersionConfig()
	config.Version = types.Int64Value(0)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: dsConfig(t, sch, objType.AttributeTypes(), config)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotPath != "/api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions/0" {
		t.Fatalf("unexpected path: %s", gotPath)
	}

	var got ds.ModelVersionDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Version.ValueInt64() != 0 {
		t.Errorf("unexpected version: %d", got.Version.ValueInt64())
	}
	if got.URIs.Elements()["hdfs"].(types.String).ValueString() != "hdfs://path/to/model" {
		t.Errorf("unexpected uris: %v", got.URIs)
	}
	if len(got.Aliases.Elements()) != 2 {
		t.Errorf("unexpected aliases: %v", got.Aliases)
	}
	if got.Comment.ValueString() != "This is a comment" {
		t.Errorf("unexpected comment: %q", got.Comment.ValueString())
	}
}

func TestModelVersionDataSource_ReadByAlias(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(modelVersionResponseExample))
	}))
	defer server.Close()

	d := newModelVersionDataSource(server)
	ctx := context.Background()
	sch, objType := dsSchema(t, d)

	config := nullModelVersionConfig()
	config.Alias = types.StringValue("alias1")

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: dsConfig(t, sch, objType.AttributeTypes(), config)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	// Aliases live under /aliases/{alias}, not under /versions/aliases/{alias}.
	if gotPath != "/api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/aliases/alias1" {
		t.Fatalf("unexpected path: %s", gotPath)
	}

	var got ds.ModelVersionDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Version.ValueInt64() != 0 {
		t.Errorf("version must be filled from the alias lookup, got %d", got.Version.ValueInt64())
	}
}

func TestModelVersionDataSource_ReadWithoutVersionOrAlias(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()

	d := newModelVersionDataSource(server)
	ctx := context.Background()
	sch, objType := dsSchema(t, d)

	for name, config := range map[string]ds.ModelVersionDataSourceModel{
		"neither": nullModelVersionConfig(),
		"both": func() ds.ModelVersionDataSourceModel {
			c := nullModelVersionConfig()
			c.Version = types.Int64Value(0)
			c.Alias = types.StringValue("alias1")
			return c
		}(),
	} {
		resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch.Schema}}
		d.Read(ctx, datasource.ReadRequest{Config: dsConfig(t, sch, objType.AttributeTypes(), config)}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected an error when %s version and alias are set", name)
		}
	}
	if requests != 0 {
		t.Fatalf("expected no request, got %d", requests)
	}
}

func TestModelVersionDataSource_ReadNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{
  "code": 1003,
  "type": "NoSuchModelVersionException",
  "message": "Model version does not exist",
  "stack": ["org.apache.gravitino.exceptions.NoSuchModelVersionException: Model version does not exist"]
}`))
	}))
	defer server.Close()

	d := newModelVersionDataSource(server)
	ctx := context.Background()
	sch, objType := dsSchema(t, d)

	config := nullModelVersionConfig()
	config.Version = types.Int64Value(7)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: dsConfig(t, sch, objType.AttributeTypes(), config)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("a 404 must be reported as an error diagnostic")
	}
}

// TestModelVersionsDataSource_ReadVersionNumbers is the regression test for the
// `versions` key of the spec's ModelVersionListResponse: the provider used to
// read `modelVersions`, so the list came back empty.
func TestModelVersionsDataSource_ReadVersionNumbers(t *testing.T) {
	var gotRequests []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequests = append(gotRequests, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case "/api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions":
			// ModelVersionListResponse example of the spec, verbatim.
			_, _ = w.Write([]byte(`{
  "code": 0,
  "versions": [0, 1, 2]
}`))
		default:
			version, _ := strconv.Atoi(path.Base(r.URL.Path))
			_, _ = w.Write([]byte(modelVersionResponseFor(version)))
		}
	}))
	defer server.Close()

	d := newModelVersionsDataSource(server)
	ctx := context.Background()
	sch, objType := dsSchema(t, d)

	config := ds.ModelVersionsDataSourceModel{
		Metalake: types.StringValue("ml"),
		Catalog:  types.StringValue("cat"),
		Schema:   types.StringValue("sch"),
		Model:    types.StringValue("model1"),
		Versions: types.ListNull(types.ObjectType{AttrTypes: ds.ModelVersionItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: dsConfig(t, sch, objType.AttributeTypes(), config)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	wantRequests := []string{
		"GET /api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions?details=true",
		"GET /api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions/0",
		"GET /api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions/1",
		"GET /api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions/2",
	}
	if !reflect.DeepEqual(gotRequests, wantRequests) {
		t.Fatalf("unexpected requests:\ngot  %v\nwant %v", gotRequests, wantRequests)
	}

	var got ds.ModelVersionsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	elements := got.Versions.Elements()
	if len(elements) != 3 {
		t.Fatalf("the list data source must return the version numbers instead of an empty list, got %v", got.Versions)
	}
	for i, version := range []int64{0, 1, 2} {
		item := elements[i].(types.Object)
		if gotVersion := item.Attributes()["version"].(types.Int64).ValueInt64(); gotVersion != version {
			t.Errorf("expected version %d at index %d, got %d", version, i, gotVersion)
		}
	}
}

func TestModelVersionsDataSource_ReadInfos(t *testing.T) {
	requests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(modelVersionInfoListResponseExample))
	}))
	defer server.Close()

	d := newModelVersionsDataSource(server)
	ctx := context.Background()
	sch, objType := dsSchema(t, d)

	config := ds.ModelVersionsDataSourceModel{
		Metalake: types.StringValue("ml"),
		Catalog:  types.StringValue("cat"),
		Schema:   types.StringValue("sch"),
		Model:    types.StringValue("model1"),
		Versions: types.ListNull(types.ObjectType{AttrTypes: ds.ModelVersionItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: dsConfig(t, sch, objType.AttributeTypes(), config)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if requests != 1 {
		t.Fatalf("expected a single request when the details list is returned, got %d", requests)
	}

	var got ds.ModelVersionsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	elements := got.Versions.Elements()
	if len(elements) != 1 {
		t.Fatalf("expected 1 model version, got %v", got.Versions)
	}
	item := elements[0].(types.Object)
	if uri := item.Attributes()["uri"].(types.String).ValueString(); uri != "hdfs://path/to/model" {
		t.Errorf("unexpected uri: %q", uri)
	}
	if len(item.Attributes()["aliases"].(types.Set).Elements()) != 2 {
		t.Errorf("unexpected aliases: %v", item.Attributes()["aliases"])
	}
}

func TestModelVersionsDataSource_ReadEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code": 0, "infos": []}`))
	}))
	defer server.Close()

	d := newModelVersionsDataSource(server)
	ctx := context.Background()
	sch, objType := dsSchema(t, d)

	config := ds.ModelVersionsDataSourceModel{
		Metalake: types.StringValue("ml"),
		Catalog:  types.StringValue("cat"),
		Schema:   types.StringValue("sch"),
		Model:    types.StringValue("model1"),
		Versions: types.ListNull(types.ObjectType{AttrTypes: ds.ModelVersionItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: dsConfig(t, sch, objType.AttributeTypes(), config)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got ds.ModelVersionsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Versions.IsNull() || got.Versions.IsUnknown() || len(got.Versions.Elements()) != 0 {
		t.Fatalf("expected a known empty list, got %v", got.Versions)
	}
}
