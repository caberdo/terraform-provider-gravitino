package model_version_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/model_version"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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

// noSuchModelVersionExample is the `NoSuchModelVersionException` example of the spec.
const noSuchModelVersionExample = `{
  "code": 1003,
  "type": "NoSuchModelVersionException",
  "message": "Model version does not exist",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchModelVersionException: Model version does not exist"
  ]
}`

// modelVersionListResponseExample is the `ModelVersionListResponse` example of the
// spec: the shape Gravitino returns when it reports version numbers only.
const modelVersionListResponseExample = `{
  "code": 0,
  "versions": [0, 1, 2]
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

// modelVersionResponseFor is the `ModelVersionResponse` example of the spec with
// the version number of the requested model version.
func modelVersionResponseFor(version int) string {
	return fmt.Sprintf(`{
  "code": 0,
  "modelVersion": {
    "uri": "hdfs://path/to/model",
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

func modelVersionSchema(t *testing.T, r resource.Resource) schema.Schema {
	t.Helper()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	return schemaResp.Schema
}

func modelVersionPlan(t *testing.T, sch schema.Schema, m res.ModelVersionResourceModel) tfsdk.Plan {
	t.Helper()
	obj, diags := types.ObjectValueFrom(context.Background(), sch.Type().(types.ObjectType).AttributeTypes(), m)
	if diags.HasError() {
		t.Fatalf("failed to create object: %v", diags)
	}
	tfVal, err := obj.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}
	return tfsdk.Plan{Schema: sch, Raw: tfVal}
}

func modelVersionState(t *testing.T, sch schema.Schema, m res.ModelVersionResourceModel) tfsdk.State {
	t.Helper()
	obj, diags := types.ObjectValueFrom(context.Background(), sch.Type().(types.ObjectType).AttributeTypes(), m)
	if diags.HasError() {
		t.Fatalf("failed to create object: %v", diags)
	}
	tfVal, err := obj.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}
	return tfsdk.State{Schema: sch, Raw: tfVal}
}

func newModelVersionResource(server *httptest.Server) (resource.Resource, *res.ModelVersionResource) {
	uri := ""
	if server != nil {
		uri = server.URL
	}
	c, _ := client.New(uri, nil)
	r := res.NewModelVersionResource()
	concrete := r.(*res.ModelVersionResource)
	concrete.SetClient(c)
	return r, concrete
}

func TestModelVersionResource_Schema(t *testing.T) {
	r, _ := newModelVersionResource(nil)
	metadataResp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, metadataResp)
	if metadataResp.TypeName != "gravitino_model_version" {
		t.Fatalf("expected gravitino_model_version, got %s", metadataResp.TypeName)
	}

	sch := modelVersionSchema(t, r)
	for _, name := range []string{"id", "metalake", "catalog", "schema", "model", "version", "uri", "uris", "aliases", "comment", "properties", "audit"} {
		if _, ok := sch.Attributes[name]; !ok {
			t.Errorf("expected attribute %q in the schema", name)
		}
	}
	if got := sch.Attributes["version"].GetType(); got != types.Int64Type {
		t.Errorf("version must be an int64, got %s", got)
	}
	// Gravitino assigns the version number when a version is linked, so version
	// cannot be required.
	if !sch.Attributes["version"].IsComputed() {
		t.Error("version must be computed: the spec's ModelVersionLinkRequest has no version field")
	}
	if got := sch.Attributes["uris"].GetType(); got != (types.MapType{ElemType: types.StringType}) {
		t.Errorf("uris must be a map, got %s", got)
	}
}

func TestModelVersionResource_Create_SpecExample(t *testing.T) {
	var gotBody map[string]interface{}
	var gotRequests []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequests = append(gotRequests, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			// linkModelVersion answers with a BaseResponse.
			_, _ = w.Write([]byte(`{"code": 0}`))
			return
		}
		// The version number assigned by the server is read back afterwards.
		_, _ = w.Write([]byte(modelVersionInfoListResponseExample))
	}))
	defer server.Close()

	r, _ := newModelVersionResource(server)
	ctx := context.Background()
	sch := modelVersionSchema(t, r)

	plan := res.ModelVersionResourceModel{
		Metalake: types.StringValue("ml"),
		Catalog:  types.StringValue("cat"),
		Schema:   types.StringValue("sch"),
		Model:    types.StringValue("model1"),
		Version:  types.Int64Unknown(),
		URI:      types.StringValue("hdfs://path/to/model"),
		URIs:     types.MapUnknown(types.StringType),
		Aliases: types.SetValueMust(types.StringType, []attr.Value{
			types.StringValue("alias1"),
			types.StringValue("alias2"),
		}),
		Comment: types.StringValue("This is a comment"),
		Properties: types.MapValueMust(types.StringType, map[string]attr.Value{
			"key1": types.StringValue("value1"),
			"key2": types.StringValue("value2"),
		}),
		Audit: types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(ctx, resource.CreateRequest{Plan: modelVersionPlan(t, sch, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	wantRequests := []string{
		"POST /api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions",
		"GET /api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions?details=true",
	}
	if !reflect.DeepEqual(gotRequests, wantRequests) {
		t.Fatalf("unexpected requests:\ngot  %v\nwant %v", gotRequests, wantRequests)
	}
	// ModelVersionLinkRequest example of the spec.
	want := map[string]interface{}{
		"uri":        "hdfs://path/to/model",
		"aliases":    []interface{}{"alias1", "alias2"},
		"comment":    "This is a comment",
		"properties": map[string]interface{}{"key1": "value1", "key2": "value2"},
	}
	if !reflect.DeepEqual(gotBody, want) {
		t.Fatalf("unexpected link request body:\ngot  %#v\nwant %#v", gotBody, want)
	}

	var got res.ModelVersionResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state diagnostics: %v", resp.Diagnostics)
	}
	if got.Version.ValueInt64() != 0 {
		t.Errorf("expected the server assigned version 0, got %d", got.Version.ValueInt64())
	}
	if got.ID.ValueString() != "ml.cat.sch.model1.0" {
		t.Errorf("expected id ml.cat.sch.model1.0, got %s", got.ID.ValueString())
	}
	if got.URI.ValueString() != "hdfs://path/to/model" {
		t.Errorf("expected the uri in state, got %q", got.URI.ValueString())
	}
	if len(got.Aliases.Elements()) != 2 {
		t.Errorf("expected 2 aliases in state, got %v", got.Aliases)
	}
	if got.Comment.ValueString() != "This is a comment" {
		t.Errorf("unexpected comment in state: %q", got.Comment.ValueString())
	}
}

func TestModelVersionResource_Create_WithURIs(t *testing.T) {
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			_, _ = w.Write([]byte(`{"code": 0}`))
			return
		}
		// Shape returned by a real Gravitino 1.3.0 after linking a version with a uris map.
		_, _ = w.Write([]byte(`{
  "code": 0,
  "infos": [
    {
      "uris": {"s3": "s3://path/to/model", "gcs": "gs://path/to/model"},
      "version": 0,
      "aliases": [],
      "comment": "This is a comment",
      "properties": {},
      "audit": {
        "creator": "user1",
        "createTime": "2021-01-01T00:00:00Z"
      }
    }
  ]
}`))
	}))
	defer server.Close()

	r, _ := newModelVersionResource(server)
	ctx := context.Background()
	sch := modelVersionSchema(t, r)

	plan := res.ModelVersionResourceModel{
		Metalake: types.StringValue("ml"),
		Catalog:  types.StringValue("cat"),
		Schema:   types.StringValue("sch"),
		Model:    types.StringValue("model1"),
		Version:  types.Int64Unknown(),
		URI:      types.StringNull(),
		URIs: types.MapValueMust(types.StringType, map[string]attr.Value{
			"hdfs": types.StringValue("hdfs://path/to/model"),
			"s3":   types.StringValue("s3://path/to/model"),
		}),
		Aliases:    types.SetNull(types.StringType),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(ctx, resource.CreateRequest{Plan: modelVersionPlan(t, sch, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	want := map[string]interface{}{
		"uris": map[string]interface{}{
			"hdfs": "hdfs://path/to/model",
			"s3":   "s3://path/to/model",
		},
	}
	if !reflect.DeepEqual(gotBody, want) {
		t.Fatalf("unexpected link request body:\ngot  %#v\nwant %#v", gotBody, want)
	}

	var got res.ModelVersionResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.URIs.Elements()["s3"].(types.String).ValueString() != "s3://path/to/model" {
		t.Errorf("expected the s3 uri in state, got %v", got.URIs)
	}
	if !got.URI.IsNull() {
		t.Errorf("expected no unnamed uri in state, got %q", got.URI.ValueString())
	}
}

// TestModelVersionResource_Create_VersionNumbersOnly covers a server that
// answers the details list with version numbers: the newest version has to be
// fetched individually.
func TestModelVersionResource_Create_VersionNumbersOnly(t *testing.T) {
	var gotRequests []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequests = append(gotRequests, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"code": 0}`))
		case r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions":
			// ModelVersionListResponse example of the spec.
			_, _ = w.Write([]byte(modelVersionListResponseExample))
		default:
			_, _ = w.Write([]byte(modelVersionResponseFor(2)))
		}
	}))
	defer server.Close()

	r, _ := newModelVersionResource(server)
	ctx := context.Background()
	sch := modelVersionSchema(t, r)

	plan := res.ModelVersionResourceModel{
		Metalake:   types.StringValue("ml"),
		Catalog:    types.StringValue("cat"),
		Schema:     types.StringValue("sch"),
		Model:      types.StringValue("model1"),
		Version:    types.Int64Unknown(),
		URI:        types.StringValue("hdfs://path/to/model"),
		URIs:       types.MapUnknown(types.StringType),
		Aliases:    types.SetNull(types.StringType),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(ctx, resource.CreateRequest{Plan: modelVersionPlan(t, sch, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	wantRequests := []string{
		"POST /api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions",
		"GET /api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions?details=true",
		"GET /api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions/2",
	}
	if !reflect.DeepEqual(gotRequests, wantRequests) {
		t.Fatalf("unexpected requests:\ngot  %v\nwant %v", gotRequests, wantRequests)
	}

	var got res.ModelVersionResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Version.ValueInt64() != 2 {
		t.Errorf("expected the newest version 2, got %d", got.Version.ValueInt64())
	}
	if got.ID.ValueString() != "ml.cat.sch.model1.2" {
		t.Errorf("unexpected id: %s", got.ID.ValueString())
	}
}

func TestModelVersionResource_Create_WithoutURI(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code": 0}`))
	}))
	defer server.Close()

	r, _ := newModelVersionResource(server)
	ctx := context.Background()
	sch := modelVersionSchema(t, r)

	plan := res.ModelVersionResourceModel{
		Metalake:   types.StringValue("ml"),
		Catalog:    types.StringValue("cat"),
		Schema:     types.StringValue("sch"),
		Model:      types.StringValue("model1"),
		Version:    types.Int64Unknown(),
		URI:        types.StringNull(),
		URIs:       types.MapNull(types.StringType),
		Aliases:    types.SetNull(types.StringType),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(ctx, resource.CreateRequest{Plan: modelVersionPlan(t, sch, plan)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when neither uri nor uris is configured")
	}
	if requests != 0 {
		t.Fatal("Gravitino requires an artifact location, so no request must be sent")
	}
}

// TestModelVersionResource_Create_NoVersionReported covers a server that does
// not report the linked version: the create must fail loudly instead of leaving
// unknown values in state.
func TestModelVersionResource_Create_NoVersionReported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"code": 0}`))
			return
		}
		_, _ = w.Write([]byte(`{"code": 0, "infos": []}`))
	}))
	defer server.Close()

	r, _ := newModelVersionResource(server)
	ctx := context.Background()
	sch := modelVersionSchema(t, r)

	plan := res.ModelVersionResourceModel{
		Metalake:   types.StringValue("ml"),
		Catalog:    types.StringValue("cat"),
		Schema:     types.StringValue("sch"),
		Model:      types.StringValue("model1"),
		Version:    types.Int64Unknown(),
		URI:        types.StringValue("hdfs://path/to/model"),
		URIs:       types.MapUnknown(types.StringType),
		Aliases:    types.SetNull(types.StringType),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(ctx, resource.CreateRequest{Plan: modelVersionPlan(t, sch, plan)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when Gravitino reports no linked model version")
	}
}

func TestModelVersionResource_Update_Payload(t *testing.T) {
	var gotUpdates []interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Updates []interface{} `json:"updates"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		gotUpdates = body.Updates
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{
  "code": 0,
  "modelVersion": {
    "uri": "s3://path/to/model",
    "uris": {"hdfs": "hdfs://path/to/model/updated", "gcs": "gs://path/to/model"},
    "version": 0,
    "aliases": ["alias2", "alias3"],
    "comment": "This is a new comment",
    "properties": {"key1": "value1", "key2": "value2"},
    "audit": {
      "creator": "user1",
      "createTime": "2021-01-01T00:00:00Z",
      "lastModifier": "user1",
      "lastModifiedTime": "2021-01-02T00:00:00Z"
    }
  }
}`))
	}))
	defer server.Close()

	r, _ := newModelVersionResource(server)
	ctx := context.Background()
	sch := modelVersionSchema(t, r)

	state := res.ModelVersionResourceModel{
		ID:       types.StringValue("ml.cat.sch.model1.0"),
		Metalake: types.StringValue("ml"),
		Catalog:  types.StringValue("cat"),
		Schema:   types.StringValue("sch"),
		Model:    types.StringValue("model1"),
		Version:  types.Int64Value(0),
		URI:      types.StringValue("hdfs://path/to/model"),
		URIs: types.MapValueMust(types.StringType, map[string]attr.Value{
			"hdfs": types.StringValue("hdfs://path/to/model"),
			"s3":   types.StringValue("s3://path/to/model"),
		}),
		Aliases: types.SetValueMust(types.StringType, []attr.Value{
			types.StringValue("alias1"),
			types.StringValue("alias2"),
		}),
		Comment: types.StringValue("This is a comment"),
		Properties: types.MapValueMust(types.StringType, map[string]attr.Value{
			"key1": types.StringValue("value1"),
		}),
		Audit: types.ObjectNull(res.AuditAttrTypes),
	}

	plan := state
	plan.URI = types.StringValue("s3://path/to/model")
	plan.URIs = types.MapValueMust(types.StringType, map[string]attr.Value{
		"hdfs": types.StringValue("hdfs://path/to/model/updated"),
		"gcs":  types.StringValue("gs://path/to/model"),
	})
	plan.Aliases = types.SetValueMust(types.StringType, []attr.Value{
		types.StringValue("alias2"),
		types.StringValue("alias3"),
	})
	plan.Comment = types.StringValue("This is a new comment")
	plan.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{
		"key1": types.StringValue("value1"),
		"key2": types.StringValue("value2"),
	})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	r.Update(ctx, resource.UpdateRequest{Plan: modelVersionPlan(t, sch, plan), State: modelVersionState(t, sch, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	// The update types of the spec: updateComment, setProperty, updateAliases,
	// updateUri (without uriName for the unnamed uri), removeUri, addUri and
	// updateUri (with uriName).
	want := []interface{}{
		map[string]interface{}{"@type": "updateComment", "newComment": "This is a new comment"},
		map[string]interface{}{"@type": "setProperty", "property": "key2", "value": "value2"},
		map[string]interface{}{"@type": "updateAliases", "aliasesToAdd": []interface{}{"alias3"}, "aliasesToRemove": []interface{}{"alias1"}},
		map[string]interface{}{"@type": "updateUri", "newUri": "s3://path/to/model"},
		map[string]interface{}{"@type": "removeUri", "uriName": "s3"},
		map[string]interface{}{"@type": "addUri", "uriName": "gcs", "uri": "gs://path/to/model"},
		map[string]interface{}{"@type": "updateUri", "newUri": "hdfs://path/to/model/updated", "uriName": "hdfs"},
	}
	if !reflect.DeepEqual(gotUpdates, want) {
		t.Fatalf("unexpected update payload:\ngot  %#v\nwant %#v", gotUpdates, want)
	}

	var got res.ModelVersionResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.URI.ValueString() != "s3://path/to/model" {
		t.Errorf("unexpected uri in state: %q", got.URI.ValueString())
	}
	if len(got.URIs.Elements()) != 2 {
		t.Errorf("unexpected uris in state: %v", got.URIs)
	}
	if len(got.Aliases.Elements()) != 2 {
		t.Errorf("unexpected aliases in state: %v", got.Aliases)
	}
	if got.Comment.ValueString() != "This is a new comment" {
		t.Errorf("unexpected comment in state: %q", got.Comment.ValueString())
	}
}

func TestModelVersionResource_Update_UnchangedCommentKeepsNoUpdate(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method == http.MethodPut {
			t.Error("no update request must be sent when nothing changed for Gravitino")
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(modelVersionResponseExample))
	}))
	defer server.Close()

	r, _ := newModelVersionResource(server)
	ctx := context.Background()
	sch := modelVersionSchema(t, r)

	state := res.ModelVersionResourceModel{
		ID:       types.StringValue("ml.cat.sch.model1.0"),
		Metalake: types.StringValue("ml"),
		Catalog:  types.StringValue("cat"),
		Schema:   types.StringValue("sch"),
		Model:    types.StringValue("model1"),
		Version:  types.Int64Value(0),
		URI:      types.StringValue("hdfs://path/to/model"),
		// Only a removed unnamed uri is a change Gravitino cannot express.
		URIs:       types.MapUnknown(types.StringType),
		Aliases:    types.SetUnknown(types.StringType),
		Comment:    types.StringUnknown(),
		Properties: types.MapUnknown(types.StringType),
		Audit:      types.ObjectNull(res.AuditAttrTypes),
	}

	plan := state
	plan.URI = types.StringNull()

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	r.Update(ctx, resource.UpdateRequest{Plan: modelVersionPlan(t, sch, plan), State: modelVersionState(t, sch, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if requests == 0 {
		t.Fatal("the model version must be read back when no update is sent")
	}

	var got res.ModelVersionResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.URI.IsUnknown() {
		t.Fatal("uri must be known after apply")
	}
	if len(got.Aliases.Elements()) != 2 {
		t.Errorf("aliases must be filled from the API response, got %v", got.Aliases)
	}
}

func TestModelVersionResource_Delete(t *testing.T) {
	var deleteCalled bool
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.Method == http.MethodDelete {
			deleteCalled = true
			gotPath = r.URL.Path
		}
		_, _ = w.Write([]byte(`{"code": 0, "dropped": true}`))
	}))
	defer server.Close()

	r, _ := newModelVersionResource(server)
	ctx := context.Background()
	sch := modelVersionSchema(t, r)

	state := res.ModelVersionResourceModel{
		ID:         types.StringValue("ml.cat.sch.model1.0"),
		Metalake:   types.StringValue("ml"),
		Catalog:    types.StringValue("cat"),
		Schema:     types.StringValue("sch"),
		Model:      types.StringValue("model1"),
		Version:    types.Int64Value(0),
		URI:        types.StringValue("hdfs://path/to/model"),
		URIs:       types.MapNull(types.StringType),
		Aliases:    types.SetNull(types.StringType),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: sch}}
	r.Delete(ctx, resource.DeleteRequest{State: modelVersionState(t, sch, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !deleteCalled {
		t.Fatal("delete was not called")
	}
	// The version is a path parameter, not a query parameter.
	if gotPath != "/api/metalakes/ml/catalogs/cat/schemas/sch/models/model1/versions/0" {
		t.Fatalf("unexpected delete path: %s", gotPath)
	}
}

func TestModelVersionResource_Delete_NotFoundIsIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{
  "code": 1003,
  "type": "NoSuchModelException",
  "message": "Model does not exist",
  "stack": ["org.apache.gravitino.exceptions.NoSuchModelException: Model does not exist"]
}`))
	}))
	defer server.Close()

	r, _ := newModelVersionResource(server)
	ctx := context.Background()
	sch := modelVersionSchema(t, r)

	state := res.ModelVersionResourceModel{
		ID:         types.StringValue("ml.cat.sch.model1.0"),
		Metalake:   types.StringValue("ml"),
		Catalog:    types.StringValue("cat"),
		Schema:     types.StringValue("sch"),
		Model:      types.StringValue("model1"),
		Version:    types.Int64Value(0),
		URI:        types.StringValue("hdfs://path/to/model"),
		URIs:       types.MapNull(types.StringType),
		Aliases:    types.SetNull(types.StringType),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: sch}}
	r.Delete(ctx, resource.DeleteRequest{State: modelVersionState(t, sch, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 on delete must be treated as success, got: %v", resp.Diagnostics)
	}
}

func TestModelVersionResource_Read_NotFoundRemovesState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(noSuchModelVersionExample))
	}))
	defer server.Close()

	r, _ := newModelVersionResource(server)
	ctx := context.Background()
	sch := modelVersionSchema(t, r)

	state := res.ModelVersionResourceModel{
		ID:         types.StringValue("ml.cat.sch.model1.0"),
		Metalake:   types.StringValue("ml"),
		Catalog:    types.StringValue("cat"),
		Schema:     types.StringValue("sch"),
		Model:      types.StringValue("model1"),
		Version:    types.Int64Value(0),
		URI:        types.StringValue("hdfs://path/to/model"),
		URIs:       types.MapNull(types.StringType),
		Aliases:    types.SetNull(types.StringType),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.ReadResponse{State: modelVersionState(t, sch, state)}
	r.Read(ctx, resource.ReadRequest{State: modelVersionState(t, sch, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 on read must not produce an error, got: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("a 404 on read must remove the model version from state")
	}
}

func TestModelVersionResource_ImportState(t *testing.T) {
	r, _ := newModelVersionResource(nil)
	ctx := context.Background()
	sch := modelVersionSchema(t, r)

	resp := &resource.ImportStateResponse{State: modelVersionState(t, sch, nullModelVersionState())}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "my_metalake.my_catalog.my_schema.my_model.3"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got res.ModelVersionResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Version.ValueInt64() != 3 {
		t.Errorf("expected version 3, got %d", got.Version.ValueInt64())
	}
	if got.ID.ValueString() != "my_metalake.my_catalog.my_schema.my_model.3" {
		t.Errorf("unexpected id: %s", got.ID.ValueString())
	}
}

func TestModelVersionResource_ImportState_Invalid(t *testing.T) {
	r, _ := newModelVersionResource(nil)
	ctx := context.Background()
	sch := modelVersionSchema(t, r)

	for _, id := range []string{"too.few.parts", "ml.cat.sch.model1", "ml.cat.sch.model1.notanumber", "ml.cat.sch.model1.", "ml.cat.sch..0"} {
		resp := &resource.ImportStateResponse{State: modelVersionState(t, sch, nullModelVersionState())}
		r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected an error for import ID %q", id)
		}
	}
}

func nullModelVersionState() res.ModelVersionResourceModel {
	return res.ModelVersionResourceModel{
		ID:         types.StringNull(),
		Metalake:   types.StringNull(),
		Catalog:    types.StringNull(),
		Schema:     types.StringNull(),
		Model:      types.StringNull(),
		Version:    types.Int64Null(),
		URI:        types.StringNull(),
		URIs:       types.MapNull(types.StringType),
		Aliases:    types.SetNull(types.StringType),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(res.AuditAttrTypes),
	}
}
