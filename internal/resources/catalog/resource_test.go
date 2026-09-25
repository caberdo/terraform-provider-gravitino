package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/catalog"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// noSuchCatalogError is the verbatim NoSuchCatalogException payload of
// /tmp/gravspec/catalogs.yaml (components/examples/NoSuchCatalogException),
// returned by the Gravitino server with HTTP status 404.
const noSuchCatalogError = `{
  "code": 1003,
  "type": "NoSuchCatalogException",
  "message": "Failed to operate catalog(s) [test] operation [LOAD] under metalake [my_test_metalake], reason [NoSuchCatalogException]",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchCatalogException: Catalog my_test_metalake.test does not exist",
    "..."
  ]
}`

// catalogResponse is the `CatalogResponse` example of catalogs.yaml, extended
// with the server-owned "in-use" property that a real Gravitino 1.3.0 server
// always adds to a catalog (measured against the live server).
const catalogResponse = `{
  "code": 0,
  "catalog": {
    "name": "my_hive_catalog",
    "type": "relational",
    "provider": "hive",
    "comment": "This is my hive catalog",
    "properties": {
      "key1": "value1",
      "in-use": "true",
      "gravitino.bypass.hive.metastore.client.capability.check": "false",
      "metastore.uris": "thrift://127.0.0.1:9083"
    },
    "audit": {
      "creator": "gravitino",
      "createTime": "2023-12-08T03:41:25.595Z"
    }
  }
}`

func catalogSchema(t *testing.T) schema.Schema {
	t.Helper()
	r := res.New()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func tfValue(t *testing.T, ctx context.Context, s schema.Schema, model res.CatalogResourceModel) tftypes.Value {
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

func baseModel() res.CatalogResourceModel {
	return res.CatalogResourceModel{
		Metalake:   types.StringValue("my_test_metalake"),
		Name:       types.StringValue("my_hive_catalog"),
		Type:       types.StringValue("relational"),
		Provider:   types.StringValue("hive"),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(res.AuditAttrTypes),
	}
}

func TestCatalogResource_Metadata(t *testing.T) {
	r := res.New()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_catalog" {
		t.Fatalf("expected gravitino_catalog, got %s", resp.TypeName)
	}
}

// TestCatalogResource_SchemaUpdatePolicy locks in which attributes may be
// changed in place (per the v1.3.0 catalogs.yaml update requests) and which
// ones force a replacement.
func TestCatalogResource_SchemaUpdatePolicy(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)

	for _, name := range []string{"metalake", "type", "catalog_provider"} {
		attr := s.Attributes[name].(schema.StringAttribute)
		_, requiresReplace := stringPlan(ctx, attr, types.StringValue("old"), types.StringValue("new"))
		if !requiresReplace {
			t.Fatalf("%s: changing this value must force a replacement (no update request exists in catalogs.yaml)", name)
		}
	}

	// name and comment are updateable: the API has rename and updateComment.
	for _, name := range []string{"name", "comment"} {
		attr := s.Attributes[name].(schema.StringAttribute)
		_, requiresReplace := stringPlan(ctx, attr, types.StringValue("old"), types.StringValue("new"))
		if requiresReplace {
			t.Fatalf("%s: must be updateable in place, but RequiresReplace is set", name)
		}
	}

	// Every computed attribute must be known on every code path; the plan
	// modifiers guarantee that for the "nothing changed" Update branch.
	for _, name := range []string{"id", "comment", "catalog_provider"} {
		attr := s.Attributes[name].(schema.StringAttribute)
		planned, _ := stringPlanWithConfig(ctx, attr, types.StringValue("prior"), types.StringUnknown(), types.StringNull())
		if planned.IsUnknown() || planned.IsNull() || planned.ValueString() != "prior" {
			t.Fatalf("%s: expected UseStateForUnknown to keep the prior state value, got %v", name, planned)
		}
	}

	priorProps := types.MapValueMust(types.StringType, map[string]attr.Value{"key1": types.StringValue("value1")})
	propsAttr := s.Attributes["properties"].(schema.MapAttribute)
	plannedProps, _ := mapPlanWithConfig(ctx, propsAttr, priorProps, types.MapUnknown(types.StringType), types.MapNull(types.StringType))
	if plannedProps.IsUnknown() || !plannedProps.Equal(priorProps) {
		t.Fatalf("properties: expected UseStateForUnknown to keep the prior state value, got %v", plannedProps)
	}

	priorAudit := types.ObjectValueMust(res.AuditAttrTypes, map[string]attr.Value{
		"creator":            types.StringValue("gravitino"),
		"create_time":        types.StringValue("2023-12-08T03:41:25Z"),
		"last_modifier":      types.StringNull(),
		"last_modified_time": types.StringNull(),
	})
	// audit is server-managed (lastModifier/lastModifiedTime change on every
	// update): it must stay unknown in the plan instead of pinning the prior
	// state, otherwise an in-place update would fail with "Provider produced
	// inconsistent result after apply".
	auditAttr := s.Attributes["audit"].(schema.ObjectAttribute)
	plannedAudit, _ := objectPlanWithConfig(ctx, auditAttr, priorAudit, types.ObjectUnknown(res.AuditAttrTypes), types.ObjectNull(res.AuditAttrTypes))
	if !plannedAudit.IsUnknown() {
		t.Fatalf("audit: expected the planned value to stay unknown, got %v", plannedAudit)
	}

	// Catalog properties may hold credentials (JDBC passwords, S3 keys).
	if !propsAttr.Sensitive {
		t.Fatal("properties: expected Sensitive: true")
	}

	// Enum validator must reject values that are not in catalogs.yaml.
	typeAttr := s.Attributes["type"].(schema.StringAttribute)
	v := validator.StringRequest{ConfigValue: types.StringValue("bogus"), Path: path.Root("type")}
	vResp := &validator.StringResponse{}
	for _, val := range typeAttr.Validators {
		val.ValidateString(ctx, v, vResp)
	}
	if !vResp.Diagnostics.HasError() {
		t.Fatal("type: expected an enum validator rejecting an unknown catalog type")
	}
}

// nonNullRaw stands in for the raw state/plan/config values that the framework
// passes to plan modifiers; the modifiers only inspect IsNull() on them.
var nonNullRaw = tftypes.NewValue(tftypes.String, "state")

func stringPlan(ctx context.Context, attr schema.StringAttribute, state, plan types.String) (types.String, bool) {
	return stringPlanWithConfig(ctx, attr, state, plan, plan)
}

// stringPlanWithConfig runs the attribute's plan modifiers with an explicit
// configuration value; UseStateForUnknown only kicks in when the configuration
// is known (null counts as known) while the planned value is unknown.
func stringPlanWithConfig(ctx context.Context, attr schema.StringAttribute, state, plan, config types.String) (types.String, bool) {
	req := planmodifier.StringRequest{
		State:       tfsdk.State{Raw: nonNullRaw},
		Plan:        tfsdk.Plan{Raw: nonNullRaw},
		Config:      tfsdk.Config{Raw: nonNullRaw},
		StateValue:  state,
		PlanValue:   plan,
		ConfigValue: config,
	}
	resp := &planmodifier.StringResponse{PlanValue: plan}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyString(ctx, req, resp)
	}
	return resp.PlanValue, resp.RequiresReplace
}

func mapPlanWithConfig(ctx context.Context, attr schema.MapAttribute, state, plan, config types.Map) (types.Map, bool) {
	req := planmodifier.MapRequest{
		State:       tfsdk.State{Raw: nonNullRaw},
		Plan:        tfsdk.Plan{Raw: nonNullRaw},
		Config:      tfsdk.Config{Raw: nonNullRaw},
		StateValue:  state,
		PlanValue:   plan,
		ConfigValue: config,
	}
	resp := &planmodifier.MapResponse{PlanValue: plan}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyMap(ctx, req, resp)
	}
	return resp.PlanValue, resp.RequiresReplace
}

func objectPlanWithConfig(ctx context.Context, attr schema.ObjectAttribute, state, plan, config types.Object) (types.Object, bool) {
	req := planmodifier.ObjectRequest{
		State:       tfsdk.State{Raw: nonNullRaw},
		Plan:        tfsdk.Plan{Raw: nonNullRaw},
		Config:      tfsdk.Config{Raw: nonNullRaw},
		StateValue:  state,
		PlanValue:   plan,
		ConfigValue: config,
	}
	resp := &planmodifier.ObjectResponse{PlanValue: plan}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyObject(ctx, req, resp)
	}
	return resp.PlanValue, resp.RequiresReplace
}

func TestCatalogResource_Create(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)

	var method, requestPath string
	var rawBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, requestPath = r.Method, r.URL.Path
		rawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, catalogResponse)
	}))
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	r := res.New()
	r.(*res.CatalogResource).SetClient(c)

	plan := baseModel()
	plan.Comment = types.StringValue("This is my hive catalog")
	plan.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{"key1": types.StringValue("value1")})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	// The create body must match CatalogCreateRequest of catalogs.yaml exactly:
	// only the fields of the spec, with the spec's camelCase names.
	if method != http.MethodPost {
		t.Fatalf("expected POST, got %s", method)
	}
	if want := "/api/metalakes/my_test_metalake/catalogs"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}

	var body map[string]any
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("invalid request body %s: %v", rawBody, err)
	}
	want := map[string]any{
		"name":       "my_hive_catalog",
		"type":       "relational",
		"provider":   "hive",
		"comment":    "This is my hive catalog",
		"properties": map[string]any{"key1": "value1"},
	}
	assertJSONEqual(t, want, body)

	var state res.CatalogResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.ID.ValueString() != "my_test_metalake.my_hive_catalog" {
		t.Fatalf("unexpected id: %s", state.ID.ValueString())
	}
	if state.Comment.ValueString() != "This is my hive catalog" {
		t.Fatalf("unexpected comment: %s", state.Comment.ValueString())
	}
	// The server echoed extra properties; only configured keys are tracked.
	if got, ok := state.Properties.Elements()["key1"].(types.String); !ok || got.ValueString() != "value1" {
		t.Fatalf("unexpected properties: %v", state.Properties)
	}
	for _, serverOnly := range []string{"metastore.uris", "in-use"} {
		if _, ok := state.Properties.Elements()[serverOnly]; ok {
			t.Fatalf("server-only property %q must not leak into state", serverOnly)
		}
	}
	if state.Audit.IsNull() {
		t.Fatal("audit must be set from the response (Computed attribute)")
	}
}

func TestCatalogResource_ReadNotFoundRemovesState(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchCatalogError)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.New()
	r.(*res.CatalogResource).SetClient(c)

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_hive_catalog")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 must not produce an error, got: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected the resource to be removed from state on 404")
	}
}

func TestCatalogResource_ReadServerErrorReportsGravitinoError(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalServerError","message":"boom","stack":["..."],"}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.New()
	r.(*res.CatalogResource).SetClient(c)

	state := baseModel()
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed reading catalog "my_hive_catalog"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

// TestCatalogResource_Update asserts the effective update payload and that the
// request is sent to the *current* name, also when the catalog is renamed.
func TestCatalogResource_Update(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)

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
		fmt.Fprint(w, `{
  "code": 0,
  "catalog": {
    "name": "my_catalog_new",
    "type": "relational",
    "provider": "hive",
    "comment": "This is my new catalog comment",
    "properties": {"keeper": "keep", "value1": "value1"},
    "audit": {"creator": "gravitino", "createTime": "2023-12-08T03:41:25.595Z"}
  }
}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.New()
	r.(*res.CatalogResource).SetClient(c)

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_hive_catalog")
	state.Comment = types.StringValue("This is my hive catalog")
	state.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{
		"keeper": types.StringValue("keep"),
		"key2":   types.StringValue("value2"),
	})

	plan := baseModel()
	plan.Name = types.StringValue("my_catalog_new")
	plan.Comment = types.StringValue("This is my new catalog comment")
	plan.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{
		"keeper": types.StringValue("keep"),
		"value1": types.StringValue("value1"),
	})

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
	if want := "/api/metalakes/my_test_metalake/catalogs/my_hive_catalog"; requestPath != want {
		t.Fatalf("a rename must target the current name; expected path %s, got %s", want, requestPath)
	}

	wantUpdates := []map[string]any{
		{"@type": "rename", "newName": "my_catalog_new"},
		{"@type": "updateComment", "newComment": "This is my new catalog comment"},
		{"@type": "setProperty", "property": "value1", "value": "value1"},
		{"@type": "removeProperty", "property": "key2"},
	}
	assertUpdatesEqual(t, wantUpdates, updates)

	var newState res.CatalogResourceModel
	if diags := resp.State.Get(ctx, &newState); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if newState.ID.ValueString() != "my_test_metalake.my_catalog_new" {
		t.Fatalf("unexpected id after rename: %s", newState.ID.ValueString())
	}
}

// TestCatalogResource_UpdateIgnoresUnknownPlanValues guards against the classic
// "Computed attribute is unknown in the plan" bug: an unknown comment or
// property map must never be turned into an updateComment("") or a
// removeProperty request.
func TestCatalogResource_UpdateIgnoresUnknownPlanValues(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)

	var updates []map[string]any
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Updates []map[string]any `json:"updates"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		updates = body.Updates
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, catalogResponse)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.New()
	r.(*res.CatalogResource).SetClient(c)

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_hive_catalog")
	state.Comment = types.StringValue("This is my hive catalog")
	state.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{"key1": types.StringValue("value1")})

	plan := baseModel()
	plan.Comment = types.StringUnknown()
	plan.Properties = types.MapUnknown(types.StringType)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
	}, resp)

	for _, u := range updates {
		if u["@type"] == "updateComment" || u["@type"] == "removeProperty" {
			t.Fatalf("unknown plan values must not be sent as updates, got %v", updates)
		}
	}
	if calls != 0 && len(updates) != 0 {
		t.Fatalf("unexpected update payload: %v", updates)
	}

	// The audit is unknown in the plan (it is server-managed), so the state
	// must not be left with an unknown value.
	var got res.CatalogResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if got.Audit.IsUnknown() {
		t.Fatal("audit must be a known value after apply")
	}
}

// TestCatalogResource_PropertiesEmptyMapStaysEmpty: a configured empty map must
// stay an empty (known) map, and reserved server keys must not be added —
// otherwise Terraform reports "Provider produced inconsistent result".
func TestCatalogResource_PropertiesEmptyMapStaysEmpty(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{"code":0,"catalog":{"name":"my_hive_catalog","type":"relational","provider":"hive","properties":{"in-use":"true"}}}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.New()
	r.(*res.CatalogResource).SetClient(c)

	plan := baseModel()
	plan.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{})
	plan.Comment = types.StringValue("This is my hive catalog")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state res.CatalogResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.Properties.IsNull() || state.Properties.IsUnknown() {
		t.Fatalf("a configured empty map must stay a known empty map, got %v", state.Properties)
	}
	if len(state.Properties.Elements()) != 0 {
		t.Fatalf("expected no properties, got %v", state.Properties)
	}
}

// TestCatalogResource_RejectsReservedProperty: 'in-use' is managed by Gravitino
// via PATCH and cannot be configured as a catalog property; otherwise the value
// the server returns would differ from the configuration.
func TestCatalogResource_RejectsReservedProperty(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)

	propsAttr := s.Attributes["properties"].(schema.MapAttribute)
	req := validator.MapRequest{
		ConfigValue: types.MapValueMust(types.StringType, map[string]attr.Value{"in-use": types.StringValue("true")}),
		Path:        path.Root("properties"),
	}
	resp := &validator.MapResponse{}
	for _, v := range propsAttr.Validators {
		v.ValidateMap(ctx, req, resp)
	}
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the reserved 'in-use' key to be rejected")
	}

	okResp := &validator.MapResponse{}
	okReq := validator.MapRequest{
		ConfigValue: types.MapValueMust(types.StringType, map[string]attr.Value{"metastore.uris": types.StringValue("thrift://127.0.0.1:9083")}),
		Path:        path.Root("properties"),
	}
	for _, v := range propsAttr.Validators {
		v.ValidateMap(ctx, okReq, okResp)
	}
	if okResp.Diagnostics.HasError() {
		t.Fatalf("a regular property must be accepted, got %v", okResp.Diagnostics)
	}
}

func TestCatalogResource_Delete(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)

	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Query().Get("force") != "true" {
			t.Errorf("expected force=true, got %s", r.URL.Query().Get("force"))
		}
		json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.New()
	r.(*res.CatalogResource).SetClient(c)

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_hive_catalog")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !called {
		t.Fatal("delete was not called")
	}
}

func TestCatalogResource_DeleteNotFoundIsSuccess(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchCatalogError)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.New()
	r.(*res.CatalogResource).SetClient(c)

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_hive_catalog")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("deleting an already deleted catalog must succeed, got: %v", resp.Diagnostics)
	}
}

func TestCatalogResource_ImportState(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)
	r := res.New().(resource.ResourceWithImportState)

	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, baseModel())}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "my_metalake.my_catalog"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state res.CatalogResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.Metalake.ValueString() != "my_metalake" || state.Name.ValueString() != "my_catalog" {
		t.Fatalf("unexpected import result: metalake=%s name=%s", state.Metalake.ValueString(), state.Name.ValueString())
	}
	if state.ID.ValueString() != "my_metalake.my_catalog" {
		t.Fatalf("unexpected id: %s", state.ID.ValueString())
	}
}

func TestCatalogResource_ImportState_Invalid(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)
	r := res.New().(resource.ResourceWithImportState)

	for _, id := range []string{"no_dot_here", "trailing.", ".leading", ""} {
		resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, baseModel())}}
		r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected an error for invalid import ID %q", id)
		}
	}
}

func assertJSONEqual(t *testing.T, want, got map[string]any) {
	t.Helper()
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("unexpected payload:\n want %s\n  got %s", wantJSON, gotJSON)
	}
}

func assertUpdatesEqual(t *testing.T, want, got []map[string]any) {
	t.Helper()
	canon := func(updates []map[string]any) []string {
		out := make([]string, 0, len(updates))
		for _, u := range updates {
			b, _ := json.Marshal(u)
			out = append(out, string(b))
		}
		sort.Strings(out)
		return out
	}
	wantCanon, gotCanon := canon(want), canon(got)
	if strings.Join(wantCanon, "\n") != strings.Join(gotCanon, "\n") {
		t.Fatalf("unexpected update payload:\n want %s\n  got %s", strings.Join(wantCanon, "\n"), strings.Join(gotCanon, "\n"))
	}
}

// TestCatalogResource_ModifyPlan: an in-place rename must mark the id as
// unknown (the id embeds the name), otherwise the applied id differs from the
// planned one ("Provider produced inconsistent result after apply").
func TestCatalogResource_ModifyPlan(t *testing.T) {
	ctx := context.Background()
	s := catalogSchema(t)
	r := res.New().(resource.ResourceWithModifyPlan)

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_hive_catalog")

	rename := state
	rename.Name = types.StringValue("my_hive_catalog_new")

	for _, tc := range []struct {
		name        string
		plan        res.CatalogResourceModel
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

			var got res.CatalogResourceModel
			if diags := resp.Plan.Get(ctx, &got); diags.HasError() {
				t.Fatalf("failed to read plan: %v", diags)
			}
			if got.ID.IsUnknown() != tc.wantUnknown {
				t.Fatalf("expected id unknown=%v, got %v", tc.wantUnknown, got.ID)
			}
		})
	}
}
