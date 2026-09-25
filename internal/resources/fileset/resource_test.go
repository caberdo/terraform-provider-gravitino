package fileset_test

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
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/fileset"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The payloads below are copied literally from the v1.3.0 OpenAPI spec
// (docs/open-api/filesets.yaml#/components/examples).

const (
	specFilesetCreateRequestBody = `{
		"name": "fileset1",
		"type": "managed",
		"comment": "This is a comment",
		"storageLocation": "s3://bucket/path",
		"properties": {
			"key1": "value1",
			"key2": "value2"
		}
	}`

	specFilesetResponseBody = `{
		"code": 0,
		"fileset": {
			"name": "fileset1",
			"type": "managed",
			"comment": "This is a comment",
			"storageLocation": "hdfs://host/user/s_fileset/schema/fileset1",
			"properties": {
				"key1": "value1",
				"key2": "value2"
			}
		}
	}`

	// Same shape as the spec's FilesetResponse example; only the name is the one
	// the spec's own RenameFilesetRequest example renames to.
	specFilesetRenamedResponseBody = `{
		"code": 0,
		"fileset": {
			"name": "newName",
			"type": "managed",
			"comment": "new comment",
			"storageLocation": "hdfs://host/user/s_fileset/schema/fileset1",
			"properties": {
				"key": "value"
			}
		}
	}`

	specNoSuchFilesetBody = `{
		"code": 1003,
		"type": "NoSuchFilesetException",
		"message": "Fileset does not exist",
		"stack": [
			"java.lang.NoSuchFilesetException: Fileset does not exist"
		]
	}`
)

func unitFilesetResource(t *testing.T, serverURL string) *res.FilesetResource {
	t.Helper()

	c, err := client.New(serverURL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	r := res.New().(*res.FilesetResource)
	r.SetClient(c)
	return r
}

func unitFilesetSchema(t *testing.T, r resource.Resource) resourceSchema.Schema {
	t.Helper()

	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func unitFilesetModel(t *testing.T, s resourceSchema.Schema) res.FilesetResourceModel {
	t.Helper()

	return res.FilesetResourceModel{
		ID:              types.StringValue("my_metalake.my_catalog.my_schema.fileset1"),
		Metalake:        types.StringValue("my_metalake"),
		Catalog:         types.StringValue("my_catalog"),
		Schema:          types.StringValue("my_schema"),
		Name:            types.StringValue("fileset1"),
		Comment:         types.StringNull(),
		Type:            types.StringNull(),
		StorageLocation: types.StringNull(),
		Properties:      types.MapNull(types.StringType),
		Audit:           types.ObjectNull(res.AuditAttrTypes),
	}
}

func unitFilesetValue(t *testing.T, s resourceSchema.Schema, model res.FilesetResourceModel) tftypes.Value {
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

func unitFilesetPlan(t *testing.T, s resourceSchema.Schema, model res.FilesetResourceModel) tfsdk.Plan {
	t.Helper()
	return tfsdk.Plan{Schema: s, Raw: unitFilesetValue(t, s, model)}
}

func unitFilesetState(t *testing.T, s resourceSchema.Schema, model res.FilesetResourceModel) tfsdk.State {
	t.Helper()
	return tfsdk.State{Schema: s, Raw: unitFilesetValue(t, s, model)}
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

// unitStringPlanModifiers runs a string attribute's plan modifiers the way the
// framework does (in registration order) and reports the resulting plan value and
// replacement decision. key is "state" or "plan".
func unitStringPlanModifiers(t *testing.T, attr resourceSchema.StringAttribute, state, plan string, stateUnknown, planUnknown bool) (types.String, bool) {
	t.Helper()

	ctx := context.Background()
	stateValue := types.StringValue(state)
	if stateUnknown {
		stateValue = types.StringUnknown()
	}
	planValue := types.StringValue(plan)
	if planUnknown {
		planValue = types.StringUnknown()
	}

	req := planmodifier.StringRequest{
		State:       tfsdk.State{Raw: unitRawString(state, stateUnknown)},
		Plan:        tfsdk.Plan{Raw: unitRawString(plan, planUnknown)},
		StateValue:  stateValue,
		PlanValue:   planValue,
		ConfigValue: types.StringNull(),
	}
	resp := &planmodifier.StringResponse{PlanValue: planValue}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyString(ctx, req, resp)
		// The framework feeds the modified plan value into the next modifier.
		req.PlanValue = resp.PlanValue
		if resp.PlanValue.IsUnknown() {
			req.Plan.Raw = unitRawString("", true)
		} else if !resp.PlanValue.IsNull() {
			req.Plan.Raw = unitRawString(resp.PlanValue.ValueString(), false)
		}
	}
	return resp.PlanValue, resp.RequiresReplace
}

func unitRawString(value string, unknown bool) tftypes.Value {
	if unknown {
		return tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	}
	return tftypes.NewValue(tftypes.String, value)
}

func TestFilesetResourceMetadata(t *testing.T) {
	r := res.New()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_fileset" {
		t.Errorf("expected gravitino_fileset, got %s", resp.TypeName)
	}
}

func TestFilesetResourceSchema(t *testing.T) {
	s := unitFilesetSchema(t, res.New())

	idAttr, ok := s.Attributes["id"].(resourceSchema.StringAttribute)
	if !ok || !idAttr.Computed {
		t.Fatalf("id must be a computed string attribute, got %#v", s.Attributes["id"])
	}
	// The id embeds the fileset name, which the rename update request changes in
	// place: pinning the id with UseStateForUnknown would make Terraform reject
	// the renamed id after apply (measured).
	if len(idAttr.PlanModifiers) != 0 {
		t.Errorf("id must not have plan modifiers, got %d", len(idAttr.PlanModifiers))
	}

	typeAttr, ok := s.Attributes["type"].(resourceSchema.StringAttribute)
	if !ok || !typeAttr.Optional || !typeAttr.Computed {
		t.Fatalf("type must be an optional+computed string attribute, got %#v", s.Attributes["type"])
	}
	if len(typeAttr.Validators) == 0 {
		t.Error("type must validate its enum values")
	}
	// UseStateForUnknown must run before RequiresReplace, otherwise an omitted
	// Optional+Computed attribute is unknown in the plan and every update is
	// planned as a replacement (measured with terraform 1.14).
	if len(typeAttr.PlanModifiers) != 2 {
		t.Fatalf("type must have two plan modifiers, got %d", len(typeAttr.PlanModifiers))
	}

	storageAttr, ok := s.Attributes["storage_location"].(resourceSchema.StringAttribute)
	if !ok || !storageAttr.Optional || !storageAttr.Computed {
		t.Fatalf("storage_location must be an optional+computed string attribute, got %#v", s.Attributes["storage_location"])
	}
	if len(storageAttr.PlanModifiers) != 2 {
		t.Fatalf("storage_location must have two plan modifiers, got %d", len(storageAttr.PlanModifiers))
	}

	propertiesAttr, ok := s.Attributes["properties"].(resourceSchema.MapAttribute)
	if !ok || !propertiesAttr.Optional || !propertiesAttr.Computed {
		t.Fatalf("properties must be an optional+computed map attribute, got %#v", s.Attributes["properties"])
	}
	if len(propertiesAttr.PlanModifiers) == 0 {
		t.Error("properties needs a UseStateForUnknown plan modifier")
	}

	auditAttr, ok := s.Attributes["audit"].(resourceSchema.ObjectAttribute)
	if !ok || !auditAttr.Computed {
		t.Fatalf("audit must be a computed object attribute, got %#v", s.Attributes["audit"])
	}
	// The server rewrites last_modifier/last_modified_time on every in-place
	// update: a UseStateForUnknown modifier here makes Terraform reject the
	// applied state (measured).
	if len(auditAttr.PlanModifiers) != 0 {
		t.Errorf("audit must not have plan modifiers, got %d", len(auditAttr.PlanModifiers))
	}
}

func TestFilesetResourceRequiresReplace(t *testing.T) {
	s := unitFilesetSchema(t, res.New())

	for _, name := range []string{"metalake", "catalog", "schema", "type", "storage_location"} {
		attr, ok := s.Attributes[name].(resourceSchema.StringAttribute)
		if !ok {
			t.Fatalf("%s must be a string attribute, got %#v", name, s.Attributes[name])
		}

		// A configured change forces a replacement ...
		if _, replace := unitStringPlanModifiers(t, attr, "old_value", "new_value", false, false); !replace {
			t.Errorf("changing %s must force replacement", name)
		}
		// ... an unchanged value does not ...
		if _, replace := unitStringPlanModifiers(t, attr, "same_value", "same_value", false, false); replace {
			t.Errorf("an unchanged %s must not force replacement", name)
		}
		// ... and neither does an omitted Optional+Computed attribute, whose plan
		// value is unknown and resolved from state by UseStateForUnknown.
		if attr.Optional && attr.Computed {
			plan, replace := unitStringPlanModifiers(t, attr, "server_value", "", false, true)
			if replace {
				t.Errorf("%s omitted from the configuration must not force replacement", name)
			}
			if plan.IsUnknown() || plan.ValueString() != "server_value" {
				t.Errorf("%s plan value = %v, want the state value", name, plan)
			}
		}
	}

	// name is updateable in place (rename), so it must not force a replacement.
	nameAttr, ok := s.Attributes["name"].(resourceSchema.StringAttribute)
	if !ok {
		t.Fatalf("name must be a string attribute, got %#v", s.Attributes["name"])
	}
	if len(nameAttr.PlanModifiers) != 0 {
		t.Error("name must not have plan modifiers: renaming is an in-place update")
	}
}

func TestFilesetResourceCreateSendsSpecPayload(t *testing.T) {
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
		_, _ = w.Write([]byte(specFilesetResponseBody))
	}))
	defer server.Close()

	r := unitFilesetResource(t, server.URL)
	s := unitFilesetSchema(t, r)

	plan := unitFilesetModel(t, s)
	plan.Comment = types.StringValue("This is a comment")
	plan.Type = types.StringValue("managed")
	plan.StorageLocation = types.StringValue("s3://bucket/path")
	plan.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{
		"key1": types.StringValue("value1"),
		"key2": types.StringValue("value2"),
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(context.Background(), resource.CreateRequest{Plan: unitFilesetPlan(t, s, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/api/metalakes/my_metalake/catalogs/my_catalog/schemas/my_schema/filesets" {
		t.Errorf("path = %s", gotPath)
	}
	unitAssertJSONEqual(t, specFilesetCreateRequestBody, gotBody)

	var got res.FilesetResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("state: %v", d)
	}
	if got.ID.ValueString() != "my_metalake.my_catalog.my_schema.fileset1" {
		t.Errorf("id = %q", got.ID.ValueString())
	}
	// Gravitino reports the storage location in normalised form; the configured
	// value is what state keeps so that a later refresh cannot plan a replacement.
	if got.StorageLocation.ValueString() != "s3://bucket/path" {
		t.Errorf("storage_location = %q, want the configured value", got.StorageLocation.ValueString())
	}
	gotProps := map[string]string{}
	if d := got.Properties.ElementsAs(context.Background(), &gotProps, false); d.HasError() {
		t.Fatalf("properties: %v", d)
	}
	if !reflect.DeepEqual(gotProps, map[string]string{"key1": "value1", "key2": "value2"}) {
		t.Errorf("properties = %v", gotProps)
	}
}

func TestFilesetResourceCreateWithoutConfiguredComputedAttributes(t *testing.T) {
	var mu sync.Mutex
	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotBody = string(raw)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(specFilesetResponseBody))
	}))
	defer server.Close()

	r := unitFilesetResource(t, server.URL)
	s := unitFilesetSchema(t, r)

	// An omitted Optional+Computed attribute is unknown in the plan; only the
	// required name may be sent.
	plan := unitFilesetModel(t, s)
	plan.Comment = types.StringUnknown()
	plan.Type = types.StringUnknown()
	plan.StorageLocation = types.StringUnknown()
	plan.Properties = types.MapUnknown(types.StringType)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(context.Background(), resource.CreateRequest{Plan: unitFilesetPlan(t, s, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	mu.Lock()
	defer mu.Unlock()
	unitAssertJSONEqual(t, `{"name": "fileset1"}`, gotBody)

	var got res.FilesetResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("state: %v", d)
	}
	// Every computed attribute must be known after apply.
	if got.ID.IsUnknown() || got.Comment.IsUnknown() || got.Type.IsUnknown() ||
		got.StorageLocation.IsUnknown() || got.Properties.IsUnknown() || got.Audit.IsUnknown() {
		t.Fatalf("state must not contain unknown values after apply: %+v", got)
	}
	if got.Comment.ValueString() != "This is a comment" || got.Type.ValueString() != "managed" {
		t.Errorf("server values must fill the unknowns, got comment=%q type=%q", got.Comment.ValueString(), got.Type.ValueString())
	}
	if got.StorageLocation.ValueString() != "hdfs://host/user/s_fileset/schema/fileset1" {
		t.Errorf("storage_location = %q, want the server value", got.StorageLocation.ValueString())
	}
}

func TestFilesetResourceUpdateUsesOldNameAndSpecPayloads(t *testing.T) {
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
		_, _ = w.Write([]byte(specFilesetRenamedResponseBody))
	}))
	defer server.Close()

	r := unitFilesetResource(t, server.URL)
	s := unitFilesetSchema(t, r)

	stateModel := unitFilesetModel(t, s)
	stateModel.Comment = types.StringValue("This is a comment")
	stateModel.Type = types.StringValue("managed")
	stateModel.StorageLocation = types.StringValue("s3://bucket/path")
	stateModel.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{
		"key1": types.StringValue("value1"),
	})

	plan := stateModel
	plan.Name = types.StringValue("newName")
	plan.Comment = types.StringValue("new comment")
	plan.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{
		"key": types.StringValue("value"),
	})
	plan.Audit = types.ObjectUnknown(res.AuditAttrTypes)
	plan.ID = types.StringUnknown()

	req := resource.UpdateRequest{
		Plan:  unitFilesetPlan(t, s, plan),
		State: unitFilesetState(t, s, stateModel),
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
	// A rename must be sent to the OLD name: targeting the new one 404s.
	if gotPath != "/api/metalakes/my_metalake/catalogs/my_catalog/schemas/my_schema/filesets/fileset1" {
		t.Errorf("path = %s, want the previous fileset name", gotPath)
	}
	// All four update request examples are the spec's own examples.
	unitAssertJSONUpdatesEqual(t, []string{
		`{"@type": "rename", "newName": "newName"}`,
		`{"@type": "updateComment", "newComment": "new comment"}`,
		`{"@type": "setProperty", "property": "key", "value": "value"}`,
		`{"@type": "removeProperty", "property": "key1"}`,
	}, gotBody)

	var got res.FilesetResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("state: %v", d)
	}
	if got.Name.ValueString() != "newName" {
		t.Errorf("name = %q, want the renamed fileset", got.Name.ValueString())
	}
	if got.ID.ValueString() != "my_metalake.my_catalog.my_schema.newName" {
		t.Errorf("id = %q, want the renamed compound id", got.ID.ValueString())
	}
	if got.Comment.ValueString() != "new comment" {
		t.Errorf("comment = %q", got.Comment.ValueString())
	}
}

func TestFilesetResourceUpdateRemovesComment(t *testing.T) {
	var mu sync.Mutex
	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotBody = string(raw)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(specFilesetResponseBody))
	}))
	defer server.Close()

	r := unitFilesetResource(t, server.URL)
	s := unitFilesetSchema(t, r)

	stateModel := unitFilesetModel(t, s)
	stateModel.Comment = types.StringValue("This is a comment")
	stateModel.Type = types.StringValue("managed")
	stateModel.StorageLocation = types.StringValue("s3://bucket/path")
	stateModel.Properties = types.MapNull(types.StringType)

	plan := stateModel
	plan.Comment = types.StringValue("")
	plan.Audit = types.ObjectUnknown(res.AuditAttrTypes)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  unitFilesetPlan(t, s, plan),
		State: unitFilesetState(t, s, stateModel),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	mu.Lock()
	defer mu.Unlock()
	// RemoveFilesetCommentRequest; the measured 1.3.0 server accepts it even though
	// the spec's FilesetUpdateRequest discriminator mapping does not list it.
	unitAssertJSONUpdatesEqual(t, []string{`{"@type": "removeComment"}`}, gotBody)
}

func TestFilesetResourceUpdateSkipsUnknownPlanValues(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(specFilesetResponseBody))
	}))
	defer server.Close()

	r := unitFilesetResource(t, server.URL)
	s := unitFilesetSchema(t, r)

	stateModel := unitFilesetModel(t, s)
	stateModel.Comment = types.StringValue("This is a comment")
	stateModel.Type = types.StringValue("managed")
	stateModel.StorageLocation = types.StringValue("s3://bucket/path")
	stateModel.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{
		"key1": types.StringValue("value1"),
	})

	// The configuration omits comment and properties for this plan, so both are
	// unknown. Unknown carries no information: nothing may be sent, and the state
	// must fall back to the prior values instead of sending newComment "" and
	// removeProperty for every property.
	plan := unitFilesetModel(t, s)
	plan.Comment = types.StringUnknown()
	plan.Properties = types.MapUnknown(types.StringType)
	plan.Type = types.StringUnknown()
	plan.StorageLocation = types.StringUnknown()
	plan.Audit = types.ObjectUnknown(res.AuditAttrTypes)
	plan.ID = types.StringUnknown()

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  unitFilesetPlan(t, s, plan),
		State: unitFilesetState(t, s, stateModel),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}
	if requests != 0 {
		t.Errorf("no request may be sent when every plan value is unknown, got %d", requests)
	}

	var got res.FilesetResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("state: %v", d)
	}
	if got.Comment.IsUnknown() || got.Properties.IsUnknown() || got.Audit.IsUnknown() ||
		got.ID.IsUnknown() || got.Type.IsUnknown() || got.StorageLocation.IsUnknown() {
		t.Fatalf("state must not contain unknown values: %+v", got)
	}
	if got.Comment.ValueString() != "This is a comment" {
		t.Errorf("comment = %q, want the prior state value", got.Comment.ValueString())
	}
	props := map[string]string{}
	if d := got.Properties.ElementsAs(context.Background(), &props, false); d.HasError() {
		t.Fatalf("properties: %v", d)
	}
	if !reflect.DeepEqual(props, map[string]string{"key1": "value1"}) {
		t.Errorf("properties = %v, want the prior state values", props)
	}
}

func TestFilesetResourceReadAdoptsServerValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(specFilesetResponseBody))
	}))
	defer server.Close()

	r := unitFilesetResource(t, server.URL)
	s := unitFilesetSchema(t, r)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s}}
	r.Read(context.Background(), resource.ReadRequest{State: unitFilesetState(t, s, unitFilesetModel(t, s))}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got res.FilesetResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("state: %v", d)
	}
	if got.ID.ValueString() != "my_metalake.my_catalog.my_schema.fileset1" {
		t.Errorf("id = %q", got.ID.ValueString())
	}
	if got.Comment.ValueString() != "This is a comment" || got.Type.ValueString() != "managed" {
		t.Errorf("unexpected server values: comment=%q type=%q", got.Comment.ValueString(), got.Type.ValueString())
	}
	props := map[string]string{}
	if d := got.Properties.ElementsAs(context.Background(), &props, false); d.HasError() {
		t.Fatalf("properties: %v", d)
	}
	if !reflect.DeepEqual(props, map[string]string{"key1": "value1", "key2": "value2"}) {
		t.Errorf("properties = %v", props)
	}
}

func TestFilesetResourceReadDropsServerManagedProperties(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		// A created fileset always carries default-location-name (measured).
		_, _ = w.Write([]byte(`{"code": 0, "fileset": {
			"name": "fileset1",
			"type": "managed",
			"storageLocation": "file:/tmp/fileset1",
			"properties": {"key1": "value1", "default-location-name": "unknown"}
		}}`))
	}))
	defer server.Close()

	r := unitFilesetResource(t, server.URL)
	s := unitFilesetSchema(t, r)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s}}
	r.Read(context.Background(), resource.ReadRequest{State: unitFilesetState(t, s, unitFilesetModel(t, s))}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got res.FilesetResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("state: %v", d)
	}
	props := map[string]string{}
	if d := got.Properties.ElementsAs(context.Background(), &props, false); d.HasError() {
		t.Fatalf("properties: %v", d)
	}
	if !reflect.DeepEqual(props, map[string]string{"key1": "value1"}) {
		t.Errorf("properties = %v, want the configured keys only", props)
	}
}

func TestFilesetResourceReadKeepsConfiguredStorageLocation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		// Gravitino normalises file:///path to file:/path (measured).
		_, _ = w.Write([]byte(`{"code": 0, "fileset": {
			"name": "fileset1",
			"type": "managed",
			"storageLocation": "file:/tmp/fileset1",
			"properties": {}
		}}`))
	}))
	defer server.Close()

	r := unitFilesetResource(t, server.URL)
	s := unitFilesetSchema(t, r)

	stateModel := unitFilesetModel(t, s)
	stateModel.StorageLocation = types.StringValue("file:///tmp/fileset1")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s}}
	r.Read(context.Background(), resource.ReadRequest{State: unitFilesetState(t, s, stateModel)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got res.FilesetResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("state: %v", d)
	}
	// Gravitino cannot change the storage location of an existing fileset, so a
	// differing echo can only be normalisation; adopting it would make Terraform
	// plan a replacement on every run.
	if got.StorageLocation.ValueString() != "file:///tmp/fileset1" {
		t.Errorf("storage_location = %q, want the configured value", got.StorageLocation.ValueString())
	}
}

func TestFilesetResourceReadNotFoundRemovesState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(specNoSuchFilesetBody))
	}))
	defer server.Close()

	r := unitFilesetResource(t, server.URL)
	s := unitFilesetSchema(t, r)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s}}
	r.Read(context.Background(), resource.ReadRequest{State: unitFilesetState(t, s, unitFilesetModel(t, s))}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a 404 (NoSuchFilesetException) must remove the fileset from state")
	}
}

func TestFilesetResourceDelete(t *testing.T) {
	var mu sync.Mutex
	var gotMethod, gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod = r.Method
		gotPath = r.URL.Path
		mu.Unlock()
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code": 0, "dropped": true}`))
	}))
	defer server.Close()

	r := unitFilesetResource(t, server.URL)
	s := unitFilesetSchema(t, r)

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(context.Background(), resource.DeleteRequest{State: unitFilesetState(t, s, unitFilesetModel(t, s))}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", resp.Diagnostics)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if gotPath != "/api/metalakes/my_metalake/catalogs/my_catalog/schemas/my_schema/filesets/fileset1" {
		t.Errorf("path = %s", gotPath)
	}
}

func TestFilesetResourceDeleteNotFoundIsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(specNoSuchFilesetBody))
	}))
	defer server.Close()

	r := unitFilesetResource(t, server.URL)
	s := unitFilesetSchema(t, r)

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(context.Background(), resource.DeleteRequest{State: unitFilesetState(t, s, unitFilesetModel(t, s))}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 on delete must be idempotent success, got: %v", resp.Diagnostics)
	}
}

func TestFilesetResourceImportState(t *testing.T) {
	r := res.New().(resource.ResourceWithImportState)
	s := unitFilesetSchema(t, r.(resource.Resource))

	ctx := context.Background()
	req := resource.ImportStateRequest{ID: "my_metalake.my_catalog.my_schema.fileset1"}
	resp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)},
	}

	r.ImportState(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	var state res.FilesetResourceModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("state: %v", d)
	}
	if state.Metalake.ValueString() != "my_metalake" ||
		state.Catalog.ValueString() != "my_catalog" ||
		state.Schema.ValueString() != "my_schema" ||
		state.Name.ValueString() != "fileset1" ||
		state.ID.ValueString() != "my_metalake.my_catalog.my_schema.fileset1" {
		t.Errorf("unexpected imported state: %+v", state)
	}
}

func TestFilesetResourceImportStateInvalid(t *testing.T) {
	r := res.New().(resource.ResourceWithImportState)
	s := unitFilesetSchema(t, r.(resource.Resource))

	for _, id := range []string{
		"",
		"my_metalake",
		"my_metalake.my_catalog.my_schema",
		"my_metalake.my_catalog..fileset1",
		".my_catalog.my_schema.fileset1",
		"my_metalake.my_catalog.my_schema.",
	} {
		t.Run(id, func(t *testing.T) {
			ctx := context.Background()
			resp := &resource.ImportStateResponse{
				State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)},
			}
			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error for import id %q", id)
			}
		})
	}
}
