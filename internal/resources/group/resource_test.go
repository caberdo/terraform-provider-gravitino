package group_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/group"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// groupResponse is the verbatim `GroupResponse` example of groups.yaml
// (components/examples/GroupResponse).
const groupResponse = `{
  "code": 0,
  "group": {
    "name": "group1",
    "roles": [],
    "audit": {
      "creator": "gravitino",
      "createTime": "2023-12-08T06:41:25.595Z"
    }
  }
}`

// noSuchGroupError is the verbatim `NoSuchGroupException` example of
// groups.yaml, returned by the Gravitino server with HTTP status 404.
const noSuchGroupError = `{
  "code": 1003,
  "type": "NoSuchGroupException",
  "message": "Group does not exist",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchGroupException: Group does not exist",
    "..."
  ]
}`

// groupWithRolesResponse is the `GroupResponse` example of groups.yaml with
// the role of the spec's `RoleGrantRequest` example (permissions.yaml,
// "role1") granted.
const groupWithRolesResponse = `{
  "code": 0,
  "group": {
    "name": "group1",
    "roles": ["role1"],
    "audit": {
      "creator": "gravitino",
      "createTime": "2023-12-08T06:41:25.595Z"
    }
  }
}`

func groupSchema(t *testing.T) schema.Schema {
	t.Helper()
	r := res.New()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func tfValue(t *testing.T, ctx context.Context, s schema.Schema, model res.GroupResourceModel) tftypes.Value {
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

func baseModel() res.GroupResourceModel {
	return res.GroupResourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Name:     types.StringValue("group1"),
		Roles:    types.SetNull(types.StringType),
		Audit:    types.ObjectNull(res.AuditAttrTypes),
	}
}

func roles(values ...string) types.Set {
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	return types.SetValueMust(types.StringType, elems)
}

func newServer(t *testing.T, handler http.HandlerFunc) *res.GroupResource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	r := res.New()
	r.(*res.GroupResource).SetClient(c)
	return r.(*res.GroupResource)
}

func TestGroupResource_Metadata(t *testing.T) {
	r := res.New()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_group" {
		t.Fatalf("expected gravitino_group, got %s", resp.TypeName)
	}
}

// TestGroupResource_SchemaUpdatePolicy locks in the update policy of
// groups.yaml: there is no group update endpoint, so name and metalake force a
// replacement, while roles are mutated through the permissions endpoints.
func TestGroupResource_SchemaUpdatePolicy(t *testing.T) {
	ctx := context.Background()
	s := groupSchema(t)

	for _, name := range []string{"metalake", "name"} {
		attr := s.Attributes[name].(schema.StringAttribute)
		_, requiresReplace := stringPlan(ctx, attr, types.StringValue("old"), types.StringValue("new"))
		if !requiresReplace {
			t.Fatalf("%s: groups.yaml has no update endpoint, so this must force a replacement", name)
		}
	}

	rolesAttr := s.Attributes["roles"].(schema.SetAttribute)
	plannedRoles, _ := setPlan(ctx, rolesAttr, roles("role1"), types.SetUnknown(types.StringType))
	if plannedRoles.IsUnknown() || !plannedRoles.Equal(roles("role1")) {
		t.Fatalf("roles: expected UseStateForUnknown to keep the prior state value, got %v", plannedRoles)
	}

	// audit carries no UseStateForUnknown: the server rewrites it on every
	// modify, so the plan must leave it "known after apply" (pinning it to the
	// prior state produces "Provider produced inconsistent result after apply"
	// as soon as anything is updated).
	auditAttr := s.Attributes["audit"].(schema.ObjectAttribute)
	priorAudit := types.ObjectValueMust(res.AuditAttrTypes, map[string]attr.Value{
		"creator":            types.StringValue("gravitino"),
		"create_time":        types.StringValue("2023-12-08T06:41:25Z"),
		"last_modifier":      types.StringNull(),
		"last_modified_time": types.StringNull(),
	})
	plannedAudit, _ := objectPlan(ctx, auditAttr, priorAudit, types.ObjectUnknown(res.AuditAttrTypes))
	if !plannedAudit.IsUnknown() {
		t.Fatalf("audit must stay unknown in the plan, got %v", plannedAudit)
	}
}

// nonNullRaw stands in for the raw state/plan/config values that the framework
// passes to plan modifiers; the modifiers only inspect IsNull() on them.
var nonNullRaw = tftypes.NewValue(tftypes.String, "state")

func stringPlan(ctx context.Context, attr schema.StringAttribute, state, plan types.String) (types.String, bool) {
	req := planmodifier.StringRequest{
		State:       tfsdk.State{Raw: nonNullRaw},
		Plan:        tfsdk.Plan{Raw: nonNullRaw},
		Config:      tfsdk.Config{Raw: nonNullRaw},
		StateValue:  state,
		PlanValue:   plan,
		ConfigValue: types.StringNull(),
	}
	resp := &planmodifier.StringResponse{PlanValue: plan}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyString(ctx, req, resp)
	}
	return resp.PlanValue, resp.RequiresReplace
}

func setPlan(ctx context.Context, attr schema.SetAttribute, state, plan types.Set) (types.Set, bool) {
	req := planmodifier.SetRequest{
		State:       tfsdk.State{Raw: nonNullRaw},
		Plan:        tfsdk.Plan{Raw: nonNullRaw},
		Config:      tfsdk.Config{Raw: nonNullRaw},
		StateValue:  state,
		PlanValue:   plan,
		ConfigValue: types.SetNull(types.StringType),
	}
	resp := &planmodifier.SetResponse{PlanValue: plan}
	for _, m := range attr.PlanModifiers {
		m.PlanModifySet(ctx, req, resp)
	}
	return resp.PlanValue, resp.RequiresReplace
}

func objectPlan(ctx context.Context, attr schema.ObjectAttribute, state, plan types.Object) (types.Object, bool) {
	req := planmodifier.ObjectRequest{
		State:       tfsdk.State{Raw: nonNullRaw},
		Plan:        tfsdk.Plan{Raw: nonNullRaw},
		Config:      tfsdk.Config{Raw: nonNullRaw},
		StateValue:  state,
		PlanValue:   plan,
		ConfigValue: types.ObjectNull(res.AuditAttrTypes),
	}
	resp := &planmodifier.ObjectResponse{PlanValue: plan}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyObject(ctx, req, resp)
	}
	return resp.PlanValue, resp.RequiresReplace
}

// TestGroupResource_Create asserts the create body equals GroupAddRequest of
// groups.yaml exactly.
func TestGroupResource_Create(t *testing.T) {
	ctx := context.Background()
	s := groupSchema(t)

	var method, requestPath string
	var rawBody []byte
	r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		method, requestPath = req.Method, req.URL.Path
		rawBody, _ = io.ReadAll(req.Body)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, groupResponse)
	})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, baseModel())}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if method != http.MethodPost {
		t.Fatalf("expected POST, got %s", method)
	}
	if want := "/api/metalakes/my_test_metalake/groups"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}
	assertJSONEqual(t, map[string]any{"name": "group1"}, decodeBody(t, rawBody))

	var state res.GroupResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.ID.ValueString() != "my_test_metalake.group1" {
		t.Fatalf("unexpected id: %s", state.ID.ValueString())
	}
	if state.Roles.IsUnknown() {
		t.Fatalf("roles must be known after create, got %v", state.Roles)
	}
	if state.Audit.IsNull() || state.Audit.IsUnknown() {
		t.Fatal("audit must be set from the response")
	}
}

// TestGroupResource_CreateGrantsPlannedRoles covers the Optional+Computed roles
// attribute: roles configured at create time are granted through the
// permissions endpoint of permissions.yaml.
func TestGroupResource_CreateGrantsPlannedRoles(t *testing.T) {
	ctx := context.Background()
	s := groupSchema(t)

	var calls []string
	var grantBody []byte
	r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		switch req.URL.Path {
		case "/api/metalakes/my_test_metalake/groups":
			fmt.Fprint(w, groupResponse)
		default:
			grantBody, _ = io.ReadAll(req.Body)
			fmt.Fprint(w, groupWithRolesResponse)
		}
	})

	plan := baseModel()
	plan.Roles = roles("role1")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	// RoleGrantRequest of permissions.yaml: the field is `roleNames`.
	assertJSONEqual(t, map[string]any{"roleNames": []any{"role1"}}, decodeBody(t, grantBody))

	var state res.GroupResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if !state.Roles.Equal(roles("role1")) {
		t.Fatalf("expected the granted role in state, got %v", state.Roles)
	}
	if len(calls) != 2 {
		t.Fatalf("expected a create plus a grant call, got %v", calls)
	}
	if calls[1] != "PUT /api/metalakes/my_test_metalake/permissions/groups/group1/grant" {
		t.Fatalf("unexpected grant call: %s", calls[1])
	}
}

func TestGroupResource_UpdateGrantsRole(t *testing.T) {
	ctx := context.Background()
	s := groupSchema(t)

	var calls []string
	var grantBody []byte
	r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		grantBody, _ = io.ReadAll(req.Body)
		fmt.Fprint(w, groupWithRolesResponse)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.group1")
	state.Roles = roles()

	plan := baseModel()
	plan.ID = types.StringValue("my_test_metalake.group1")
	plan.Roles = roles("role1")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if len(calls) != 1 {
		t.Fatalf("expected exactly one call, got %v", calls)
	}
	if calls[0] != "PUT /api/metalakes/my_test_metalake/permissions/groups/group1/grant" {
		t.Fatalf("unexpected grant call: %s", calls[0])
	}
	assertJSONEqual(t, map[string]any{"roleNames": []any{"role1"}}, decodeBody(t, grantBody))

	var newState res.GroupResourceModel
	if diags := resp.State.Get(ctx, &newState); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if !newState.Roles.Equal(roles("role1")) {
		t.Fatalf("unexpected roles: %v", newState.Roles)
	}
}

func TestGroupResource_UpdateRevokesRole(t *testing.T) {
	ctx := context.Background()
	s := groupSchema(t)

	var calls []string
	var revokeBody []byte
	r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		revokeBody, _ = io.ReadAll(req.Body)
		fmt.Fprint(w, groupResponse)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.group1")
	state.Roles = roles("role1")

	plan := baseModel()
	plan.ID = types.StringValue("my_test_metalake.group1")
	plan.Roles = roles()

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if len(calls) != 1 {
		t.Fatalf("expected exactly one call, got %v", calls)
	}
	if calls[0] != "PUT /api/metalakes/my_test_metalake/permissions/groups/group1/revoke" {
		t.Fatalf("unexpected revoke call: %s", calls[0])
	}
	// RoleRevokeRequest of permissions.yaml.
	assertJSONEqual(t, map[string]any{"roleNames": []any{"role1"}}, decodeBody(t, revokeBody))

	var newState res.GroupResourceModel
	if diags := resp.State.Get(ctx, &newState); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if newState.Roles.Elements() == nil || len(newState.Roles.Elements()) != 0 {
		t.Fatalf("expected the role to be revoked, got %v", newState.Roles)
	}
}

// TestGroupResource_UpdateNothingChangedIdempotent covers the "nothing changed"
// Update branch: no request is sent and no computed value stays unknown.
func TestGroupResource_UpdateNothingChangedIdempotent(t *testing.T) {
	ctx := context.Background()
	s := groupSchema(t)

	calls := 0
	r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		calls++
		fmt.Fprint(w, groupResponse)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.group1")
	state.Roles = roles()
	state.Audit = types.ObjectValueMust(res.AuditAttrTypes, map[string]attr.Value{
		"creator":            types.StringValue("gravitino"),
		"create_time":        types.StringValue("2023-12-08T06:41:25Z"),
		"last_modifier":      types.StringNull(),
		"last_modified_time": types.StringNull(),
	})

	plan := baseModel()
	plan.ID = types.StringValue("my_test_metalake.group1")
	plan.Roles = types.SetUnknown(types.StringType)
	plan.Audit = types.ObjectUnknown(res.AuditAttrTypes)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if calls != 0 {
		t.Fatalf("no request may be sent when nothing changed, got %d calls", calls)
	}

	var newState res.GroupResourceModel
	if diags := resp.State.Get(ctx, &newState); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if newState.Roles.IsUnknown() || newState.Audit.IsUnknown() {
		t.Fatalf("computed values resolved from state must be known, got %v", newState)
	}
	if newState.ID.ValueString() != "my_test_metalake.group1" {
		t.Fatalf("unexpected id: %s", newState.ID.ValueString())
	}
}

func TestGroupResource_ReadNotFoundRemovesState(t *testing.T) {
	ctx := context.Background()
	s := groupSchema(t)

	r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchGroupError)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.group1")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 must not produce an error, got: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected the resource to be removed from state on 404")
	}
}

func TestGroupResource_Delete(t *testing.T) {
	ctx := context.Background()
	s := groupSchema(t)

	var method, requestPath string
	r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		method, requestPath = req.Method, req.URL.Path
		json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.group1")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if method != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", method)
	}
	if want := "/api/metalakes/my_test_metalake/groups/group1"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}
}

func TestGroupResource_DeleteNotFoundIsSuccess(t *testing.T) {
	ctx := context.Background()
	s := groupSchema(t)

	r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchGroupError)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.group1")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("deleting an already deleted group must succeed, got: %v", resp.Diagnostics)
	}
}

func TestGroupResource_ImportState(t *testing.T) {
	ctx := context.Background()
	s := groupSchema(t)
	r := res.New().(resource.ResourceWithImportState)

	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, baseModel())}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "my_metalake.my_group"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state res.GroupResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.Metalake.ValueString() != "my_metalake" || state.Name.ValueString() != "my_group" {
		t.Fatalf("unexpected import result: metalake=%s name=%s", state.Metalake.ValueString(), state.Name.ValueString())
	}
	if state.ID.ValueString() != "my_metalake.my_group" {
		t.Fatalf("unexpected id: %s", state.ID.ValueString())
	}
}

func TestGroupResource_ImportState_Invalid(t *testing.T) {
	ctx := context.Background()
	s := groupSchema(t)
	r := res.New().(resource.ResourceWithImportState)

	for _, id := range []string{"no_dot_here", "trailing.", ".leading", ""} {
		resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, baseModel())}}
		r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected an error for invalid import ID %q", id)
		}
	}
}

func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("invalid request body %s: %v", raw, err)
	}
	return body
}

func assertJSONEqual(t *testing.T, want, got map[string]any) {
	t.Helper()
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("unexpected payload:\n want %s\n  got %s", wantJSON, gotJSON)
	}
}
