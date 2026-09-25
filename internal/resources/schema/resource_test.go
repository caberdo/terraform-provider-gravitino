package schema_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	resourceschema "github.com/gravitino/terraform-provider-gravitino/internal/resources/schema"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The payloads below are copied literally from the v1.3.0 OpenAPI spec
// (docs/open-api/schemas.yaml#/components/examples).

const (
	specSchemaCreateRequestBody = `{
		"name": "my_hive_schema",
		"comment": "This is my Hive schema",
		"properties": {
			"location": "/user/hive/warehouse",
			"key1": "value1",
			"key2": "value2"
		}
	}`

	specSchemaResponseBody = `{
		"code": 0,
		"schema": {
			"name": "my_hive_schema",
			"comment": "This is my Hive schema",
			"properties": {
				"key1": "value1",
				"key2": "value2",
				"location": "hdfs://0.0.0.0:9000/user/hive/warehouse"
			},
			"audit": {
				"creator": "gravitino",
				"createTime": "2023-12-08T08:37:43.531Z"
			}
		}
	}`

	specSchemaResponseUpdatedAuditBody = `{
		"code": 0,
		"schema": {
			"name": "my_hive_schema",
			"comment": "This is my Hive schema",
			"properties": {"key1": "value1_new", "key3": "value3"},
			"audit": {
				"creator": "gravitino",
				"createTime": "2023-12-08T08:37:43.531Z",
				"lastModifier": "gravitino",
				"lastModifiedTime": "2023-12-09T09:00:00Z"
			}
		}
	}`

	specNoSuchSchemaBody = `{
		"code": 1003,
		"type": "NoSuchSchemaException",
		"message": "Failed to operate schema(s) [my_hive_schema1] operation [LOAD] under catalog [my_hive_catalog], reason [NoSuchSchemaException]",
		"stack": [
			"org.apache.gravitino.exceptions.NoSuchSchemaException: Hive schema (database) does not exist: my_hive_schema1 in Hive Metastore",
			"..."
		]
	}`
)

func unitSchemaResource(t *testing.T, serverURL string) *resourceschema.SchemaResource {
	t.Helper()

	c, err := client.New(serverURL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	r := resourceschema.NewSchemaResource().(*resourceschema.SchemaResource)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("configure: %v", resp.Diagnostics)
	}
	return r
}

func unitSchemaSchema(t *testing.T, r resource.Resource) resourceSchema.Schema {
	t.Helper()

	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func unitAuditTypes(t *testing.T, s resourceSchema.Schema) map[string]attr.Type {
	t.Helper()

	audit, ok := s.Attributes["audit"].(resourceSchema.ObjectAttribute)
	if !ok {
		t.Fatalf("audit is not an object attribute: %#v", s.Attributes["audit"])
	}
	return audit.AttributeTypes
}

// unitSchemaModel builds a model whose audit is null and whose name/comment/
// properties are set as requested.
func unitSchemaModel(t *testing.T, s resourceSchema.Schema, comment string, properties map[string]string) resourceschema.SchemaResourceModel {
	t.Helper()

	m := resourceschema.SchemaResourceModel{
		ID:         types.StringValue("my_metalake.my_catalog.my_hive_schema"),
		Metalake:   types.StringValue("my_metalake"),
		Catalog:    types.StringValue("my_catalog"),
		Name:       types.StringValue("my_hive_schema"),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(unitAuditTypes(t, s)),
	}
	if comment != "" {
		m.Comment = types.StringValue(comment)
	}
	if properties != nil {
		values := map[string]attr.Value{}
		for k, v := range properties {
			values[k] = types.StringValue(v)
		}
		m.Properties = types.MapValueMust(types.StringType, values)
	}
	return m
}

func unitSchemaValue(t *testing.T, s resourceSchema.Schema, model resourceschema.SchemaResourceModel) tftypes.Value {
	t.Helper()

	ctx := context.Background()
	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("object value: %v", diags)
	}
	val, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("terraform value: %v", err)
	}
	return val
}

func unitSchemaPlan(t *testing.T, s resourceSchema.Schema, model resourceschema.SchemaResourceModel) tfsdk.Plan {
	t.Helper()
	return tfsdk.Plan{Schema: s, Raw: unitSchemaValue(t, s, model)}
}

func unitSchemaState(t *testing.T, s resourceSchema.Schema, model resourceschema.SchemaResourceModel) tfsdk.State {
	t.Helper()
	return tfsdk.State{Schema: s, Raw: unitSchemaValue(t, s, model)}
}

func unitSchemaStateFrom(t *testing.T, resp *resource.ReadResponse) resourceschema.SchemaResourceModel {
	t.Helper()

	var state resourceschema.SchemaResourceModel
	if d := resp.State.Get(context.Background(), &state); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	return state
}

func unitAuditString(t *testing.T, audit types.Object, name string) string {
	t.Helper()

	value, ok := audit.Attributes()[name].(types.String)
	if !ok {
		t.Fatalf("audit.%s is not a string attribute", name)
	}
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}

// unitAssertJSONEqual compares two JSON documents regardless of key order.
func unitAssertJSONEqual(t *testing.T, want, got string) {
	t.Helper()

	var wantVal, gotVal any
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("invalid want json: %v", err)
	}
	if err := json.Unmarshal([]byte(got), &gotVal); err != nil {
		t.Fatalf("invalid got json %q: %v", got, err)
	}
	if !reflect.DeepEqual(wantVal, gotVal) {
		t.Errorf("unexpected JSON payload:\n want: %s\n  got: %s", want, got)
	}
}

// unitAssertJSONUpdatesEqual compares an `updates` array as an unordered set.
func unitAssertJSONUpdatesEqual(t *testing.T, want []string, got string) {
	t.Helper()

	var envelope struct {
		Updates []any `json:"updates"`
	}
	if err := json.Unmarshal([]byte(got), &envelope); err != nil {
		t.Fatalf("invalid updates json %q: %v", got, err)
	}

	canonical := func(items []any) []string {
		out := make([]string, 0, len(items))
		for _, item := range items {
			raw, err := json.Marshal(item)
			if err != nil {
				t.Fatalf("marshal item: %v", err)
			}
			out = append(out, string(raw))
		}
		sort.Strings(out)
		return out
	}

	wantItems := make([]any, 0, len(want))
	for _, w := range want {
		var item any
		if err := json.Unmarshal([]byte(w), &item); err != nil {
			t.Fatalf("invalid want update %q: %v", w, err)
		}
		wantItems = append(wantItems, item)
	}

	if !reflect.DeepEqual(canonical(wantItems), canonical(envelope.Updates)) {
		t.Errorf("unexpected updates payload:\n want: %v\n  got: %s", want, got)
	}
}

// unitStringRequiresReplace runs an attribute's plan modifiers exactly the way the
// framework does (in registration order) and reports whether they require a
// replacement.
func unitStringRequiresReplace(t *testing.T, attr resourceSchema.StringAttribute, state, plan string) bool {
	t.Helper()

	ctx := context.Background()
	req := planmodifier.StringRequest{
		State:       tfsdk.State{Raw: tftypes.NewValue(tftypes.String, state)},
		Plan:        tfsdk.Plan{Raw: tftypes.NewValue(tftypes.String, plan)},
		StateValue:  types.StringValue(state),
		PlanValue:   types.StringValue(plan),
		ConfigValue: types.StringValue(plan),
	}
	resp := &planmodifier.StringResponse{}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyString(ctx, req, resp)
	}
	return resp.RequiresReplace
}

func TestSchemaResourceMetadata(t *testing.T) {
	r := resourceschema.NewSchemaResource()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_schema" {
		t.Errorf("expected type name gravitino_schema, got %s", resp.TypeName)
	}
}

func TestSchemaResourceSchema(t *testing.T) {
	s := unitSchemaSchema(t, resourceschema.NewSchemaResource())

	idAttr, ok := s.Attributes["id"].(resourceSchema.StringAttribute)
	if !ok || !idAttr.Computed || len(idAttr.PlanModifiers) == 0 {
		t.Fatalf("id must be a computed string attribute with a plan modifier, got %#v", s.Attributes["id"])
	}

	propertiesAttr, ok := s.Attributes["properties"].(resourceSchema.MapAttribute)
	if !ok {
		t.Fatalf("properties must be a map attribute, got %#v", s.Attributes["properties"])
	}
	if !propertiesAttr.Optional || propertiesAttr.Computed {
		t.Errorf("properties must be optional and not computed, got optional=%v computed=%v", propertiesAttr.Optional, propertiesAttr.Computed)
	}
	if propertiesAttr.ElementType != types.StringType {
		t.Errorf("properties element type = %v, want string", propertiesAttr.ElementType)
	}

	auditAttr, ok := s.Attributes["audit"].(resourceSchema.ObjectAttribute)
	if !ok || !auditAttr.Computed {
		t.Fatalf("audit must be a computed object attribute, got %#v", s.Attributes["audit"])
	}
	// The server rewrites last_modifier/last_modified_time on every in-place
	// update, so the audit must stay unknown in the plan and be written from the
	// response; a UseStateForUnknown modifier would make Terraform reject the
	// applied state.
	if len(auditAttr.PlanModifiers) != 0 {
		t.Errorf("audit must not have plan modifiers, got %d", len(auditAttr.PlanModifiers))
	}
}

// The v1.3.0 spec's SchemaUpdateRequest oneOf only allows setProperty and
// removeProperty: name and comment are not updateable, so every other attribute
// has to force a replacement.
func TestSchemaResourceRequiresReplace(t *testing.T) {
	s := unitSchemaSchema(t, resourceschema.NewSchemaResource())

	for _, name := range []string{"metalake", "catalog", "name", "comment"} {
		attr, ok := s.Attributes[name].(resourceSchema.StringAttribute)
		if !ok {
			t.Fatalf("%s must be a string attribute, got %#v", name, s.Attributes[name])
		}
		if !unitStringRequiresReplace(t, attr, "old_value", "new_value") {
			t.Errorf("changing %s must force replacement", name)
		}
		if unitStringRequiresReplace(t, attr, "same_value", "same_value") {
			t.Errorf("an unchanged %s must not force replacement", name)
		}
	}

	if _, exists := s.Attributes["storage_location"]; exists {
		t.Error("a Gravitino schema has no storage_location attribute")
	}
}

func TestSchemaResourceCreateSendsSpecPayload(t *testing.T) {
	var mu sync.Mutex
	var gotMethod, gotPath, gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)

		mu.Lock()
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody = string(raw)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(specSchemaResponseBody))
	}))
	defer server.Close()

	r := unitSchemaResource(t, server.URL)
	s := unitSchemaSchema(t, r)

	plan := unitSchemaModel(t, s, "This is my Hive schema", map[string]string{
		"location": "/user/hive/warehouse",
		"key1":     "value1",
		"key2":     "value2",
	})
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(context.Background(), resource.CreateRequest{Plan: unitSchemaPlan(t, s, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/api/metalakes/my_metalake/catalogs/my_catalog/schemas" {
		t.Errorf("path = %s", gotPath)
	}
	unitAssertJSONEqual(t, specSchemaCreateRequestBody, gotBody)
}

func TestSchemaResourceCreateWritesKnownState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(specSchemaResponseBody))
	}))
	defer server.Close()

	r := unitSchemaResource(t, server.URL)
	s := unitSchemaSchema(t, r)

	plan := unitSchemaModel(t, s, "This is my Hive schema", map[string]string{
		"location": "/user/hive/warehouse",
		"key1":     "value1",
		"key2":     "value2",
	})
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(context.Background(), resource.CreateRequest{Plan: unitSchemaPlan(t, s, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	var got resourceschema.SchemaResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("state: %v", d)
	}

	if got.ID.ValueString() != "my_metalake.my_catalog.my_hive_schema" {
		t.Errorf("id = %q", got.ID.ValueString())
	}
	if got.Name.ValueString() != "my_hive_schema" {
		t.Errorf("name = %q", got.Name.ValueString())
	}
	if got.Comment.ValueString() != "This is my Hive schema" {
		t.Errorf("comment = %q", got.Comment.ValueString())
	}
	// The configured property set is written back verbatim: the spec's response
	// example reports a normalised "location" value which must not replace a plan
	// value that Terraform already knows.
	wantProps := map[string]string{
		"location": "/user/hive/warehouse",
		"key1":     "value1",
		"key2":     "value2",
	}
	gotProps := map[string]string{}
	if d := got.Properties.ElementsAs(context.Background(), &gotProps, false); d.HasError() {
		t.Fatalf("properties: %v", d)
	}
	if !reflect.DeepEqual(wantProps, gotProps) {
		t.Errorf("properties = %v, want %v", gotProps, wantProps)
	}
	if unitAuditString(t, got.Audit, "creator") != "gravitino" {
		t.Errorf("audit.creator = %q, want gravitino", unitAuditString(t, got.Audit, "creator"))
	}
	if unitAuditString(t, got.Audit, "create_time") != "2023-12-08T08:37:43Z" {
		t.Errorf("audit.create_time = %q", unitAuditString(t, got.Audit, "create_time"))
	}
}

func TestSchemaResourceUpdateSendsSpecPropertyUpdates(t *testing.T) {
	var mu sync.Mutex
	var gotMethod, gotPath, gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)

		mu.Lock()
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody = string(raw)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(specSchemaResponseUpdatedAuditBody))
	}))
	defer server.Close()

	r := unitSchemaResource(t, server.URL)
	s := unitSchemaSchema(t, r)

	stateModel := unitSchemaModel(t, s, "This is my Hive schema", map[string]string{
		"key1": "value1",
		"key2": "value2",
	})
	plan := unitSchemaModel(t, s, "This is my Hive schema", map[string]string{
		"key1": "value1_new",
		"key3": "value3",
	})
	plan.Audit = types.ObjectUnknown(unitAuditTypes(t, s))

	req := resource.UpdateRequest{
		Plan:  unitSchemaPlan(t, s, plan),
		State: unitSchemaState(t, s, stateModel),
	}
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}

	r.Update(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/api/metalakes/my_metalake/catalogs/my_catalog/schemas/my_hive_schema" {
		t.Errorf("path = %s", gotPath)
	}
	// Both update requests are the spec's own examples.
	unitAssertJSONUpdatesEqual(t, []string{
		`{"@type": "setProperty", "property": "key1", "value": "value1_new"}`,
		`{"@type": "removeProperty", "property": "key2"}`,
		`{"@type": "setProperty", "property": "key3", "value": "value3"}`,
	}, gotBody)

	// The refreshed audit from the response must land in state: audit has no
	// UseStateForUnknown modifier exactly because it changes on every update.
	var got resourceschema.SchemaResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("state: %v", d)
	}
	if unitAuditString(t, got.Audit, "last_modified_time") != "2023-12-09T09:00:00Z" {
		t.Errorf("audit.last_modified_time = %q, want the refreshed server value", unitAuditString(t, got.Audit, "last_modified_time"))
	}
}

func TestSchemaResourceUpdateWithoutChangesSkipsApi(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(specSchemaResponseBody))
	}))
	defer server.Close()

	r := unitSchemaResource(t, server.URL)
	s := unitSchemaSchema(t, r)

	stateModel := unitSchemaModel(t, s, "This is my Hive schema", map[string]string{"key1": "value1"})
	auditObj, d := types.ObjectValue(unitAuditTypes(t, s), map[string]attr.Value{
		"creator":            types.StringValue("gravitino"),
		"create_time":        types.StringValue("2023-12-08T08:37:43.531Z"),
		"last_modifier":      types.StringNull(),
		"last_modified_time": types.StringNull(),
	})
	if d.HasError() {
		t.Fatalf("audit object: %v", d)
	}
	stateModel.Audit = auditObj

	// Nothing changed, so audit (and id) stay unknown in the plan.
	plan := stateModel
	plan.Audit = types.ObjectUnknown(unitAuditTypes(t, s))
	plan.ID = types.StringUnknown()

	req := resource.UpdateRequest{
		Plan:  unitSchemaPlan(t, s, plan),
		State: unitSchemaState(t, s, stateModel),
	}
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}

	r.Update(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}
	if requests != 0 {
		t.Errorf("expected no API request for an unchanged plan, got %d", requests)
	}

	var got resourceschema.SchemaResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("state: %v", d)
	}
	if got.Audit.IsUnknown() || got.Audit.IsNull() {
		t.Fatalf("audit must be a known value after an update without changes, got %v", got.Audit)
	}
	if got.ID.ValueString() != "my_metalake.my_catalog.my_hive_schema" {
		t.Errorf("id = %q", got.ID.ValueString())
	}
}

func TestSchemaResourceReadAdoptsServerValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(specSchemaResponseBody))
	}))
	defer server.Close()

	r := unitSchemaResource(t, server.URL)
	s := unitSchemaSchema(t, r)

	req := resource.ReadRequest{State: unitSchemaState(t, s, unitSchemaModel(t, s, "", nil))}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s}}

	r.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	state := unitSchemaStateFrom(t, resp)
	if state.Name.ValueString() != "my_hive_schema" {
		t.Errorf("name = %q", state.Name.ValueString())
	}
	if state.Comment.ValueString() != "This is my Hive schema" {
		t.Errorf("comment = %q", state.Comment.ValueString())
	}
	gotProps := map[string]string{}
	if d := state.Properties.ElementsAs(context.Background(), &gotProps, false); d.HasError() {
		t.Fatalf("properties: %v", d)
	}
	if gotProps["location"] != "hdfs://0.0.0.0:9000/user/hive/warehouse" {
		t.Errorf("properties = %v, want the server values", gotProps)
	}
	if unitAuditString(t, state.Audit, "creator") != "gravitino" {
		t.Errorf("audit.creator = %q", unitAuditString(t, state.Audit, "creator"))
	}
}

func TestSchemaResourceReadNotFoundRemovesState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(specNoSuchSchemaBody))
	}))
	defer server.Close()

	r := unitSchemaResource(t, server.URL)
	s := unitSchemaSchema(t, r)

	req := resource.ReadRequest{State: unitSchemaState(t, s, unitSchemaModel(t, s, "", nil))}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s}}

	r.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a 404 (NoSuchSchemaException) must remove the schema from state")
	}
}

func TestSchemaResourceDelete(t *testing.T) {
	var mu sync.Mutex
	var gotMethod, gotRawQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod = r.Method
		gotRawQuery = r.URL.RawQuery
		mu.Unlock()
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code": 0, "dropped": true}`))
	}))
	defer server.Close()

	r := unitSchemaResource(t, server.URL)
	s := unitSchemaSchema(t, r)

	req := resource.DeleteRequest{State: unitSchemaState(t, s, unitSchemaModel(t, s, "", nil))}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}

	r.Delete(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", resp.Diagnostics)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	// dropSchema declares no force query parameter in the v1.3.0 spec.
	if gotRawQuery != "" {
		t.Errorf("delete query = %q, want empty", gotRawQuery)
	}
}

func TestSchemaResourceDeleteNotFoundIsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(specNoSuchSchemaBody))
	}))
	defer server.Close()

	r := unitSchemaResource(t, server.URL)
	s := unitSchemaSchema(t, r)

	req := resource.DeleteRequest{State: unitSchemaState(t, s, unitSchemaModel(t, s, "", nil))}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}

	r.Delete(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 on delete must be idempotent success, got: %v", resp.Diagnostics)
	}
}

func TestSchemaResourceImportState(t *testing.T) {
	r := resourceschema.NewSchemaResource().(resource.ResourceWithImportState)
	s := unitSchemaSchema(t, r.(resource.Resource))

	ctx := context.Background()
	req := resource.ImportStateRequest{ID: "my_metalake.my_catalog.my_hive_schema"}
	resp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)},
	}

	r.ImportState(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	var state resourceschema.SchemaResourceModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("state: %v", d)
	}
	if state.Metalake.ValueString() != "my_metalake" ||
		state.Catalog.ValueString() != "my_catalog" ||
		state.Name.ValueString() != "my_hive_schema" ||
		state.ID.ValueString() != "my_metalake.my_catalog.my_hive_schema" {
		t.Errorf("unexpected imported state: %+v", state)
	}
}

func TestSchemaResourceImportStateInvalid(t *testing.T) {
	r := resourceschema.NewSchemaResource().(resource.ResourceWithImportState)
	s := unitSchemaSchema(t, r.(resource.Resource))

	for _, id := range []string{
		"",
		"my_metalake",
		"my_metalake.my_catalog",
		"my_metalake..my_hive_schema",
		".my_catalog.my_hive_schema",
		"my_metalake.my_catalog.",
	} {
		t.Run(id, func(t *testing.T) {
			ctx := context.Background()
			resp := &resource.ImportStateResponse{
				State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)},
			}
			r.ImportState(context.Background(), resource.ImportStateRequest{ID: id}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error for import id %q", id)
			}
		})
	}
}

// The model must only be able to produce the update requests the spec allows:
// the measured 1.3.0 server rejects any other @type with HTTP 400
// "Malformed json request ... known type ids = [removeProperty, setProperty]".
func TestSchemaUpdateRequestPayloads(t *testing.T) {
	raw, err := json.Marshal(models.NewSetSchemaPropertyRequest("key1", "value1_new"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	unitAssertJSONEqual(t, `{"@type": "setProperty", "property": "key1", "value": "value1_new"}`, string(raw))

	raw, err = json.Marshal(models.NewRemoveSchemaPropertyRequest("key2"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	unitAssertJSONEqual(t, `{"@type": "removeProperty", "property": "key2"}`, string(raw))
}
