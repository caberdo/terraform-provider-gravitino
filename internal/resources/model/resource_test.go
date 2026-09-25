package model_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/model"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// modelResponseExample is the `ModelResponse` example of the Gravitino v1.3.0
// OpenAPI spec (docs/open-api/models.yaml), served verbatim by the mocks.
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
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchModelException: Model does not exist"
  ]
}`

func modelSchema(t *testing.T, r resource.Resource) schema.Schema {
	t.Helper()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	return schemaResp.Schema
}

func modelPlan(t *testing.T, sch schema.Schema, m res.ModelResourceModel) tfsdk.Plan {
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

func modelState(t *testing.T, sch schema.Schema, m res.ModelResourceModel) tfsdk.State {
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

func newModelResource(server *httptest.Server) (resource.Resource, *res.ModelResource) {
	uri := ""
	if server != nil {
		uri = server.URL
	}
	c, _ := client.New(uri, nil)
	r := res.New()
	concrete := r.(*res.ModelResource)
	concrete.SetClient(c)
	return r, concrete
}

func TestModelResource_Schema(t *testing.T) {
	r, _ := newModelResource(nil)
	metadataResp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, metadataResp)
	if metadataResp.TypeName != "gravitino_model" {
		t.Fatalf("expected gravitino_model, got %s", metadataResp.TypeName)
	}

	sch := modelSchema(t, r)
	for _, name := range []string{"id", "metalake", "catalog", "schema", "name", "comment", "latest_version", "properties", "audit"} {
		if _, ok := sch.Attributes[name]; !ok {
			t.Errorf("expected attribute %q in the schema", name)
		}
	}
	// A Gravitino model has no URI: only its versions carry artifact locations.
	if _, ok := sch.Attributes["model_uri"]; ok {
		t.Error("model_uri must not exist on gravitino_model: Model has no modelUri field in the v1.3.0 spec")
	}
	if got := sch.Attributes["latest_version"].GetType(); got != types.Int64Type {
		t.Errorf("latest_version must be an int64, got %s", got)
	}
	if !sch.Attributes["latest_version"].IsComputed() {
		t.Error("latest_version must be computed")
	}
	if !sch.Attributes["id"].IsComputed() {
		t.Error("id must be computed")
	}
	// Gravitino has no operation to move a model between parents.
	for _, name := range []string{"metalake", "catalog", "schema"} {
		if !sch.Attributes[name].IsRequired() {
			t.Errorf("%s must be required", name)
		}
	}
}

func TestModelResource_Create_SpecExample(t *testing.T) {
	var gotBody map[string]interface{}
	var gotMethod, gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(modelResponseExample))
	}))
	defer server.Close()

	r, _ := newModelResource(server)
	ctx := context.Background()
	sch := modelSchema(t, r)

	plan := res.ModelResourceModel{
		Metalake:      types.StringValue("ml"),
		Catalog:       types.StringValue("cat"),
		Schema:        types.StringValue("sch"),
		Name:          types.StringValue("model1"),
		Comment:       types.StringValue("This is a comment"),
		LatestVersion: types.Int64Unknown(),
		Properties: types.MapValueMust(types.StringType, map[string]attr.Value{
			"key1": types.StringValue("value1"),
			"key2": types.StringValue("value2"),
		}),
		Audit: types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(ctx, resource.CreateRequest{Plan: modelPlan(t, sch, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if gotMethod != http.MethodPost || gotPath != "/api/metalakes/ml/catalogs/cat/schemas/sch/models" {
		t.Fatalf("expected POST on the models collection, got %s %s", gotMethod, gotPath)
	}
	// ModelRegisterRequest example of the spec.
	want := map[string]interface{}{
		"name":       "model1",
		"comment":    "This is a comment",
		"properties": map[string]interface{}{"key1": "value1", "key2": "value2"},
	}
	if !reflect.DeepEqual(gotBody, want) {
		t.Fatalf("unexpected register request body:\ngot  %#v\nwant %#v", gotBody, want)
	}

	var got res.ModelResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state diagnostics: %v", resp.Diagnostics)
	}
	if got.ID.ValueString() != "ml.cat.sch.model1" {
		t.Errorf("expected id ml.cat.sch.model1, got %s", got.ID.ValueString())
	}
	if got.LatestVersion.ValueInt64() != 0 {
		t.Errorf("expected latest_version 0, got %d", got.LatestVersion.ValueInt64())
	}
	if got.Comment.ValueString() != "This is a comment" {
		t.Errorf("expected the model comment in state, got %q", got.Comment.ValueString())
	}
	if got.Audit.IsNull() {
		t.Fatal("expected the audit in state")
	}
	if creator := got.Audit.Attributes()["creator"].(types.String).ValueString(); creator != "user1" {
		t.Errorf("expected audit creator user1, got %q", creator)
	}
}

func TestModelResource_Update_Payload(t *testing.T) {
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
  "model": {
    "name": "my_model_new",
    "latestVersion": 0,
    "comment": "This is a new comment",
    "properties": {"key2": "value2", "key3": "value3"},
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

	r, _ := newModelResource(server)
	ctx := context.Background()
	sch := modelSchema(t, r)

	state := res.ModelResourceModel{
		ID:            types.StringValue("ml.cat.sch.model1"),
		Metalake:      types.StringValue("ml"),
		Catalog:       types.StringValue("cat"),
		Schema:        types.StringValue("sch"),
		Name:          types.StringValue("model1"),
		Comment:       types.StringValue("This is a comment"),
		LatestVersion: types.Int64Value(0),
		Properties: types.MapValueMust(types.StringType, map[string]attr.Value{
			"key1": types.StringValue("value1"),
			"key2": types.StringValue("value2"),
		}),
		Audit: types.ObjectNull(res.AuditAttrTypes),
	}
	plan := state
	plan.Name = types.StringValue("my_model_new")
	plan.Comment = types.StringValue("This is a new comment")
	plan.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{
		"key2": types.StringValue("value2"),
		"key3": types.StringValue("value3"),
	})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	r.Update(ctx, resource.UpdateRequest{Plan: modelPlan(t, sch, plan), State: modelState(t, sch, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	// The update types of the spec: rename, updateComment, removeProperty and setProperty.
	want := []interface{}{
		map[string]interface{}{"@type": "rename", "newName": "my_model_new"},
		map[string]interface{}{"@type": "updateComment", "newComment": "This is a new comment"},
		map[string]interface{}{"@type": "removeProperty", "property": "key1"},
		map[string]interface{}{"@type": "setProperty", "property": "key3", "value": "value3"},
	}
	if !reflect.DeepEqual(gotUpdates, want) {
		t.Fatalf("unexpected update payload:\ngot  %#v\nwant %#v", gotUpdates, want)
	}

	var got res.ModelResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Name.ValueString() != "my_model_new" {
		t.Errorf("expected the renamed model in state, got %s", got.Name.ValueString())
	}
	if got.ID.ValueString() != "ml.cat.sch.my_model_new" {
		t.Errorf("expected the renamed id in state, got %s", got.ID.ValueString())
	}
	if got.Comment.ValueString() != "This is a new comment" {
		t.Errorf("unexpected comment in state: %q", got.Comment.ValueString())
	}
}

func TestModelResource_Delete(t *testing.T) {
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

	r, _ := newModelResource(server)
	ctx := context.Background()
	sch := modelSchema(t, r)

	state := res.ModelResourceModel{
		ID:            types.StringValue("ml.cat.sch.model1"),
		Metalake:      types.StringValue("ml"),
		Catalog:       types.StringValue("cat"),
		Schema:        types.StringValue("sch"),
		Name:          types.StringValue("model1"),
		LatestVersion: types.Int64Value(0),
		Comment:       types.StringNull(),
		Properties:    types.MapNull(types.StringType),
		Audit:         types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: sch}}
	r.Delete(ctx, resource.DeleteRequest{State: modelState(t, sch, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !deleteCalled {
		t.Fatal("delete was not called")
	}
	if gotPath != "/api/metalakes/ml/catalogs/cat/schemas/sch/models/model1" {
		t.Fatalf("unexpected delete path: %s", gotPath)
	}
}

func TestModelResource_Delete_NotFoundIsIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(noSuchModelExample))
	}))
	defer server.Close()

	r, _ := newModelResource(server)
	ctx := context.Background()
	sch := modelSchema(t, r)

	state := res.ModelResourceModel{
		ID:            types.StringValue("ml.cat.sch.model1"),
		Metalake:      types.StringValue("ml"),
		Catalog:       types.StringValue("cat"),
		Schema:        types.StringValue("sch"),
		Name:          types.StringValue("model1"),
		LatestVersion: types.Int64Value(0),
		Comment:       types.StringNull(),
		Properties:    types.MapNull(types.StringType),
		Audit:         types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: sch}}
	r.Delete(ctx, resource.DeleteRequest{State: modelState(t, sch, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 on delete must be treated as success, got: %v", resp.Diagnostics)
	}
}

func TestModelResource_Read_NotFoundRemovesState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(noSuchModelExample))
	}))
	defer server.Close()

	r, _ := newModelResource(server)
	ctx := context.Background()
	sch := modelSchema(t, r)

	state := res.ModelResourceModel{
		ID:            types.StringValue("ml.cat.sch.model1"),
		Metalake:      types.StringValue("ml"),
		Catalog:       types.StringValue("cat"),
		Schema:        types.StringValue("sch"),
		Name:          types.StringValue("model1"),
		LatestVersion: types.Int64Value(0),
		Comment:       types.StringNull(),
		Properties:    types.MapNull(types.StringType),
		Audit:         types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.ReadResponse{State: modelState(t, sch, state)}
	r.Read(ctx, resource.ReadRequest{State: modelState(t, sch, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 on read must not produce an error, got: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("a 404 on read must remove the model from state")
	}
}

func TestModelResource_Read_SpecExample(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metalakes/ml/catalogs/cat/schemas/sch/models/model1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(modelResponseExample))
	}))
	defer server.Close()

	r, _ := newModelResource(server)
	ctx := context.Background()
	sch := modelSchema(t, r)

	state := res.ModelResourceModel{
		ID:            types.StringValue("ml.cat.sch.model1"),
		Metalake:      types.StringValue("ml"),
		Catalog:       types.StringValue("cat"),
		Schema:        types.StringValue("sch"),
		Name:          types.StringValue("model1"),
		LatestVersion: types.Int64Value(7),
		Comment:       types.StringNull(),
		Properties:    types.MapNull(types.StringType),
		Audit:         types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.ReadResponse{State: modelState(t, sch, state)}
	r.Read(ctx, resource.ReadRequest{State: modelState(t, sch, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got res.ModelResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state diagnostics: %v", resp.Diagnostics)
	}
	if got.LatestVersion.ValueInt64() != 0 {
		t.Errorf("expected latest_version from the API (0), got %d", got.LatestVersion.ValueInt64())
	}
	if got.Comment.ValueString() != "This is a comment" {
		t.Errorf("unexpected comment: %q", got.Comment.ValueString())
	}
	if got.Properties.Elements()["key1"].(types.String).ValueString() != "value1" {
		t.Errorf("unexpected properties: %v", got.Properties)
	}
}

func TestModelResource_ImportState(t *testing.T) {
	r, _ := newModelResource(nil)
	ctx := context.Background()
	sch := modelSchema(t, r)

	nullState := res.ModelResourceModel{
		ID:            types.StringNull(),
		Metalake:      types.StringNull(),
		Catalog:       types.StringNull(),
		Schema:        types.StringNull(),
		Name:          types.StringNull(),
		Comment:       types.StringNull(),
		LatestVersion: types.Int64Null(),
		Properties:    types.MapNull(types.StringType),
		Audit:         types.ObjectNull(res.AuditAttrTypes),
	}

	resp := &resource.ImportStateResponse{State: modelState(t, sch, nullState)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "my_metalake.my_catalog.my_schema.my_model"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got res.ModelResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Metalake.ValueString() != "my_metalake" || got.Name.ValueString() != "my_model" || got.ID.ValueString() != "my_metalake.my_catalog.my_schema.my_model" {
		t.Fatalf("unexpected imported state: %v", got)
	}
}

func TestModelResource_ImportState_Invalid(t *testing.T) {
	r, _ := newModelResource(nil)
	ctx := context.Background()
	sch := modelSchema(t, r)

	nullState := res.ModelResourceModel{
		ID:            types.StringNull(),
		Metalake:      types.StringNull(),
		Catalog:       types.StringNull(),
		Schema:        types.StringNull(),
		Name:          types.StringNull(),
		Comment:       types.StringNull(),
		LatestVersion: types.Int64Null(),
		Properties:    types.MapNull(types.StringType),
		Audit:         types.ObjectNull(res.AuditAttrTypes),
	}

	// The leaf may contain dots (the name is the last segment), so only missing
	// or empty segments are invalid.
	for _, id := range []string{"too.few.parts", "ml.cat.sch", ".cat.sch.name", "ml.cat.sch."} {
		resp := &resource.ImportStateResponse{State: modelState(t, sch, nullState)}
		r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected an error for import ID %q", id)
		}
	}
}
