package tag_test

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
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/tag"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Payloads below are copied literally from the Gravitino v1.3.0 OpenAPI spec
// (docs/open-api/tags.yaml).

const (
	tagBasePath    = "/api/metalakes/test_metalake/tags"
	tagItemPath    = tagBasePath + "/my_tag1"
	tagJSONMedia   = "application/vnd.gravitino.v1+json"
	specTagCreate  = `{"name":"my_tag1","comment":"This is my tag1","properties":{"key1":"value1","key2":"value2"}}`
	specTagDropped = `{"code": 0, "dropped": true}`

	// components/examples/TagResponse
	specTagResponse = `{
  "code": 0,
  "tag": {
    "name": "my_tag1",
    "comment": "This is my tag1",
    "properties": {
      "key1": "value1",
      "key2": "value2"
    },
    "audit": {
      "creator": "gravitino",
      "createTime": "2023-12-08T03:41:25.595Z"
    },
    "inherited": false
  }
}`

	// components/examples/NoSuchTagException
	specNoSuchTagException = `{
  "code": 1003,
  "type": "NoSuchTagException",
  "message": "Failed to operate tag(s) [my_tag] operation [LOAD] under metalake [my_test_metalake], reason [NoSuchTagException]",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchTagException: Tag my_tag does not exist",
    "..."
  ]
}`
)

type tagRecorder struct {
	mu       sync.Mutex
	requests []tagRequest
}

type tagRequest struct {
	Method string
	Path   string
	Body   map[string]interface{}
}

func (rec *tagRecorder) record(r *http.Request) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var body map[string]interface{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	rec.requests = append(rec.requests, tagRequest{Method: r.Method, Path: r.URL.Path, Body: body})
}

func (rec *tagRecorder) only(t *testing.T, method string) tagRequest {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var found []tagRequest
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

func (rec *tagRecorder) count(method string) int {
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

func newTagResource(t *testing.T, rec *tagRecorder, handler func(w http.ResponseWriter, r *http.Request)) *res.TagResource {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", tagJSONMedia)
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	r := res.New()
	r.(*res.TagResource).SetClient(c)
	return r.(*res.TagResource)
}

func tagSchema(t *testing.T) schema.Schema {
	t.Helper()
	r := res.New()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func tagModel(t *testing.T, name, comment types.String, properties types.Map, audit types.Object, inherited types.Bool) res.TagResourceModel {
	t.Helper()
	return res.TagResourceModel{
		ID:         types.StringValue("test_metalake." + name.ValueString()),
		Metalake:   types.StringValue("test_metalake"),
		Name:       name,
		Comment:    comment,
		Properties: properties,
		Audit:      audit,
		Inherited:  inherited,
	}
}

func tagValue(t *testing.T, s schema.Schema, model res.TagResourceModel) tftypes.Value {
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

func TestTagResource_Schema(t *testing.T) {
	r := res.New()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.TODO(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_tag" {
		t.Fatalf("expected gravitino_tag, got %s", resp.TypeName)
	}

	s := tagSchema(t)
	for _, name := range []string{"id", "metalake", "name", "comment", "properties", "audit", "inherited"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	inherited, ok := s.Attributes["inherited"].(schema.BoolAttribute)
	if !ok {
		t.Fatal("inherited must be a bool attribute")
	}
	if !inherited.Computed {
		t.Error("inherited must be computed")
	}
	if len(inherited.PlanModifiers) == 0 {
		t.Error("inherited must carry UseStateForUnknown so it is never unknown in state")
	}
	audit, ok := s.Attributes["audit"].(schema.ObjectAttribute)
	if !ok {
		t.Fatal("audit must be an object attribute")
	}
	if len(audit.PlanModifiers) != 0 {
		t.Error("audit must not use UseStateForUnknown: Gravitino rewrites lastModifier/lastModifiedTime on every update, so the plan value must stay unknown")
	}
}

// The create request must be exactly the spec's TagCreate example: the real 1.3.0
// server rejects unknown fields (UnrecognizedPropertyException).
func TestTagResource_Create_SendsSpecPayload(t *testing.T) {
	rec := &tagRecorder{}
	r := newTagResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost && req.URL.Path == tagBasePath {
			io.WriteString(w, specTagResponse)
			return
		}
		http.NotFound(w, req)
	})

	s := tagSchema(t)
	ctx := context.Background()
	plan := tagModel(t, types.StringValue("my_tag1"), types.StringValue("This is my tag1"),
		stringMap(t, map[string]string{"key1": "value1", "key2": "value2"}),
		types.ObjectUnknown(res.AuditAttrTypes), types.BoolUnknown())

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: tagValue(t, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	sent := rec.only(t, http.MethodPost)
	var expected map[string]interface{}
	if err := json.Unmarshal([]byte(specTagCreate), &expected); err != nil {
		t.Fatalf("spec example: %v", err)
	}
	if !reflect.DeepEqual(sent.Body, expected) {
		t.Errorf("create body = %#v, want the spec's TagCreate example %#v", sent.Body, expected)
	}

	var got res.TagResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.ID.ValueString() != "test_metalake.my_tag1" {
		t.Errorf("id = %q", got.ID.ValueString())
	}
	if got.Comment.ValueString() != "This is my tag1" {
		t.Errorf("comment = %q", got.Comment.ValueString())
	}
	if got.Inherited.IsNull() || got.Inherited.IsUnknown() || got.Inherited.ValueBool() {
		t.Errorf("inherited = %v, want false from the spec response", got.Inherited)
	}
	if got.Audit.IsNull() || got.Audit.IsUnknown() {
		t.Fatalf("audit = %v, want the server value", got.Audit)
	}
	if creator, ok := got.Audit.Attributes()["creator"].(types.String); !ok || creator.ValueString() != "gravitino" {
		t.Errorf("audit.creator = %v, want gravitino", got.Audit.Attributes()["creator"])
	}
}

// inherited is nullable in the spec: a response without it must yield a known null.
func TestTagResource_Read_InheritedNullWhenServerOmitsIt(t *testing.T) {
	const responseWithoutInherited = `{"code":0,"tag":{"name":"my_tag1","comment":"This is my tag1","properties":{"key1":"value1"}}}`

	rec := &tagRecorder{}
	r := newTagResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, responseWithoutInherited)
	})

	s := tagSchema(t)
	ctx := context.Background()
	state := tagModel(t, types.StringValue("my_tag1"), types.StringValue("This is my tag1"),
		types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes), types.BoolValue(false))

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tagValue(t, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: tagValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got res.TagResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if !got.Inherited.IsNull() || got.Inherited.IsUnknown() {
		t.Errorf("inherited = %v, want a known null", got.Inherited)
	}
	if !got.Audit.IsNull() || got.Audit.IsUnknown() {
		t.Errorf("audit = %v, want a known null when the server reports none", got.Audit)
	}
	// The imported state has no properties: Read must reconstruct them from the
	// server instead of leaving them null.
	if got.Properties.IsNull() || len(got.Properties.Elements()) != 1 {
		t.Errorf("properties = %v, want the server property after import", got.Properties)
	}
}

func TestTagResource_Read_NotFoundRemovesState(t *testing.T) {
	rec := &tagRecorder{}
	r := newTagResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, specNoSuchTagException)
	})

	s := tagSchema(t)
	ctx := context.Background()
	state := tagModel(t, types.StringValue("my_tag1"), types.StringNull(), types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes), types.BoolNull())
	raw := tagValue(t, s, state)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 read must not error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("404 read must remove the resource from state")
	}
}

// All four spec update types must be sent, and a rename must target the old name.
func TestTagResource_Update_SendsSpecPayload(t *testing.T) {
	const responseAfterUpdate = `{
  "code": 0,
  "tag": {
    "name": "my_tag_new",
    "comment": "This is my new tag comment",
    "properties": {
      "key1": "value1"
    },
    "audit": {
      "creator": "gravitino",
      "createTime": "2023-12-08T03:41:25.595Z"
    },
    "inherited": false
  }
}`

	rec := &tagRecorder{}
	r := newTagResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPut && req.URL.Path == tagItemPath {
			io.WriteString(w, responseAfterUpdate)
			return
		}
		http.NotFound(w, req)
	})

	s := tagSchema(t)
	ctx := context.Background()
	state := tagModel(t, types.StringValue("my_tag1"), types.StringValue("This is my tag1"),
		stringMap(t, map[string]string{"key1": "value1", "key2": "value2"}),
		types.ObjectNull(res.AuditAttrTypes), types.BoolNull())
	plan := tagModel(t, types.StringValue("my_tag_new"), types.StringValue("This is my new tag comment"),
		stringMap(t, map[string]string{"key1": "value1"}),
		types.ObjectUnknown(res.AuditAttrTypes), types.BoolUnknown())

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tagValue(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tagValue(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	sent := rec.only(t, http.MethodPut)
	if sent.Path != tagItemPath {
		t.Errorf("rename must be sent to the old tag name %q, got %q", tagItemPath, sent.Path)
	}
	assertTagUpdates(t, sent.Body, []map[string]interface{}{
		{"@type": "rename", "newName": "my_tag_new"},
		{"@type": "updateComment", "newComment": "This is my new tag comment"},
		{"@type": "removeProperty", "property": "key2"},
	})

	var got res.TagResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Name.ValueString() != "my_tag_new" {
		t.Errorf("name = %q, want the renamed tag", got.Name.ValueString())
	}
	if got.ID.ValueString() != "test_metalake.my_tag_new" {
		t.Errorf("id = %q, want the id to follow the rename", got.ID.ValueString())
	}
	if got.Comment.ValueString() != "This is my new tag comment" {
		t.Errorf("comment = %q", got.Comment.ValueString())
	}
	// The plan's audit is unknown (no UseStateForUnknown): Update must write the
	// server value because Gravitino rewrites lastModifier on every update.
	if got.Audit.IsNull() || got.Audit.IsUnknown() {
		t.Errorf("audit = %v, want the server value after the update", got.Audit)
	}
	if got.Inherited.IsUnknown() {
		t.Error("inherited must never be unknown in state after an update")
	}
}

func TestTagResource_Update_SetPropertyPayload(t *testing.T) {
	rec := &tagRecorder{}
	r := newTagResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, specTagResponse)
	})

	s := tagSchema(t)
	ctx := context.Background()
	state := tagModel(t, types.StringValue("my_tag1"), types.StringValue("This is my tag1"),
		stringMap(t, map[string]string{}), types.ObjectNull(res.AuditAttrTypes), types.BoolNull())
	plan := tagModel(t, types.StringValue("my_tag1"), types.StringValue("This is my tag1"),
		stringMap(t, map[string]string{"key1": "value1"}),
		types.ObjectUnknown(res.AuditAttrTypes), types.BoolUnknown())

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tagValue(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tagValue(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	assertTagUpdates(t, rec.only(t, http.MethodPut).Body, []map[string]interface{}{
		{"@type": "setProperty", "property": "key1", "value": "value1"},
	})
}

// Clearing a tag comment uses updateComment with an empty newComment (the spec
// has no removeComment).
func TestTagResource_Update_ClearsCommentWithEmptyNewComment(t *testing.T) {
	rec := &tagRecorder{}
	r := newTagResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, `{"code":0,"tag":{"name":"my_tag1","comment":"","inherited":null}}`)
	})

	s := tagSchema(t)
	ctx := context.Background()
	state := tagModel(t, types.StringValue("my_tag1"), types.StringValue("This is my tag1"),
		types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes), types.BoolNull())
	plan := tagModel(t, types.StringValue("my_tag1"), types.StringValue(""),
		types.MapNull(types.StringType), types.ObjectUnknown(res.AuditAttrTypes), types.BoolUnknown())

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tagValue(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tagValue(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	assertTagUpdates(t, rec.only(t, http.MethodPut).Body, []map[string]interface{}{
		{"@type": "updateComment", "newComment": ""},
	})
}

// With nothing to change the resource must not send an update request, and every
// computed attribute must still be written from a real server read.
func TestTagResource_Update_NoChangesRefreshesState(t *testing.T) {
	rec := &tagRecorder{}
	r := newTagResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, specTagResponse)
	})

	s := tagSchema(t)
	ctx := context.Background()
	state := tagModel(t, types.StringValue("my_tag1"), types.StringValue("This is my tag1"),
		stringMap(t, map[string]string{"key1": "value1", "key2": "value2"}),
		types.ObjectNull(res.AuditAttrTypes), types.BoolNull())
	plan := tagModel(t, types.StringValue("my_tag1"), types.StringValue("This is my tag1"),
		stringMap(t, map[string]string{"key1": "value1", "key2": "value2"}),
		types.ObjectUnknown(res.AuditAttrTypes), types.BoolUnknown())

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tagValue(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tagValue(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	if rec.count(http.MethodPut) != 0 {
		t.Error("no update request must be sent when nothing changed")
	}
	if rec.count(http.MethodGet) != 1 {
		t.Error("the resource must refresh from the server when nothing changed")
	}

	var got res.TagResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Inherited.IsNull() || got.Inherited.IsUnknown() {
		t.Errorf("inherited = %v, want the server value", got.Inherited)
	}
	if got.Audit.IsNull() || got.Audit.IsUnknown() {
		t.Errorf("audit = %v, want the server value", got.Audit)
	}
	if got.Properties.IsNull() || len(got.Properties.Elements()) != 2 {
		t.Errorf("properties = %v, want the two server properties", got.Properties)
	}
}

func TestTagResource_Delete(t *testing.T) {
	rec := &tagRecorder{}
	r := newTagResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, specTagDropped)
	})

	s := tagSchema(t)
	ctx := context.Background()
	state := tagModel(t, types.StringValue("my_tag1"), types.StringValue("This is my tag1"), types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes), types.BoolNull())

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tagValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", resp.Diagnostics)
	}
	if rec.count(http.MethodDelete) != 1 {
		t.Fatal("delete was not called")
	}
}

func TestTagResource_Delete_NotFoundIsSuccess(t *testing.T) {
	rec := &tagRecorder{}
	r := newTagResource(t, rec, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, specNoSuchTagException)
	})

	s := tagSchema(t)
	ctx := context.Background()
	state := tagModel(t, types.StringValue("my_tag1"), types.StringNull(), types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes), types.BoolNull())

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tagValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 delete must be treated as success: %v", resp.Diagnostics)
	}
}

func TestTagResource_ImportState(t *testing.T) {
	r := res.New().(resource.ResourceWithImportState)
	ctx := context.Background()
	s := tagSchema(t)

	nullModel := res.TagResourceModel{
		ID:         types.StringNull(),
		Metalake:   types.StringNull(),
		Name:       types.StringNull(),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(res.AuditAttrTypes),
		Inherited:  types.BoolNull(),
	}

	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tagValue(t, s, nullModel)}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "my_metalake.my_tag"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	var got res.TagResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Metalake.ValueString() != "my_metalake" || got.Name.ValueString() != "my_tag" {
		t.Errorf("imported state = %#v", got)
	}
	if got.ID.ValueString() != "my_metalake.my_tag" {
		t.Errorf("imported id = %q", got.ID.ValueString())
	}
}

func TestTagResource_ImportState_Invalid(t *testing.T) {
	for _, id := range []string{"no_dot_here", ".my_tag", "my_metalake.", ""} {
		t.Run(id, func(t *testing.T) {
			r := res.New().(resource.ResourceWithImportState)
			ctx := context.Background()
			s := tagSchema(t)

			nullModel := res.TagResourceModel{
				ID:         types.StringNull(),
				Metalake:   types.StringNull(),
				Name:       types.StringNull(),
				Comment:    types.StringNull(),
				Properties: types.MapNull(types.StringType),
				Audit:      types.ObjectNull(res.AuditAttrTypes),
				Inherited:  types.BoolNull(),
			}

			resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tagValue(t, s, nullModel)}}
			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error for invalid import ID %q", id)
			}
		})
	}
}

// A rename changes the id (metalake.name). The id attribute uses
// UseStateForUnknown, so ModifyPlan must mark it unknown as soon as the name
// changes; otherwise Terraform rejects the applied rename with "provider
// produced inconsistent result after apply".
func TestTagResource_ModifyPlan_MarksIdUnknownOnRename(t *testing.T) {
	s := tagSchema(t)
	modifier, ok := res.New().(resource.ResourceWithModifyPlan)
	if !ok {
		t.Fatal("tag resource must implement ResourceWithModifyPlan so that a rename can mark the id unknown")
	}

	ctx := context.Background()
	state := tagModel(t, types.StringValue("my_tag1"), types.StringValue("This is my tag1"),
		types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes), types.BoolNull())
	// The id UseStateForUnknown would copy into the plan during a rename.
	plan := tagModel(t, types.StringValue("my_tag_new"), types.StringValue("This is my tag1"),
		types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes), types.BoolNull())
	plan.ID = types.StringValue("test_metalake.my_tag1")

	planValue := tagValue(t, s, plan)
	resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: s, Raw: planValue}}
	modifier.ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: planValue},
		State: tfsdk.State{Schema: s, Raw: tagValue(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("modify plan diagnostics: %v", resp.Diagnostics)
	}

	var got res.TagResourceModel
	if diags := resp.Plan.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read plan: %v", diags)
	}
	if !got.ID.IsUnknown() {
		t.Errorf("id = %v, want unknown after a rename", got.ID)
	}
}

func TestTagResource_ModifyPlan_KeepsIdWhenNameUnchanged(t *testing.T) {
	s := tagSchema(t)
	modifier, ok := res.New().(resource.ResourceWithModifyPlan)
	if !ok {
		t.Fatal("tag resource must implement ResourceWithModifyPlan")
	}

	ctx := context.Background()
	state := tagModel(t, types.StringValue("my_tag1"), types.StringValue("This is my tag1"),
		types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes), types.BoolNull())
	plan := tagModel(t, types.StringValue("my_tag1"), types.StringValue("This is my tag2"),
		types.MapNull(types.StringType), types.ObjectNull(res.AuditAttrTypes), types.BoolNull())

	planValue := tagValue(t, s, plan)
	resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: s, Raw: planValue}}
	modifier.ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: planValue},
		State: tfsdk.State{Schema: s, Raw: tagValue(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("modify plan diagnostics: %v", resp.Diagnostics)
	}

	var got res.TagResourceModel
	if diags := resp.Plan.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read plan: %v", diags)
	}
	if got.ID.IsUnknown() || got.ID.ValueString() != "test_metalake.my_tag1" {
		t.Errorf("id = %v, want the planned UseStateForUnknown value when the name is unchanged", got.ID)
	}
}

// Tags cannot be moved between metalakes, but they can be renamed in place.
func TestTagResource_PlanModifiers(t *testing.T) {
	s := tagSchema(t)

	metalake, ok := s.Attributes["metalake"].(schema.StringAttribute)
	if !ok {
		t.Fatal("metalake must be a string attribute")
	}
	if !tagStringRequiresReplace(t, metalake, "old", "new") {
		t.Error("metalake: changing the value must require replacement")
	}

	name, ok := s.Attributes["name"].(schema.StringAttribute)
	if !ok {
		t.Fatal("name must be a string attribute")
	}
	if tagStringRequiresReplace(t, name, "my_tag1", "my_tag_new") {
		t.Error("name: a rename must be applied in place, not by replacing the tag")
	}
}

func tagStringRequiresReplace(t *testing.T, attr schema.StringAttribute, state, plan string) bool {
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

func assertTagUpdates(t *testing.T, body map[string]interface{}, want []map[string]interface{}) {
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
	if canonicalTagUpdates(t, got) != canonicalTagUpdates(t, want) {
		t.Errorf("updates = %s, want %s", canonicalTagUpdates(t, got), canonicalTagUpdates(t, want))
	}
}

func canonicalTagUpdates(t *testing.T, updates []map[string]interface{}) string {
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
