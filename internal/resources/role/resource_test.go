package role_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/role"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Spec example payloads, copied verbatim from /tmp/gravspec/roles.yaml and
// /tmp/gravspec/permissions.yaml (components.examples).
const (
	specRoleCreateRequest = `{
        "name": "role1",
        "properties": {"k1": "v1"},
        "securableObjects": [
          {
            "fullName" : "catalog1.schema1.table1",
            "type": "TABLE",
            "privileges": [
              {
                "name": "SELECT_TABLE",
                "condition": "ALLOW"
              }
            ]
          }
        ]
      }`

	specPrivilegeOverrideRequest = `{
        "overrides": [
          {
            "fullName": "catalog1.schema1.table1",
            "type": "TABLE",
            "privileges": [
              {
                "name": "SELECT_TABLE",
                "condition": "ALLOW"
              }
            ]
          }
        ]
      }`

	specNoSuchRoleException = `{
        "code": 1003,
        "type": "NoSuchRoleException",
        "message": "Role does not exist",
        "stack": [
          "org.apache.gravitino.exceptions.NoSuchRoleException: Role does not exist",
          "..."
        ]
      }`
)

func TestRoleResource_Schema(t *testing.T) {
	r := res.NewRoleResource()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.TODO(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_role" {
		t.Fatalf("expected gravitino_role, got %s", resp.TypeName)
	}
}

// jsonEqual compares two JSON documents structurally.
func jsonEqual(t *testing.T, want, got string) bool {
	t.Helper()

	var wantVal, gotVal interface{}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("failed to unmarshal expected JSON %q: %v", want, err)
	}
	if err := json.Unmarshal([]byte(got), &gotVal); err != nil {
		t.Fatalf("failed to unmarshal actual JSON %q: %v", got, err)
	}
	return reflect.DeepEqual(wantVal, gotVal)
}

func securableObjectsValue(t *testing.T, objects []models.SecurableObject) types.Set {
	t.Helper()

	emptySet := types.SetValueMust(types.ObjectType{AttrTypes: res.SecurableObjectAttrTypes}, []attr.Value{})

	items := make([]attr.Value, 0, len(objects))
	for _, o := range objects {
		privItems := make([]attr.Value, 0, len(o.Privileges))
		for _, p := range o.Privileges {
			priv, d := types.ObjectValue(res.PrivilegeAttrTypes, map[string]attr.Value{
				"name":      types.StringValue(p.Name),
				"condition": types.StringValue(p.Condition),
			})
			if d.HasError() {
				t.Fatalf("failed to build privilege value: %v", d)
			}
			privItems = append(privItems, priv)
		}

		privSet, d := types.SetValue(types.ObjectType{AttrTypes: res.PrivilegeAttrTypes}, privItems)
		if d.HasError() {
			t.Fatalf("failed to build privilege set: %v", d)
		}

		obj, d := types.ObjectValue(res.SecurableObjectAttrTypes, map[string]attr.Value{
			"full_name":  types.StringValue(o.FullName),
			"type":       types.StringValue(o.Type),
			"privileges": privSet,
		})
		if d.HasError() {
			t.Fatalf("failed to build securable object value: %v", d)
		}
		items = append(items, obj)
	}

	if len(items) == 0 {
		return emptySet
	}

	set, d := types.SetValue(types.ObjectType{AttrTypes: res.SecurableObjectAttrTypes}, items)
	if d.HasError() {
		t.Fatalf("failed to build securable object set: %v", d)
	}
	return set
}

// newResource wires a role resource to a test server and returns the resource, its
// schema and the client.
func newResource(t *testing.T, handler http.Handler) (*res.RoleResource, schema.Schema, *client.Client) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	r := res.NewRoleResource().(*res.RoleResource)
	r.SetClient(c)

	schemaResp := &resource.SchemaResponse{}
	res.NewRoleResource().Schema(context.Background(), resource.SchemaRequest{}, schemaResp)

	return r, schemaResp.Schema, c
}

func rawValue(t *testing.T, schemaObj schema.Schema, model interface{}) tftypes.Value {
	t.Helper()

	ctx := context.Background()
	obj, diags := types.ObjectValueFrom(ctx, schemaObj.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("failed to build object value: %v", diags)
	}
	raw, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}
	return raw
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal %T: %v", v, err)
	}
	return string(raw)
}

// TestRoleResource_Create_RequestBody asserts the create request body is exactly the
// RoleCreateRequest example of the v1.3.0 spec.
func TestRoleResource_Create_RequestBody(t *testing.T) {
	var body string
	var method, requestPath string

	r, schemaObj, _ := newResource(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		method = req.Method
		requestPath = req.URL.Path
		raw, _ := io.ReadAll(req.Body)
		body = string(raw)

		// The response is the RoleResponse example of the spec.
		_, _ = w.Write([]byte(`{
        "code": 0,
        "role": {
          "name": "role1",
          "properties" : { "k1": "v1" },
          "securableObjects": [
            {
              "fullName": "catalog1.schema1.table1",
              "type": "TABLE",
              "privileges": [
                {
                    "name": "SELECT_TABLE",
                    "condition": "ALLOW"
                }
              ]
            }
          ]
        }
      }`))
	}))

	plan := res.RoleResourceModel{
		Metalake:   types.StringValue("ml"),
		Name:       types.StringValue("role1"),
		Properties: types.MapValueMust(types.StringType, map[string]attr.Value{"k1": types.StringValue("v1")}),
		SecurableObjects: securableObjectsValue(t, []models.SecurableObject{{
			FullName:   "catalog1.schema1.table1",
			Type:       "TABLE",
			Privileges: []models.Privilege{{Name: "SELECT_TABLE", Condition: "ALLOW"}},
		}}),
		Audit: types.ObjectNull(res.AuditAttrTypes),
	}

	ctx := context.Background()
	req := resource.CreateRequest{Plan: tfsdk.Plan{Schema: schemaObj, Raw: rawValue(t, schemaObj, plan)}}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}

	r.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	if method != http.MethodPost || requestPath != "/api/metalakes/ml/roles" {
		t.Fatalf("expected POST /api/metalakes/ml/roles, got %s %s", method, requestPath)
	}

	if !jsonEqual(t, specRoleCreateRequest, body) {
		t.Fatalf("create request body does not match the spec example.\nwant: %s\ngot:  %s", specRoleCreateRequest, body)
	}

	var state res.RoleResourceModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("failed to read state: %v", d)
	}
	if state.ID.ValueString() != "ml.role1" {
		t.Fatalf("expected id ml.role1, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "role1" {
		t.Fatalf("expected name role1, got %q", state.Name.ValueString())
	}
	if len(state.SecurableObjects.Elements()) != 1 {
		t.Fatalf("expected 1 securable object in state, got %d", len(state.SecurableObjects.Elements()))
	}
	if state.Audit.IsUnknown() {
		t.Fatal("audit must have a known value in state")
	}
}

// TestRoleResource_Create_OmitsSecurableObjects sends an empty array when the user
// does not configure securable objects: the API rejects a request without the field.
func TestRoleResource_Create_OmitsSecurableObjects(t *testing.T) {
	var body string

	r, schemaObj, _ := newResource(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		body = string(raw)
		_, _ = w.Write([]byte(`{"code":0,"role":{"name":"role1","properties":{},"securableObjects":[]}}`))
	}))

	plan := res.RoleResourceModel{
		Metalake:         types.StringValue("ml"),
		Name:             types.StringValue("role1"),
		Properties:       types.MapNull(types.StringType),
		SecurableObjects: types.SetNull(types.ObjectType{AttrTypes: res.SecurableObjectAttrTypes}),
		Audit:            types.ObjectNull(res.AuditAttrTypes),
	}

	ctx := context.Background()
	req := resource.CreateRequest{Plan: tfsdk.Plan{Schema: schemaObj, Raw: rawValue(t, schemaObj, plan)}}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}

	r.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	if !jsonEqual(t, `{"name":"role1","securableObjects":[]}`, body) {
		t.Fatalf("expected securableObjects to be sent as an empty array, got %s", body)
	}
}

// TestRoleResource_Create_KeepsFullNameRelativeToTheMetalake guards the wire convention
// verified against a live Gravitino 1.3.0: `fullName` is relative to the metalake, so the
// provider must never add a metalake prefix. `my_metalake.my_catalog` for a CATALOG is
// rejected by the server with HTTP 400 IllegalNamespaceException.
func TestRoleResource_Create_KeepsFullNameRelativeToTheMetalake(t *testing.T) {
	var body string

	r, schemaObj, _ := newResource(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		body = string(raw)
		_, _ = w.Write([]byte(`{"code":0,"role":{"name":"role1","securableObjects":[]}}`))
	}))

	plan := res.RoleResourceModel{
		Metalake:   types.StringValue("my_metalake"),
		Name:       types.StringValue("role1"),
		Properties: types.MapNull(types.StringType),
		SecurableObjects: securableObjectsValue(t, []models.SecurableObject{
			{
				FullName:   "my_metalake",
				Type:       "METALAKE",
				Privileges: []models.Privilege{{Name: "CREATE_CATALOG", Condition: "ALLOW"}},
			},
			{
				FullName:   "my_catalog",
				Type:       "CATALOG",
				Privileges: []models.Privilege{{Name: "USE_CATALOG", Condition: "ALLOW"}},
			},
		}),
		Audit: types.ObjectNull(res.AuditAttrTypes),
	}

	ctx := context.Background()
	req := resource.CreateRequest{Plan: tfsdk.Plan{Schema: schemaObj, Raw: rawValue(t, schemaObj, plan)}}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}

	r.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	var sent models.RoleCreateRequest
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("failed to decode create body %q: %v", body, err)
	}

	names := make([]string, 0, len(sent.SecurableObjects))
	for _, o := range sent.SecurableObjects {
		names = append(names, o.FullName)
	}
	want := []string{"my_metalake", "my_catalog"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("expected the full names to be sent unchanged (relative to the metalake), got %v", names)
	}
}

// TestRoleResource_Update_OverridePayload asserts the update payload is exactly the
// PrivilegeOverrideRequest example of the v1.3.0 spec.
func TestRoleResource_Update_OverridePayload(t *testing.T) {
	var overrideBody string
	var overrideCalled bool

	r, schemaObj, _ := newResource(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPut && req.URL.Path == "/api/metalakes/ml/permissions/roles/role1":
			overrideCalled = true
			raw, _ := io.ReadAll(req.Body)
			overrideBody = string(raw)
			_, _ = w.Write([]byte(`{"code":0,"role":{"name":"role1","properties":{"k1":"v1"},"securableObjects":[{"fullName":"catalog1.schema1.table1","type":"TABLE","privileges":[{"name":"SELECT_TABLE","condition":"ALLOW"}]}]}}`))
		case req.Method == http.MethodGet && req.URL.Path == "/api/metalakes/ml/roles/role1":
			_, _ = w.Write([]byte(`{"code":0,"role":{"name":"role1","properties":{"k1":"v1"},"securableObjects":[{"fullName":"catalog1.schema1.table1","type":"TABLE","privileges":[{"name":"SELECT_TABLE","condition":"ALLOW"}]}],"audit":{"creator":"admin","createTime":"2026-01-01T00:00:00Z","lastModifier":"admin","lastModifiedTime":"2026-01-01T00:00:00Z"}}}`))
		default:
			http.NotFound(w, req)
		}
	}))

	state := res.RoleResourceModel{
		ID:         types.StringValue("ml.role1"),
		Metalake:   types.StringValue("ml"),
		Name:       types.StringValue("role1"),
		Properties: types.MapValueMust(types.StringType, map[string]attr.Value{"k1": types.StringValue("v1")}),
		SecurableObjects: securableObjectsValue(t, []models.SecurableObject{{
			FullName:   "catalog1.schema1.table1",
			Type:       "TABLE",
			Privileges: []models.Privilege{{Name: "MODIFY_TABLE", Condition: "ALLOW"}},
		}}),
		Audit: types.ObjectNull(res.AuditAttrTypes),
	}

	plan := state
	plan.SecurableObjects = securableObjectsValue(t, []models.SecurableObject{{
		FullName:   "catalog1.schema1.table1",
		Type:       "TABLE",
		Privileges: []models.Privilege{{Name: "SELECT_TABLE", Condition: "ALLOW"}},
	}})

	ctx := context.Background()
	req := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schemaObj, Raw: rawValue(t, schemaObj, plan)},
		State: tfsdk.State{Schema: schemaObj, Raw: rawValue(t, schemaObj, state)},
	}
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaObj}}

	r.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	if !overrideCalled {
		t.Fatal("expected PUT /api/metalakes/ml/permissions/roles/role1 to be called")
	}

	if !jsonEqual(t, specPrivilegeOverrideRequest, overrideBody) {
		t.Fatalf("override payload does not match the spec example.\nwant: %s\ngot:  %s", specPrivilegeOverrideRequest, overrideBody)
	}

	var newState res.RoleResourceModel
	if d := resp.State.Get(ctx, &newState); d.HasError() {
		t.Fatalf("failed to read state: %v", d)
	}
	if newState.SecurableObjects.IsUnknown() {
		t.Fatal("securable_objects must have a known value after update")
	}
	if newState.Audit.IsNull() || newState.Audit.IsUnknown() {
		t.Fatalf("audit must be populated from the read-back response, got %s", newState.Audit.String())
	}
}

// TestRoleResource_Update_UnchangedSecurablesIsNoop proves that an update that does
// not touch the privileges does not send an override request.
func TestRoleResource_Update_UnchangedSecurablesIsNoop(t *testing.T) {
	overrideCalled := false
	getCalled := false

	r, schemaObj, _ := newResource(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPut:
			overrideCalled = true
			_, _ = w.Write([]byte(`{"code":0,"role":{"name":"role1","securableObjects":[]}}`))
		case http.MethodGet:
			getCalled = true
			_, _ = w.Write([]byte(`{"code":0,"role":{"name":"role1","properties":{"k1":"v1"},"securableObjects":[{"fullName":"catalog1.schema1.table1","type":"TABLE","privileges":[{"name":"SELECT_TABLE","condition":"ALLOW"}]}]}}`))
		default:
			http.NotFound(w, req)
		}
	}))

	model := res.RoleResourceModel{
		ID:         types.StringValue("ml.role1"),
		Metalake:   types.StringValue("ml"),
		Name:       types.StringValue("role1"),
		Properties: types.MapValueMust(types.StringType, map[string]attr.Value{"k1": types.StringValue("v1")}),
		SecurableObjects: securableObjectsValue(t, []models.SecurableObject{{
			FullName:   "catalog1.schema1.table1",
			Type:       "TABLE",
			Privileges: []models.Privilege{{Name: "SELECT_TABLE", Condition: "ALLOW"}},
		}}),
		Audit: types.ObjectNull(res.AuditAttrTypes),
	}
	plan := model

	ctx := context.Background()
	req := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schemaObj, Raw: rawValue(t, schemaObj, plan)},
		State: tfsdk.State{Schema: schemaObj, Raw: rawValue(t, schemaObj, model)},
	}
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaObj}}

	r.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	if overrideCalled {
		t.Fatal("no privilege override must be sent when the securable objects did not change")
	}
	if !getCalled {
		t.Fatal("expected the role to be re-read after the update")
	}
}

// TestRoleResource_PropertiesRequiresReplace proves that changing `properties` forces
// a replacement instead of being silently ignored: Gravitino v1.3.0 has no role
// property update endpoint.
// TestRoleResource_PropertiesRequiresReplace proves that a *configured* change of
// `properties` forces a replacement instead of being silently ignored: Gravitino v1.3.0
// has no role property update endpoint. An unconfigured `properties` (computed from the
// API) must NOT force a replacement, otherwise every privilege update would turn into a
// destroy/recreate cycle.
func ptrMap(m types.Map) *types.Map {
	return &m
}

func TestRoleResource_PropertiesRequiresReplace(t *testing.T) {
	ctx := context.Background()

	r := res.NewRoleResource()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	propertiesAttr, ok := schemaResp.Schema.Attributes["properties"].(schema.MapAttribute)
	if !ok {
		t.Fatalf("expected properties to be a MapAttribute, got %T", schemaResp.Schema.Attributes["properties"])
	}
	if len(propertiesAttr.PlanModifiers) == 0 {
		t.Fatal("properties must have plan modifiers")
	}

	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"properties": tftypes.Map{ElementType: tftypes.String},
	}}
	raw := func(value string) tftypes.Value {
		return tftypes.NewValue(objType, map[string]tftypes.Value{
			"properties": tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{
				"managed_by": tftypes.NewValue(tftypes.String, value),
			}),
		})
	}
	mapValue := func(value string) types.Map {
		return types.MapValueMust(types.StringType, map[string]attr.Value{"managed_by": types.StringValue(value)})
	}

	tests := []struct {
		name            string
		state           types.Map
		plan            types.Map
		config          types.Map
		requiresReplace bool
		expectPlan      *types.Map
	}{
		{
			name:            "configured and changed",
			state:           mapValue("security-team"),
			plan:            mapValue("platform-team"),
			config:          mapValue("platform-team"),
			requiresReplace: true,
		},
		{
			name:            "configured and unchanged",
			state:           mapValue("security-team"),
			plan:            mapValue("security-team"),
			config:          mapValue("security-team"),
			requiresReplace: false,
		},
		{
			name:            "unconfigured stays computed",
			state:           mapValue("security-team"),
			plan:            types.MapUnknown(types.StringType),
			config:          types.MapNull(types.StringType),
			requiresReplace: false,
			expectPlan:      ptrMap(mapValue("security-team")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := planmodifier.MapRequest{
				State:       tfsdk.State{Raw: raw("state")},
				StateValue:  tt.state,
				Plan:        tfsdk.Plan{Raw: raw("plan")},
				PlanValue:   tt.plan,
				ConfigValue: tt.config,
				Path:        path.Root("properties"),
			}
			resp := &planmodifier.MapResponse{PlanValue: req.PlanValue}

			for _, m := range propertiesAttr.PlanModifiers {
				m.PlanModifyMap(ctx, req, resp)
			}

			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}

			if tt.requiresReplace && !resp.RequiresReplace {
				t.Fatal("changing configured properties must require a replacement")
			}
			if !tt.requiresReplace && resp.RequiresReplace {
				t.Fatalf("%s must not require a replacement", tt.name)
			}
			if tt.expectPlan != nil && !resp.PlanValue.Equal(*tt.expectPlan) {
				t.Fatalf("expected the planned value to keep the state value %s, got %s", *tt.expectPlan, resp.PlanValue)
			}
			if !tt.requiresReplace && tt.expectPlan == nil && resp.PlanValue.IsUnknown() {
				t.Fatal("the planned value must not stay unknown")
			}
		})
	}
}

// TestRoleResource_Read_NotFound_RemovesFromState uses the real Gravitino 404 body.
func TestRoleResource_Read_NotFound_RemovesFromState(t *testing.T) {
	r, schemaObj, _ := newResource(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(specNoSuchRoleException))
	}))

	state := res.RoleResourceModel{
		ID:               types.StringValue("ml.role1"),
		Metalake:         types.StringValue("ml"),
		Name:             types.StringValue("role1"),
		Properties:       types.MapNull(types.StringType),
		SecurableObjects: types.SetNull(types.ObjectType{AttrTypes: res.SecurableObjectAttrTypes}),
		Audit:            types.ObjectNull(res.AuditAttrTypes),
	}

	ctx := context.Background()
	req := resource.ReadRequest{State: tfsdk.State{Schema: schemaObj, Raw: rawValue(t, schemaObj, state)}}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: schemaObj, Raw: rawValue(t, schemaObj, state)}}

	r.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("a 404 must not be reported as an error")
	}

	if !resp.State.Raw.IsNull() {
		t.Fatal("expected the role to be removed from state")
	}
}

// TestRoleResource_Delete_NotFound proves that deleting an already deleted role is
// treated as success.
func TestRoleResource_Delete_NotFound(t *testing.T) {
	r, schemaObj, _ := newResource(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(specNoSuchRoleException))
	}))

	state := res.RoleResourceModel{
		ID:               types.StringValue("ml.role1"),
		Metalake:         types.StringValue("ml"),
		Name:             types.StringValue("role1"),
		Properties:       types.MapNull(types.StringType),
		SecurableObjects: types.SetNull(types.ObjectType{AttrTypes: res.SecurableObjectAttrTypes}),
		Audit:            types.ObjectNull(res.AuditAttrTypes),
	}

	ctx := context.Background()
	req := resource.DeleteRequest{State: tfsdk.State{Schema: schemaObj, Raw: rawValue(t, schemaObj, state)}}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: schemaObj}}

	r.Delete(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("a 404 on delete must be treated as success")
	}
}

func TestRoleResource_Delete(t *testing.T) {
	var method, requestPath string

	r, schemaObj, _ := newResource(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		method = req.Method
		requestPath = req.URL.Path
		_, _ = w.Write([]byte(`{"code":0,"dropped":true}`))
	}))

	state := res.RoleResourceModel{
		ID:               types.StringValue("ml.role1"),
		Metalake:         types.StringValue("ml"),
		Name:             types.StringValue("role1"),
		Properties:       types.MapNull(types.StringType),
		SecurableObjects: types.SetNull(types.ObjectType{AttrTypes: res.SecurableObjectAttrTypes}),
		Audit:            types.ObjectNull(res.AuditAttrTypes),
	}

	ctx := context.Background()
	req := resource.DeleteRequest{State: tfsdk.State{Schema: schemaObj, Raw: rawValue(t, schemaObj, state)}}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: schemaObj}}

	r.Delete(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	if method != http.MethodDelete || requestPath != "/api/metalakes/ml/roles/role1" {
		t.Fatalf("expected DELETE /api/metalakes/ml/roles/role1, got %s %s", method, requestPath)
	}
}

func TestRoleResource_ImportState(t *testing.T) {
	r := res.NewRoleResource().(resource.ResourceWithImportState)

	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.(resource.Resource).Schema(ctx, resource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	nullModel := res.RoleResourceModel{
		Metalake:         types.StringNull(),
		Name:             types.StringNull(),
		Properties:       types.MapNull(types.StringType),
		SecurableObjects: types.SetNull(types.ObjectType{AttrTypes: res.SecurableObjectAttrTypes}),
		Audit:            types.ObjectNull(res.AuditAttrTypes),
	}

	req := resource.ImportStateRequest{ID: "my_metalake.my_role"}
	resp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaObj, Raw: rawValue(t, schemaObj, nullModel)},
	}

	r.ImportState(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	var state res.RoleResourceModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("failed to read state: %v", d)
	}
	if state.Metalake.ValueString() != "my_metalake" {
		t.Fatalf("expected metalake my_metalake, got %q", state.Metalake.ValueString())
	}
	if state.Name.ValueString() != "my_role" {
		t.Fatalf("expected name my_role, got %q", state.Name.ValueString())
	}
	if state.ID.ValueString() != "my_metalake.my_role" {
		t.Fatalf("expected id my_metalake.my_role, got %q", state.ID.ValueString())
	}
}

func TestRoleResource_ImportState_Invalid(t *testing.T) {
	r := res.NewRoleResource().(resource.ResourceWithImportState)

	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.(resource.Resource).Schema(ctx, resource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	nullModel := res.RoleResourceModel{
		Metalake:         types.StringNull(),
		Name:             types.StringNull(),
		Properties:       types.MapNull(types.StringType),
		SecurableObjects: types.SetNull(types.ObjectType{AttrTypes: res.SecurableObjectAttrTypes}),
		Audit:            types.ObjectNull(res.AuditAttrTypes),
	}

	for _, id := range []string{"nodot", "", ".", "my_metalake.", ".my_role", "a.b.c"} {
		t.Run(id, func(t *testing.T) {
			req := resource.ImportStateRequest{ID: id}
			resp := &resource.ImportStateResponse{
				State: tfsdk.State{Schema: schemaObj, Raw: rawValue(t, schemaObj, nullModel)},
			}

			r.ImportState(ctx, req, resp)

			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error for import ID %q", id)
			}
		})
	}
}
