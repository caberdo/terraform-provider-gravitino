package topic_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/topic"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Payloads below are copied literally from the Gravitino v1.3.0 OpenAPI spec
// (docs/open-api/topics.yaml).

const (
	topicBasePath       = "/api/metalakes/test_metalake/catalogs/test_catalog/schemas/test_schema/topics"
	topicItemPath       = topicBasePath + "/topic1"
	topicJSONMediaType  = "application/vnd.gravitino.v1+json"
	specTopicCollection = `{
  "name": "topic1",
  "comment": "This is a topic",
  "properties": {
    "partition-count": "1",
    "replication-factor": "1"
  }
}`

	// components/examples/TopicResponse
	specTopicResponse = `{
  "code": 0,
  "topic": {
    "name": "topic1",
    "comment": "This is a topic",
    "properties": {
      "partition-count": "1",
      "replication-factor": "1"
    }
  }
}`

	// components/examples/NoSuchTopicException
	specNoSuchTopicException = `{
  "code": 1003,
  "type": "NoSuchTopicException",
  "message": "Topic does not exist",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchTopicException: Topic does not exist",
    "..."
  ]
}`

	// components/responses/DropResponse (the real 1.3.0 server answers DELETE with this body).
	specDropResponse = `{"code": 0, "dropped": true}`
)

// topicModel mirrors a topic created from the spec's TopicCreateRequest example.
func topicModel(name string, comment types.String, properties types.Map, audit types.Object) res.TopicResourceModel {
	return res.TopicResourceModel{
		ID:         types.StringValue("test_metalake.test_catalog.test_schema." + name),
		Metalake:   types.StringValue("test_metalake"),
		Catalog:    types.StringValue("test_catalog"),
		Schema:     types.StringValue("test_schema"),
		Name:       types.StringValue(name),
		Comment:    comment,
		Properties: properties,
		Audit:      audit,
	}
}

func topicSchema(t *testing.T) schema.Schema {
	t.Helper()
	r := res.NewTopicResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func topicValue(t *testing.T, s schema.Schema, model res.TopicResourceModel) tftypes.Value {
	t.Helper()
	ctx := context.Background()
	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("failed to build object: %v", diags)
	}
	v, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}
	return v
}

// requestRecorder captures the requests the resource sends to the mock server.
type requestRecorder struct {
	mu       sync.Mutex
	requests []recordedRequest
}

type recordedRequest struct {
	Method string
	Path   string
	Body   map[string]interface{}
}

func (rec *requestRecorder) record(r *http.Request) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var body map[string]interface{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	rec.requests = append(rec.requests, recordedRequest{Method: r.Method, Path: r.URL.Path, Body: body})
}

func (rec *requestRecorder) only(t *testing.T, method string) recordedRequest {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var found []recordedRequest
	for _, req := range rec.requests {
		if req.Method == method {
			found = append(found, req)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly one %s request, got %d (%v)", method, len(found), rec.requests)
	}
	return found[0]
}

func (rec *requestRecorder) count(method string) int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	n := 0
	for _, req := range rec.requests {
		if req.Method == method {
			n++
		}
	}
	return n
}

func specTopicResource(t *testing.T, rec *requestRecorder, handler func(w http.ResponseWriter, r *http.Request)) (*res.TopicResource, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", topicJSONMediaType)
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	r := res.NewTopicResource()
	r.(*res.TopicResource).SetClient(c)
	return r.(*res.TopicResource), server
}

func TestTopicResource_Schema(t *testing.T) {
	r := res.NewTopicResource()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.TODO(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_topic" {
		t.Fatalf("expected gravitino_topic, got %s", resp.TypeName)
	}

	s := topicSchema(t)
	for _, name := range []string{"id", "metalake", "catalog", "schema", "name", "comment", "properties", "audit"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if _, ok := s.Attributes["rename"]; ok {
		t.Error("topic schema must not expose a rename attribute")
	}
}

// The create request must be exactly the spec's TopicCreateRequest example: the
// real 1.3.0 server rejects unknown fields (UnrecognizedPropertyException).
func TestTopicResource_Create_SendsSpecPayload(t *testing.T) {
	rec := &requestRecorder{}
	r, _ := specTopicResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == topicBasePath:
			io.WriteString(w, specTopicResponse)
		default:
			http.NotFound(w, req)
		}
	})

	s := topicSchema(t)
	ctx := context.Background()
	plan := topicModel("topic1", types.StringValue("This is a topic"), stringMap(t, map[string]string{
		"partition-count":    "1",
		"replication-factor": "1",
	}), types.ObjectUnknown(res.AuditAttrTypes))

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: topicValue(t, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	sent := rec.only(t, http.MethodPost)
	if sent.Path != topicBasePath {
		t.Errorf("create path = %q, want %q", sent.Path, topicBasePath)
	}

	var expected map[string]interface{}
	if err := json.Unmarshal([]byte(specTopicCollection), &expected); err != nil {
		t.Fatalf("spec example: %v", err)
	}
	if !reflect.DeepEqual(sent.Body, expected) {
		t.Errorf("create body = %#v, want the spec's TopicCreateRequest example %#v", sent.Body, expected)
	}

	var got res.TopicResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.ID.ValueString() != "test_metalake.test_catalog.test_schema.topic1" {
		t.Errorf("id = %q", got.ID.ValueString())
	}
	if got.Comment.ValueString() != "This is a topic" {
		t.Errorf("comment = %q", got.Comment.ValueString())
	}
	if got.Properties.IsNull() {
		t.Error("properties must be set from the server response")
	}
	// The spec declares no audit field for a topic: the attribute must still end
	// up known (null here) so Terraform never sees an unknown value.
	if got.Audit.IsUnknown() {
		t.Error("audit must never be unknown in state")
	}
	if !got.Audit.IsNull() {
		t.Errorf("audit = %v, want null when the server reports no audit", got.Audit)
	}
}

// A server that does report audit (the 1.3.0 response DTOs carry one) must have
// its value stored.
func TestTopicResource_Read_StoresAudit(t *testing.T) {
	const responseWithAudit = `{
  "code": 0,
  "topic": {
    "name": "topic1",
    "comment": "This is a topic",
    "audit": {
      "creator": "gravitino",
      "createTime": "2023-12-08T03:41:25.595Z"
    }
  }
}`

	rec := &requestRecorder{}
	r, _ := specTopicResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, responseWithAudit)
	})

	s := topicSchema(t)
	ctx := context.Background()
	state := topicModel("topic1", types.StringValue("This is a topic"), types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes))

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: topicValue(t, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: topicValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got res.TopicResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Audit.IsNull() || got.Audit.IsUnknown() {
		t.Fatalf("audit = %v, want the server value", got.Audit)
	}
	auditVals := got.Audit.Attributes()
	if creator, ok := auditVals["creator"].(types.String); !ok || creator.ValueString() != "gravitino" {
		t.Errorf("audit.creator = %v, want gravitino", auditVals["creator"])
	}
	if createTime, ok := auditVals["create_time"].(types.String); !ok || createTime.ValueString() != "2023-12-08T03:41:25Z" {
		t.Errorf("audit.create_time = %v, want the 2023-12-08T03:41:25.595Z server value as RFC3339", auditVals["create_time"])
	}
}

// After ImportState the state has no comment/properties: Read must reconstruct
// them from the server so an imported topic matches its configuration.
func TestTopicResource_Read_AdoptsServerValues(t *testing.T) {
	rec := &requestRecorder{}
	r, _ := specTopicResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, specTopicResponse)
	})

	s := topicSchema(t)
	ctx := context.Background()
	state := topicModel("topic1", types.StringNull(), types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes))

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: topicValue(t, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: topicValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got res.TopicResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Comment.IsNull() || got.Comment.ValueString() != "This is a topic" {
		t.Errorf("comment = %v, want the server value", got.Comment)
	}
	if got.Properties.IsNull() || len(got.Properties.Elements()) != 2 {
		t.Errorf("properties = %v, want the two server properties after import", got.Properties)
	}
	if got.Audit.IsUnknown() {
		t.Error("audit must never be unknown in state")
	}
}

// A 404 from the real Gravitino API ({"code":1003,"type":"NoSuchTopicException",...})
// must remove the resource from state, not raise an error.
func TestTopicResource_Read_NotFoundRemovesState(t *testing.T) {
	rec := &requestRecorder{}
	r, _ := specTopicResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, specNoSuchTopicException)
	})

	s := topicSchema(t)
	ctx := context.Background()
	state := topicModel("topic1", types.StringNull(), types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes))
	raw := topicValue(t, s, state)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 read must not error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("404 read must remove the resource from state")
	}
}

// The effective update payload must only contain update types that exist in the
// spec: updateComment, setProperty and removeProperty.
func TestTopicResource_Update_SendsSpecPayload(t *testing.T) {
	// Mirrors the server state after applying the spec's UpdateTopicCommentRequest
	// example ({"@type": "updateComment", "newComment": "This is the new comment"}).
	const responseAfterUpdate = `{
  "code": 0,
  "topic": {
    "name": "topic1",
    "comment": "This is the new comment"
  }
}`

	rec := &requestRecorder{}
	r, _ := specTopicResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPut && req.URL.Path == topicItemPath {
			io.WriteString(w, responseAfterUpdate)
			return
		}
		http.NotFound(w, req)
	})

	s := topicSchema(t)
	ctx := context.Background()
	state := topicModel("topic1", types.StringValue("This is a topic"),
		stringMap(t, map[string]string{"key": "value"}),
		types.ObjectNull(res.AuditAttrTypes))
	plan := topicModel("topic1", types.StringValue("This is the new comment"),
		types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes))

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: topicValue(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: topicValue(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	sent := rec.only(t, http.MethodPut)
	if sent.Path != topicItemPath {
		t.Errorf("update path = %q, want %q", sent.Path, topicItemPath)
	}
	want := []map[string]interface{}{
		{"@type": "updateComment", "newComment": "This is the new comment"},
		{"@type": "removeProperty", "property": "key"},
	}
	assertTopicUpdates(t, sent.Body, want)

	var got res.TopicResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Comment.ValueString() != "This is the new comment" {
		t.Errorf("comment = %q", got.Comment.ValueString())
	}
	if !got.Properties.IsNull() {
		t.Errorf("properties = %v, want null after removing the only property", got.Properties)
	}
	// The plan's audit is unknown (no UseStateForUnknown); Update must resolve it
	// to a known value even when the server reports none.
	if got.Audit.IsUnknown() {
		t.Error("audit must never be unknown in state after an update")
	}
}

func TestTopicResource_Update_SetPropertyPayload(t *testing.T) {
	rec := &requestRecorder{}
	r, _ := specTopicResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, specTopicResponse)
	})

	s := topicSchema(t)
	ctx := context.Background()
	state := topicModel("topic1", types.StringValue("This is a topic"),
		stringMap(t, map[string]string{}), types.ObjectNull(res.AuditAttrTypes))
	plan := topicModel("topic1", types.StringValue("This is a topic"),
		stringMap(t, map[string]string{"key": "value"}), types.ObjectNull(res.AuditAttrTypes))

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: topicValue(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: topicValue(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	sent := rec.only(t, http.MethodPut)
	assertTopicUpdates(t, sent.Body, []map[string]interface{}{
		{"@type": "setProperty", "property": "key", "value": "value"},
	})
}

// Clearing a comment is only possible through updateComment with an empty
// newComment: the spec has no removeComment and no rename for topics.
func TestTopicResource_Update_ClearsCommentWithEmptyNewComment(t *testing.T) {
	rec := &requestRecorder{}
	r, _ := specTopicResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, `{"code":0,"topic":{"name":"topic1"}}`)
	})

	s := topicSchema(t)
	ctx := context.Background()
	state := topicModel("topic1", types.StringValue("This is a topic"), types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes))
	plan := topicModel("topic1", types.StringNull(), types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes))

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: topicValue(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: topicValue(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	sent := rec.only(t, http.MethodPut)
	assertTopicUpdates(t, sent.Body, []map[string]interface{}{
		{"@type": "updateComment", "newComment": ""},
	})

	var got res.TopicResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if !got.Comment.IsNull() {
		t.Errorf("comment = %v, want null after clearing", got.Comment)
	}
}

func TestTopicResource_Delete(t *testing.T) {
	rec := &requestRecorder{}
	r, _ := specTopicResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, specDropResponse)
	})

	s := topicSchema(t)
	ctx := context.Background()
	state := topicModel("topic1", types.StringValue("This is a topic"), types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes))

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: topicValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", resp.Diagnostics)
	}
	if rec.count(http.MethodDelete) != 1 {
		t.Fatal("delete was not called")
	}
}

func TestTopicResource_Delete_NotFoundIsSuccess(t *testing.T) {
	rec := &requestRecorder{}
	r, _ := specTopicResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, specNoSuchTopicException)
	})

	s := topicSchema(t)
	ctx := context.Background()
	state := topicModel("topic1", types.StringNull(), types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes))

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: topicValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 delete must be treated as success: %v", resp.Diagnostics)
	}
}

func TestTopicResource_ImportState(t *testing.T) {
	r := res.NewTopicResource().(resource.ResourceWithImportState)
	ctx := context.Background()
	s := topicSchema(t)

	nullModel := topicModel("", types.StringNull(), types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes))
	nullModel.ID = types.StringNull()
	nullModel.Metalake = types.StringNull()
	nullModel.Name = types.StringNull()
	nullModel.Catalog = types.StringNull()
	nullModel.Schema = types.StringNull()

	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: topicValue(t, s, nullModel)}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "my_metalake.my_catalog.my_schema.my_topic"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	var got res.TopicResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Metalake.ValueString() != "my_metalake" || got.Catalog.ValueString() != "my_catalog" ||
		got.Schema.ValueString() != "my_schema" || got.Name.ValueString() != "my_topic" {
		t.Errorf("imported state = %#v", got)
	}
	if got.ID.ValueString() != "my_metalake.my_catalog.my_schema.my_topic" {
		t.Errorf("imported id = %q", got.ID.ValueString())
	}
}

func TestTopicResource_ImportState_Invalid(t *testing.T) {
	for _, id := range []string{
		"no_dot_here",
		"my_metalake.my_catalog.my_schema",
		"my_metalake.my_catalog.my_schema.my_topic.extra",
		"my_metalake..my_schema.my_topic",
		"my_metalake.my_catalog.my_schema.",
		".my_catalog.my_schema.my_topic",
	} {
		t.Run(id, func(t *testing.T) {
			r := res.NewTopicResource().(resource.ResourceWithImportState)
			ctx := context.Background()
			s := topicSchema(t)

			nullModel := topicModel("", types.StringNull(), types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes))
			nullModel.ID = types.StringNull()
			nullModel.Metalake = types.StringNull()
			nullModel.Name = types.StringNull()
			nullModel.Catalog = types.StringNull()
			nullModel.Schema = types.StringNull()

			resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: topicValue(t, s, nullModel)}}
			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error for invalid import ID %q", id)
			}
		})
	}
}

// Gravitino topics cannot be moved or renamed: a changed parent or name must force
// replacement instead of being silently dropped by Update.
func TestTopicResource_ChangeRequiresReplace(t *testing.T) {
	s := topicSchema(t)
	for _, name := range []string{"metalake", "catalog", "schema", "name"} {
		attr, ok := s.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("attribute %q is not a string attribute", name)
		}
		if !topicStringRequiresReplace(t, attr, "old", "new") {
			t.Errorf("%s: changing the value must require replacement", name)
		}
		if topicStringRequiresReplace(t, attr, "same", "same") {
			t.Errorf("%s: an unchanged value must not require replacement", name)
		}
	}
}

func topicStringRequiresReplace(t *testing.T, attr schema.StringAttribute, state, plan string) bool {
	t.Helper()
	req := planmodifier.StringRequest{
		State:       tfsdk.State{Raw: tftypes.NewValue(tftypes.String, state)},
		StateValue:  types.StringValue(state),
		Plan:        tfsdk.Plan{Raw: tftypes.NewValue(tftypes.String, plan)},
		PlanValue:   types.StringValue(plan),
		ConfigValue: types.StringValue(plan),
	}
	resp := &planmodifier.StringResponse{}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyString(context.Background(), req, resp)
	}
	return resp.RequiresReplace
}

func assertTopicUpdates(t *testing.T, body map[string]interface{}, want []map[string]interface{}) {
	t.Helper()
	raw, ok := body["updates"]
	if !ok {
		t.Fatalf("update body has no %q field: %#v", "updates", body)
	}
	gotList, ok := raw.([]interface{})
	if !ok {
		t.Fatalf("updates is not a list: %#v", raw)
	}
	var got []map[string]interface{}
	for _, item := range gotList {
		m, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("update item is not an object: %#v", item)
		}
		got = append(got, m)
	}
	if canonical(t, got) != canonical(t, want) {
		t.Errorf("updates = %s, want %s", canonical(t, got), canonical(t, want))
	}
}

func canonical(t *testing.T, updates []map[string]interface{}) string {
	t.Helper()
	encoded := make([]string, 0, len(updates))
	for _, u := range updates {
		b, err := json.Marshal(u)
		if err != nil {
			t.Fatalf("marshal update: %v", err)
		}
		encoded = append(encoded, string(b))
	}
	sort.Strings(encoded)
	return strings.Join(encoded, ",")
}

// stringMap builds a known map value for the test models.
func stringMap(t *testing.T, values map[string]string) types.Map {
	t.Helper()
	elements := make(map[string]attr.Value, len(values))
	for k, v := range values {
		elements[k] = types.StringValue(v)
	}
	return types.MapValueMust(types.StringType, elements)
}
