package metalake_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/metalake"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// noSuchMetalakeError is the verbatim NoSuchMetalakeException example of
// metalakes.yaml, returned by Gravitino with HTTP status 404.
const noSuchMetalakeError = `{
  "code": 1003,
  "type": "NoSuchMetalakeException",
  "message": "Failed to operate metalake(s) [test] operation [LOAD], reason [NoSuchMetalakeException]",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchMetalakeException: Metalake test does not exist",
    "..."
  ]
}`

// metalakeResponse is the MetalakeResponse example of metalakes.yaml.
const metalakeResponse = `{
  "code": 0,
  "metalake": {
    "name": "my_metalake",
    "comment": "This is my metalake",
    "properties": {
      "key1": "value1",
      "key2": "value2",
      "gravitino.identifier": "gravitino.v1.uid2062071866014250017"
    },
    "audit": {
      "creator": "gravitino",
      "createTime": "2023-12-06T14:21:24.982Z"
    }
  }
}`

var nonNullRaw = tftypes.NewValue(tftypes.String, "state")

func metalakeSchema(t *testing.T) schema.Schema {
	t.Helper()
	r := res.NewMetalakeResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func tfValue(t *testing.T, ctx context.Context, s schema.Schema, model res.MetalakeResourceModel) tftypes.Value {
	t.Helper()
	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("failed to build object value: %v", diags)
	}
	v, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}
	return v
}

func props(t *testing.T, values map[string]string) types.Map {
	t.Helper()
	attrs := make(map[string]attr.Value, len(values))
	for k, v := range values {
		attrs[k] = types.StringValue(v)
	}
	return types.MapValueMust(types.StringType, attrs)
}

func baseModel() res.MetalakeResourceModel {
	return res.MetalakeResourceModel{
		Name:       types.StringValue("my_metalake"),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(models.AuditAttrTypes),
	}
}

func TestMetalakeResource_Metadata(t *testing.T) {
	r := res.NewMetalakeResource()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "gravitino"}, resp)
	if resp.TypeName != "gravitino_metalake" {
		t.Fatalf("expected gravitino_metalake, got %s", resp.TypeName)
	}
}

// TestMetalakeResource_SchemaUpdatePolicy: metalakes support rename,
// updateComment, setProperty and removeProperty (metalakes.yaml), so name,
// comment and properties are updateable; the Computed attributes must be known
// on every code path.
func TestMetalakeResource_SchemaUpdatePolicy(t *testing.T) {
	ctx := context.Background()
	s := metalakeSchema(t)

	for _, name := range []string{"name", "comment"} {
		attr := s.Attributes[name].(schema.StringAttribute)
		req := planmodifier.StringRequest{StateValue: types.StringValue("old"), PlanValue: types.StringValue("new"),
			State: tfsdk.State{Raw: nonNullRaw}, Plan: tfsdk.Plan{Raw: nonNullRaw}, Config: tfsdk.Config{Raw: nonNullRaw}}
		r := &planmodifier.StringResponse{PlanValue: types.StringValue("new")}
		for _, m := range attr.PlanModifiers {
			m.PlanModifyString(ctx, req, r)
		}
		if r.RequiresReplace {
			t.Fatalf("%s: must be updateable in place, but RequiresReplace is set", name)
		}
	}

	// Comment is Optional+Computed: an unknown plan value must be resolved from
	// the prior state, otherwise it would be sent as an empty comment.
	commentAttr := s.Attributes["comment"].(schema.StringAttribute)
	req := planmodifier.StringRequest{
		StateValue: types.StringValue("prior"), PlanValue: types.StringUnknown(), ConfigValue: types.StringNull(),
		State: tfsdk.State{Raw: nonNullRaw}, Plan: tfsdk.Plan{Raw: nonNullRaw}, Config: tfsdk.Config{Raw: nonNullRaw},
	}
	commentResp := &planmodifier.StringResponse{PlanValue: types.StringUnknown()}
	for _, m := range commentAttr.PlanModifiers {
		m.PlanModifyString(ctx, req, commentResp)
	}
	if commentResp.PlanValue.ValueString() != "prior" {
		t.Fatalf("comment: expected UseStateForUnknown to plan the prior value, got %v", commentResp.PlanValue)
	}

	// properties: optional+computed map, same guarantee.
	priorProps := props(t, map[string]string{"env": "dev"})
	propsAttr := s.Attributes["properties"].(schema.MapAttribute)
	mapReq := planmodifier.MapRequest{
		StateValue: priorProps, PlanValue: types.MapUnknown(types.StringType), ConfigValue: types.MapNull(types.StringType),
		State: tfsdk.State{Raw: nonNullRaw}, Plan: tfsdk.Plan{Raw: nonNullRaw}, Config: tfsdk.Config{Raw: nonNullRaw},
	}
	mapResp := &planmodifier.MapResponse{PlanValue: types.MapUnknown(types.StringType)}
	for _, m := range propsAttr.PlanModifiers {
		m.PlanModifyMap(ctx, mapReq, mapResp)
	}
	if !mapResp.PlanValue.Equal(priorProps) {
		t.Fatalf("properties: expected UseStateForUnknown to plan the prior value, got %v", mapResp.PlanValue)
	}

	// audit: Computed object.
	auditAttr := s.Attributes["audit"].(schema.ObjectAttribute)
	priorAudit := types.ObjectValueMust(models.AuditAttrTypes, map[string]attr.Value{
		"creator":            types.StringValue("gravitino"),
		"create_time":        types.StringValue("2023-12-06T14:21:24Z"),
		"last_modifier":      types.StringNull(),
		"last_modified_time": types.StringNull(),
	})
	objReq := planmodifier.ObjectRequest{
		StateValue: priorAudit, PlanValue: types.ObjectUnknown(models.AuditAttrTypes), ConfigValue: types.ObjectNull(models.AuditAttrTypes),
		State: tfsdk.State{Raw: nonNullRaw}, Plan: tfsdk.Plan{Raw: nonNullRaw}, Config: tfsdk.Config{Raw: nonNullRaw},
	}
	// audit is server-managed (lastModifier/lastModifiedTime change on every
	// update): it must stay unknown in the plan instead of pinning the prior
	// state, otherwise an in-place update would fail with "Provider produced
	// inconsistent result after apply".
	if len(auditAttr.PlanModifiers) != 0 {
		t.Fatalf("audit: expected no plan modifiers, got %d", len(auditAttr.PlanModifiers))
	}
	objResp := &planmodifier.ObjectResponse{PlanValue: types.ObjectUnknown(models.AuditAttrTypes)}
	for _, m := range auditAttr.PlanModifiers {
		m.PlanModifyObject(ctx, objReq, objResp)
	}
	if !objResp.PlanValue.IsUnknown() {
		t.Fatalf("audit: expected the planned value to stay unknown, got %v", objResp.PlanValue)
	}
}

func TestMetalakeResource_Create(t *testing.T) {
	ctx := context.Background()
	s := metalakeSchema(t)

	var method, requestPath string
	var rawBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, requestPath = r.Method, r.URL.Path
		rawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, metalakeResponse)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewMetalakeResource()
	r.(*res.MetalakeResource).SetClient(c)

	plan := baseModel()
	plan.Comment = types.StringValue("This is my metalake")
	plan.Properties = props(t, map[string]string{"key1": "value1", "key2": "value2"})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if method != http.MethodPost || requestPath != "/api/metalakes" {
		t.Fatalf("unexpected request %s %s", method, requestPath)
	}
	var body map[string]any
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("invalid request body: %v", err)
	}
	want := map[string]any{
		"name":       "my_metalake",
		"comment":    "This is my metalake",
		"properties": map[string]any{"key1": "value1", "key2": "value2"},
	}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(body)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("unexpected create body:\n want %s\n  got %s", wantJSON, gotJSON)
	}

	var state res.MetalakeResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.ID.ValueString() != "my_metalake" {
		t.Fatalf("unexpected id: %s", state.ID.ValueString())
	}
	gotProps := make(map[string]string)
	if diags := state.Properties.ElementsAs(ctx, &gotProps, false); diags.HasError() {
		t.Fatalf("failed to read properties: %v", diags)
	}
	// The server echoed "gravitino.identifier", which must not become part of
	// the state of an Optional+Computed attribute.
	if _, ok := gotProps["gravitino.identifier"]; ok {
		t.Fatalf("server-only properties must not leak into state: %#v", gotProps)
	}
	if gotProps["key1"] != "value1" || gotProps["key2"] != "value2" {
		t.Fatalf("unexpected properties in state: %#v", gotProps)
	}
	if state.Audit.IsNull() {
		t.Fatal("audit must be set from the response")
	}
}

// TestMetalakeResource_ReadKeepsConfiguredProperties: a server-only 'in-use'
// key must never appear in state, while the configured keys keep the server
// values.
func TestMetalakeResource_ReadKeepsConfiguredProperties(t *testing.T) {
	ctx := context.Background()
	s := metalakeSchema(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, metalakeResponse)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewMetalakeResource()
	r.(*res.MetalakeResource).SetClient(c)

	state := baseModel()
	state.ID = types.StringValue("my_metalake")
	state.Comment = types.StringValue("This is my metalake")
	state.Properties = props(t, map[string]string{"key1": "value1", "key2": "value2"})

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got res.MetalakeResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	gotProps := make(map[string]string)
	if diags := got.Properties.ElementsAs(ctx, &gotProps, false); diags.HasError() {
		t.Fatalf("failed to read properties: %v", diags)
	}
	if len(gotProps) != 2 || gotProps["key1"] != "value1" || gotProps["key2"] != "value2" {
		t.Fatalf("unexpected properties after read: %#v", gotProps)
	}
}

func TestMetalakeResource_ReadNotFoundRemovesState(t *testing.T) {
	ctx := context.Background()
	s := metalakeSchema(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchMetalakeError)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewMetalakeResource()
	r.(*res.MetalakeResource).SetClient(c)

	state := baseModel()
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 must not produce an error, got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected the metalake to be removed from state on 404")
	}
}

func TestMetalakeResource_Update(t *testing.T) {
	ctx := context.Background()
	s := metalakeSchema(t)

	var method, requestPath string
	var updates []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, requestPath = r.Method, r.URL.Path
		var body struct {
			Updates []map[string]any `json:"updates"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		updates = body.Updates
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, metalakeResponse)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewMetalakeResource()
	r.(*res.MetalakeResource).SetClient(c)

	state := baseModel()
	state.ID = types.StringValue("my_metalake")
	state.Comment = types.StringValue("old comment")
	state.Properties = props(t, map[string]string{"key1": "value1", "drop": "me"})

	plan := baseModel()
	plan.Name = types.StringValue("my_metalake_new")
	plan.Comment = types.StringValue("This is my new metalake comment")
	plan.Properties = props(t, map[string]string{"key1": "value1", "key2": "value2"})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if method != http.MethodPut {
		t.Fatalf("expected PUT, got %s", method)
	}
	if requestPath != "/api/metalakes/my_metalake" {
		t.Fatalf("a rename must target the current name, got %s", requestPath)
	}

	wantUpdates := []string{
		`{"@type":"rename","newName":"my_metalake_new"}`,
		`{"@type":"updateComment","newComment":"This is my new metalake comment"}`,
		`{"@type":"setProperty","property":"key2","value":"value2"}`,
		`{"@type":"removeProperty","property":"drop"}`,
	}
	gotUpdates := make([]string, 0, len(updates))
	for _, u := range updates {
		b, _ := json.Marshal(u)
		gotUpdates = append(gotUpdates, string(b))
	}
	for _, want := range wantUpdates {
		found := false
		for _, got := range gotUpdates {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing update %s in %v", want, gotUpdates)
		}
	}
	if len(gotUpdates) != len(wantUpdates) {
		t.Fatalf("unexpected number of updates: %v", gotUpdates)
	}
}

// TestMetalakeResource_UpdateWithoutChangesSkipsApi: a no-op update must not
// send an empty updates array (the plan may carry unknown computed values).
func TestMetalakeResource_UpdateWithoutChangesSkipsApi(t *testing.T) {
	ctx := context.Background()
	s := metalakeSchema(t)

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, metalakeResponse)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewMetalakeResource()
	r.(*res.MetalakeResource).SetClient(c)

	state := baseModel()
	state.ID = types.StringValue("my_metalake")
	state.Comment = types.StringValue("This is my metalake")
	state.Properties = props(t, map[string]string{"key1": "value1"})

	plan := state
	plan.Comment = types.StringUnknown()
	plan.Properties = types.MapUnknown(types.StringType)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
	}, resp)

	if calls != 0 {
		t.Fatalf("expected no API call for an update without changes, got %d", calls)
	}

	// audit is unknown in the plan, so the state must end up with a known value.
	var got res.MetalakeResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if got.Audit.IsUnknown() {
		t.Fatal("audit must be a known value after apply")
	}
}

func TestMetalakeResource_Delete(t *testing.T) {
	ctx := context.Background()
	s := metalakeSchema(t)

	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Query().Get("force") != "true" {
			t.Errorf("expected force=true, got %q", r.URL.Query().Get("force"))
		}
		json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewMetalakeResource()
	r.(*res.MetalakeResource).SetClient(c)

	state := baseModel()
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !called {
		t.Fatal("delete was not called")
	}
}

func TestMetalakeResource_DeleteNotFoundIsSuccess(t *testing.T) {
	ctx := context.Background()
	s := metalakeSchema(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchMetalakeError)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewMetalakeResource()
	r.(*res.MetalakeResource).SetClient(c)

	state := baseModel()
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("deleting an already deleted metalake must succeed, got: %v", resp.Diagnostics)
	}
}

func TestMetalakeResource_Import(t *testing.T) {
	ctx := context.Background()
	s := metalakeSchema(t)
	r := res.NewMetalakeResource().(resource.ResourceWithImportState)

	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, baseModel())}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "my_metalake"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state res.MetalakeResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.Name.ValueString() != "my_metalake" {
		t.Fatalf("unexpected name: %s", state.Name.ValueString())
	}
}

func TestMetalakeResource_ImportInvalid(t *testing.T) {
	ctx := context.Background()
	s := metalakeSchema(t)
	r := res.NewMetalakeResource().(resource.ResourceWithImportState)

	for _, id := range []string{"", "my_metalake.my_catalog"} {
		resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, baseModel())}}
		r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected an error for invalid import ID %q", id)
		}
	}
}

// TestMetalakeResource_ReadReportsGravitinoError checks that API failures are
// reported through client.NewResourceError (with the Gravitino error type).
func TestMetalakeResource_ReadReportsGravitinoError(t *testing.T) {
	ctx := context.Background()
	s := metalakeSchema(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalError","message":"boom","stack":["..."]}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewMetalakeResource()
	r.(*res.MetalakeResource).SetClient(c)

	state := baseModel()
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Failed reading metalake") {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

// TestMetalakeResource_ModifyPlan: an in-place rename must mark the id as
// unknown (the id is the name), otherwise the applied id differs from the
// planned one ("Provider produced inconsistent result after apply").
func TestMetalakeResource_ModifyPlan(t *testing.T) {
	ctx := context.Background()
	s := metalakeSchema(t)
	r := res.NewMetalakeResource().(resource.ResourceWithModifyPlan)

	state := baseModel()
	state.ID = types.StringValue("my_metalake")

	rename := state
	rename.Name = types.StringValue("my_metalake_new")

	for _, tc := range []struct {
		name        string
		plan        res.MetalakeResourceModel
		wantUnknown bool
	}{
		{"rename", rename, true},
		{"no change", state, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, tc.plan)}}
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{
				Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, tc.plan)},
				State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}

			var got res.MetalakeResourceModel
			if diags := resp.Plan.Get(ctx, &got); diags.HasError() {
				t.Fatalf("failed to read plan: %v", diags)
			}
			if got.ID.IsUnknown() != tc.wantUnknown {
				t.Fatalf("expected id unknown=%v, got %v", tc.wantUnknown, got.ID)
			}
		})
	}
}
