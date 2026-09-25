package policy_test

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

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/policy"

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

// policyResponse is the verbatim `PolicyResponse` example of policies.yaml
// (components/examples/PolicyResponse). It proves that numeric customRules
// values (the spec example uses 123) decode without breaking the read path.
const policyResponse = `{
  "code": 0,
  "policy": {
    "name": "my_policy1",
    "comment": "This is a test policy",
    "policyType": "custom",
    "enabled": false,
    "content": {
      "customRules": {
        "rule1": 123
      },
      "supportedObjectTypes": [
        "SCHEMA",
        "TABLE",
        "MODEL",
        "TOPIC",
        "FILESET",
        "CATALOG"
      ],
      "properties": {
        "key1": "value1"
      }
    },
    "inherited": null,
    "audit": {
      "creator": "anonymous",
      "createTime": "2025-08-04T10:29:23.463Z"
    }
  }
}`

// noSuchPolicyError is the verbatim `NoSuchPolicyException` example of
// policies.yaml, returned by the Gravitino server with HTTP status 404.
const noSuchPolicyError = `{
  "code": 1003,
  "type": "NoSuchPolicyException",
  "message": "Failed to operate policy(s) [my_policy] operation [LOAD] under metalake [my_test_metalake], reason [NoSuchPolicyException]",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchPolicyException: Policy my_policy does not exist",
    "..."
  ]
}`

func policySchema(t *testing.T) schema.Schema {
	t.Helper()
	r := res.New()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func tfValue(t *testing.T, ctx context.Context, s schema.Schema, model res.PolicyResourceModel) tftypes.Value {
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

func allObjectTypes() types.Set {
	return types.SetValueMust(types.StringType, []attr.Value{
		types.StringValue("CATALOG"),
		types.StringValue("SCHEMA"),
		types.StringValue("TABLE"),
		types.StringValue("FILESET"),
		types.StringValue("TOPIC"),
		types.StringValue("MODEL"),
	})
}

func baseModel() res.PolicyResourceModel {
	return res.PolicyResourceModel{
		Metalake:             types.StringValue("my_test_metalake"),
		Name:                 types.StringValue("my_policy1"),
		Comment:              types.StringValue("This is a test policy"),
		PolicyType:           types.StringValue("custom"),
		Enabled:              types.BoolValue(false),
		SupportedObjectTypes: allObjectTypes(),
		Properties:           types.MapValueMust(types.StringType, map[string]attr.Value{"key1": types.StringValue("value1")}),
		CustomRules:          types.MapValueMust(types.StringType, map[string]attr.Value{"rule1": types.StringValue("123")}),
		Audit:                types.ObjectNull(res.AuditAttrTypes),
	}
}

func TestPolicyResource_Metadata(t *testing.T) {
	r := res.New()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_policy" {
		t.Fatalf("expected gravitino_policy, got %s", resp.TypeName)
	}
}

// TestPolicyResource_SchemaUpdatePolicy locks in which attributes may be
// changed in place (per policies.yaml) and which ones force a replacement.
func TestPolicyResource_SchemaUpdatePolicy(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)

	// metalake is the parent identifier and policy_type selects the content
	// structure; policies.yaml has no update request for either of them.
	for _, name := range []string{"metalake", "policy_type"} {
		attr := s.Attributes[name].(schema.StringAttribute)
		_, requiresReplace := stringPlan(ctx, attr, types.StringValue("old"), types.StringValue("new"))
		if !requiresReplace {
			t.Fatalf("%s: changing this value must force a replacement (policies.yaml has no update request for it)", name)
		}
	}

	// name and comment are updateable: policies.yaml has rename and
	// updateComment, and the content is updateable through updateContent.
	for _, name := range []string{"name", "comment"} {
		attr := s.Attributes[name].(schema.StringAttribute)
		_, requiresReplace := stringPlan(ctx, attr, types.StringValue("old"), types.StringValue("new"))
		if requiresReplace {
			t.Fatalf("%s: must be updateable in place, but RequiresReplace is set", name)
		}
	}

	// Every computed attribute must be known on every code path; the plan
	// modifiers guarantee that for the "nothing changed" Update branch.
	for _, name := range []string{"id", "comment", "policy_type"} {
		attr := s.Attributes[name].(schema.StringAttribute)
		planned, _ := stringPlan(ctx, attr, types.StringValue("prior"), types.StringUnknown())
		if planned.IsUnknown() || planned.IsNull() || planned.ValueString() != "prior" {
			t.Fatalf("%s: expected UseStateForUnknown to keep the prior state value, got %v", name, planned)
		}
	}

	priorProps := types.MapValueMust(types.StringType, map[string]attr.Value{"key1": types.StringValue("value1")})
	for _, name := range []string{"properties", "custom_rules"} {
		attr := s.Attributes[name].(schema.MapAttribute)
		planned, _ := mapPlan(ctx, attr, priorProps, types.MapUnknown(types.StringType))
		if planned.IsUnknown() || !planned.Equal(priorProps) {
			t.Fatalf("%s: expected UseStateForUnknown to keep the prior state value, got %v", name, planned)
		}
	}

	plannedEnabled, _ := boolPlan(ctx, s.Attributes["enabled"].(schema.BoolAttribute), types.BoolValue(true), types.BoolUnknown())
	if plannedEnabled.IsUnknown() || !plannedEnabled.ValueBool() {
		t.Fatalf("enabled: expected UseStateForUnknown to keep the prior state value, got %v", plannedEnabled)
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

	// supported_object_types must reject values outside the spec enum and empty
	// sets (minItems: 1 in policies.yaml).
	setAttr := s.Attributes["supported_object_types"].(schema.SetAttribute)
	if len(setAttr.Validators) == 0 {
		t.Fatal("supported_object_types: expected validators (minItems 1 + object type enum)")
	}
	setResp := &validator.SetResponse{}
	for _, v := range setAttr.Validators {
		v.ValidateSet(ctx, validator.SetRequest{
			ConfigValue: types.SetValueMust(types.StringType, []attr.Value{types.StringValue("BOGUS")}),
			Path:        path.Root("supported_object_types"),
		}, setResp)
	}
	if !setResp.Diagnostics.HasError() {
		t.Fatal("supported_object_types: expected an enum validator rejecting an unknown object type")
	}

	emptyResp := &validator.SetResponse{}
	for _, v := range setAttr.Validators {
		v.ValidateSet(ctx, validator.SetRequest{
			ConfigValue: types.SetValueMust(types.StringType, []attr.Value{}),
			Path:        path.Root("supported_object_types"),
		}, emptyResp)
	}
	if !emptyResp.Diagnostics.HasError() {
		t.Fatal("supported_object_types: expected a validator rejecting an empty set (minItems: 1)")
	}

	// The enum validator must reject values that are not in policies.yaml.
	typeAttr := s.Attributes["policy_type"].(schema.StringAttribute)
	vResp := &validator.StringResponse{}
	for _, val := range typeAttr.Validators {
		val.ValidateString(ctx, validator.StringRequest{ConfigValue: types.StringValue("bogus"), Path: path.Root("policy_type")}, vResp)
	}
	if !vResp.Diagnostics.HasError() {
		t.Fatal("policy_type: expected an enum validator rejecting an unknown policy type")
	}
}

// nonNullRaw stands in for the raw state/plan/config values that the framework
// passes to plan modifiers; the modifiers only inspect IsNull() on them.
var nonNullRaw = tftypes.NewValue(tftypes.String, "state")

func stringPlan(ctx context.Context, attr schema.StringAttribute, state, plan types.String) (types.String, bool) {
	req := planmodifier.StringRequest{
		State:      tfsdk.State{Raw: nonNullRaw},
		Plan:       tfsdk.Plan{Raw: nonNullRaw},
		Config:     tfsdk.Config{Raw: nonNullRaw},
		StateValue: state,
		PlanValue:  plan,
		// A computed attribute is unconfigured, so its config value is null;
		// UseStateForUnknown returns early for unknown config values.
		ConfigValue: types.StringNull(),
	}
	resp := &planmodifier.StringResponse{PlanValue: plan}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyString(ctx, req, resp)
	}
	return resp.PlanValue, resp.RequiresReplace
}

func mapPlan(ctx context.Context, attr schema.MapAttribute, state, plan types.Map) (types.Map, bool) {
	req := planmodifier.MapRequest{
		State:       tfsdk.State{Raw: nonNullRaw},
		Plan:        tfsdk.Plan{Raw: nonNullRaw},
		Config:      tfsdk.Config{Raw: nonNullRaw},
		StateValue:  state,
		PlanValue:   plan,
		ConfigValue: types.MapNull(types.StringType),
	}
	resp := &planmodifier.MapResponse{PlanValue: plan}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyMap(ctx, req, resp)
	}
	return resp.PlanValue, resp.RequiresReplace
}

func boolPlan(ctx context.Context, attr schema.BoolAttribute, state, plan types.Bool) (types.Bool, bool) {
	req := planmodifier.BoolRequest{
		State:       tfsdk.State{Raw: nonNullRaw},
		Plan:        tfsdk.Plan{Raw: nonNullRaw},
		Config:      tfsdk.Config{Raw: nonNullRaw},
		StateValue:  state,
		PlanValue:   plan,
		ConfigValue: types.BoolNull(),
	}
	resp := &planmodifier.BoolResponse{PlanValue: plan}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyBool(ctx, req, resp)
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

func newServer(t *testing.T, handler http.HandlerFunc) (*client.Client, *res.PolicyResource) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	r := res.New()
	r.(*res.PolicyResource).SetClient(c)
	return c, r.(*res.PolicyResource)
}

// TestPolicyResource_Create asserts the create body equals PolicyCreateRequest
// of policies.yaml (the spec's `PolicyCreate` example), with the string-typed
// Terraform schema rendering the numeric rule1 as "123".
func TestPolicyResource_Create(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)

	var method, requestPath string
	var rawBody []byte
	_, r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		method, requestPath = req.Method, req.URL.Path
		rawBody, _ = io.ReadAll(req.Body)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, policyResponse)
	})

	plan := baseModel()
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if method != http.MethodPost {
		t.Fatalf("expected POST, got %s", method)
	}
	if want := "/api/metalakes/my_test_metalake/policies"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}

	var body map[string]any
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("invalid request body %s: %v", rawBody, err)
	}
	assertJSONEqual(t, map[string]any{
		"name":       "my_policy1",
		"comment":    "This is a test policy",
		"policyType": "custom",
		"enabled":    false,
		"content": map[string]any{
			"customRules":          map[string]any{"rule1": "123"},
			"supportedObjectTypes": []any{"CATALOG", "SCHEMA", "TABLE", "FILESET", "TOPIC", "MODEL"},
			"properties":           map[string]any{"key1": "value1"},
		},
	}, body)

	// The numeric rule1 of the spec's response must decode into the string map.
	var state res.PolicyResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.ID.ValueString() != "my_test_metalake.my_policy1" {
		t.Fatalf("unexpected id: %s", state.ID.ValueString())
	}
	if got := state.CustomRules.Elements()["rule1"]; !got.Equal(types.StringValue("123")) {
		t.Fatalf("expected the numeric custom rule to render as \"123\", got %v", got)
	}
	if state.Properties.IsUnknown() || state.SupportedObjectTypes.IsUnknown() || state.Audit.IsNull() {
		t.Fatalf("computed attributes must be known after create: %v", state)
	}
}

// TestPolicyResource_ReadNormalizesObjectTypes covers the measured Gravitino
// 1.3.0 behaviour of returning `supportedObjectTypes` lower-cased: the state
// must keep the canonical upper-case enum values of the schema, otherwise the
// apply result is inconsistent with the plan.
func TestPolicyResource_ReadNormalizesObjectTypes(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)

	_, r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{
  "code": 0,
  "policy": {
    "name": "pp1",
    "policyType": "custom",
    "enabled": true,
    "content": {
      "customRules": {"r": "v"},
      "supportedObjectTypes": ["catalog"]
    },
    "inherited": null,
    "audit": {"creator": "anonymous", "createTime": "2025-08-04T10:29:23.463Z"}
  }
}`)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_policy1")
	state.Name = types.StringValue("pp1")
	state.Enabled = types.BoolValue(true)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var newState res.PolicyResourceModel
	if diags := resp.State.Get(ctx, &newState); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if !newState.SupportedObjectTypes.Equal(allObjectTypesOf("CATALOG")) {
		t.Fatalf("expected the lower-cased server value to be normalised to CATALOG, got %v", newState.SupportedObjectTypes)
	}
}

func allObjectTypesOf(values ...string) types.Set {
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	return types.SetValueMust(types.StringType, elems)
}

func TestPolicyResource_ReadNotFoundRemovesState(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)

	_, r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchPolicyError)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_policy1")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 must not produce an error, got: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected the resource to be removed from state on 404")
	}
}

// TestPolicyResource_UpdateContent asserts the effective updateContent payload:
// PUT /metalakes/{metalake}/policies/{policy} with a PolicyUpdatesRequest
// envelope, exactly as in the updateContent example of policies.yaml.
func TestPolicyResource_UpdateContent(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)

	var method, requestPath string
	var updates []map[string]any
	_, r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		method, requestPath = req.Method, req.URL.Path
		var body struct {
			Updates []map[string]any `json:"updates"`
		}
		json.NewDecoder(req.Body).Decode(&body)
		updates = body.Updates
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, policyResponse)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_policy1")

	plan := baseModel()
	plan.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{
		"key1": types.StringValue("new_value1"),
		"key2": types.StringValue("new_value2"),
	})
	plan.CustomRules = types.MapValueMust(types.StringType, map[string]attr.Value{"rule1": types.StringValue("456")})

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
	if want := "/api/metalakes/my_test_metalake/policies/my_policy1"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}

	assertUpdatesEqual(t, []map[string]any{{
		"@type":      "updateContent",
		"policyType": "custom",
		"newContent": map[string]any{
			"customRules":          map[string]any{"rule1": "456"},
			"supportedObjectTypes": []any{"CATALOG", "SCHEMA", "TABLE", "FILESET", "TOPIC", "MODEL"},
			"properties":           map[string]any{"key1": "new_value1", "key2": "new_value2"},
		},
	}}, updates)

	var newState res.PolicyResourceModel
	if diags := resp.State.Get(ctx, &newState); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if got := newState.CustomRules.Elements()["rule1"]; !got.Equal(types.StringValue("123")) {
		t.Fatalf("expected the response content to be recorded in state, got %v", got)
	}
	if !newState.Enabled.Equal(types.BoolValue(false)) {
		t.Fatalf("the alterPolicy response does not change `enabled`; expected false, got %v", newState.Enabled)
	}
}

// TestPolicyResource_UpdateRename proves the spec's RenamePolicyRequest is sent
// against the current name and that the resource ID follows the new name.
func TestPolicyResource_UpdateRename(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)

	var method, requestPath string
	var updates []map[string]any
	_, r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		method, requestPath = req.Method, req.URL.Path
		var body struct {
			Updates []map[string]any `json:"updates"`
		}
		json.NewDecoder(req.Body).Decode(&body)
		updates = body.Updates
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{
  "code": 0,
  "policy": {
    "name": "my_policy_new",
    "comment": "This is a test policy",
    "policyType": "custom",
    "enabled": false,
    "content": {
      "customRules": {"rule1": 123},
      "supportedObjectTypes": ["SCHEMA", "TABLE", "MODEL", "TOPIC", "FILESET", "CATALOG"],
      "properties": {"key1": "value1"}
    },
    "inherited": null,
    "audit": {"creator": "anonymous", "createTime": "2025-08-04T10:29:23.463Z"}
  }
}`)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_policy1")

	plan := baseModel()
	plan.Name = types.StringValue("my_policy_new")

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
	if want := "/api/metalakes/my_test_metalake/policies/my_policy1"; requestPath != want {
		t.Fatalf("a rename must target the current name; expected path %s, got %s", want, requestPath)
	}
	assertUpdatesEqual(t, []map[string]any{{"@type": "rename", "newName": "my_policy_new"}}, updates)

	var newState res.PolicyResourceModel
	if diags := resp.State.Get(ctx, &newState); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if newState.Name.ValueString() != "my_policy_new" {
		t.Fatalf("unexpected name after rename: %s", newState.Name.ValueString())
	}
	if newState.ID.ValueString() != "my_test_metalake.my_policy_new" {
		t.Fatalf("unexpected id after rename: %s", newState.ID.ValueString())
	}
}

func TestPolicyResource_UpdateComment(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)

	var method, requestPath string
	var updates []map[string]any
	_, r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		method, requestPath = req.Method, req.URL.Path
		var body struct {
			Updates []map[string]any `json:"updates"`
		}
		json.NewDecoder(req.Body).Decode(&body)
		updates = body.Updates
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{
  "code": 0,
  "policy": {
    "name": "my_policy1",
    "comment": "This is my new policy comment",
    "policyType": "custom",
    "enabled": false,
    "content": {
      "customRules": {"rule1": 123},
      "supportedObjectTypes": ["SCHEMA", "TABLE", "MODEL", "TOPIC", "FILESET", "CATALOG"],
      "properties": {"key1": "value1"}
    },
    "inherited": null,
    "audit": {"creator": "anonymous", "createTime": "2025-08-04T10:29:23.463Z"}
  }
}`)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_policy1")

	plan := baseModel()
	plan.Comment = types.StringValue("This is my new policy comment")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if method != http.MethodPut || requestPath != "/api/metalakes/my_test_metalake/policies/my_policy1" {
		t.Fatalf("unexpected request %s %s", method, requestPath)
	}
	assertUpdatesEqual(t, []map[string]any{{"@type": "updateComment", "newComment": "This is my new policy comment"}}, updates)

	var newState res.PolicyResourceModel
	if diags := resp.State.Get(ctx, &newState); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if newState.Comment.ValueString() != "This is my new policy comment" {
		t.Fatalf("unexpected comment: %s", newState.Comment.ValueString())
	}
}

// TestPolicyResource_UpdateEnabled proves `enabled` is toggled through
// PATCH with the spec's PolicySetRequest body ({"enable": ...}) and that the
// BaseResponse is accepted.
func TestPolicyResource_UpdateEnabled(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)

	var putCalls, patchCalls int
	var patchBody map[string]any
	_, r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPut:
			putCalls++
		case http.MethodPatch:
			patchCalls++
			if req.URL.Path != "/api/metalakes/my_test_metalake/policies/my_policy1" {
				t.Errorf("unexpected patch path %s", req.URL.Path)
			}
			json.NewDecoder(req.Body).Decode(&patchBody)
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		// BaseResponse of openapi.yaml.
		fmt.Fprint(w, `{"code": 0}`)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_policy1")
	state.Enabled = types.BoolValue(false)

	plan := baseModel()
	plan.Enabled = types.BoolValue(true)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if putCalls != 0 {
		t.Fatalf("an enable toggle must not send an alterPolicy request, got %d PUT calls", putCalls)
	}
	if patchCalls != 1 {
		t.Fatalf("expected exactly one PATCH, got %d", patchCalls)
	}
	// PolicySetRequest: the field is `enable`, not `enabled`.
	assertJSONEqual(t, map[string]any{"enable": true}, patchBody)

	var newState res.PolicyResourceModel
	if diags := resp.State.Get(ctx, &newState); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if !newState.Enabled.ValueBool() {
		t.Fatal("expected enabled to be true after the update")
	}
	if newState.Comment.IsUnknown() || newState.Properties.IsUnknown() || newState.Audit.IsUnknown() {
		t.Fatalf("no computed value may stay unknown after an enable toggle: %v", newState)
	}
}

// TestPolicyResource_UpdateNothingChangedIdempotent covers the "nothing
// changed" Update branch: no request is sent and no computed value is unknown.
func TestPolicyResource_UpdateNothingChangedIdempotent(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)

	calls := 0
	_, r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, policyResponse)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_policy1")

	// The plan could not resolve the computed attributes (unknown); state holds
	// the values the server returned earlier.
	plan := baseModel()
	plan.Comment = types.StringUnknown()
	plan.Properties = types.MapUnknown(types.StringType)
	plan.CustomRules = types.MapUnknown(types.StringType)
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

	var newState res.PolicyResourceModel
	if diags := resp.State.Get(ctx, &newState); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if newState.Comment.IsUnknown() || newState.Properties.IsUnknown() || newState.Audit.IsUnknown() {
		t.Fatalf("computed values resolved from state must be known, got %v", newState)
	}
	if newState.Comment.ValueString() != "This is a test policy" {
		t.Fatalf("expected the state comment to be kept, got %v", newState.Comment)
	}
	if got := newState.Properties.Elements()["key1"]; !got.Equal(types.StringValue("value1")) {
		t.Fatalf("expected the state properties to be kept, got %v", newState.Properties)
	}
	if newState.ID.ValueString() != "my_test_metalake.my_policy1" {
		t.Fatalf("unexpected id: %s", newState.ID.ValueString())
	}
}

func TestPolicyResource_Delete(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)

	var method, requestPath string
	_, r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		method, requestPath = req.Method, req.URL.Path
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "dropped": true})
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_policy1")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if method != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", method)
	}
	if want := "/api/metalakes/my_test_metalake/policies/my_policy1"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}
}

func TestPolicyResource_DeleteNotFoundIsSuccess(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)

	_, r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchPolicyError)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_policy1")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("deleting an already deleted policy must succeed, got: %v", resp.Diagnostics)
	}
}

func TestPolicyResource_ImportState(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)
	r := res.New().(resource.ResourceWithImportState)

	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, baseModel())}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "my_metalake.my_policy"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state res.PolicyResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.Metalake.ValueString() != "my_metalake" || state.Name.ValueString() != "my_policy" {
		t.Fatalf("unexpected import result: metalake=%s name=%s", state.Metalake.ValueString(), state.Name.ValueString())
	}
	if state.ID.ValueString() != "my_metalake.my_policy" {
		t.Fatalf("unexpected id: %s", state.ID.ValueString())
	}
}

func TestPolicyResource_ImportState_Invalid(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)
	r := res.New().(resource.ResourceWithImportState)

	for _, id := range []string{"no_dot_here", "trailing.", ".leading", ""} {
		resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, baseModel())}}
		r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected an error for invalid import ID %q", id)
		}
	}
}

// TestPolicyResource_ModifyPlanMarksIDUnknownOnRename proves the ID is planned
// as unknown when the name changes, so the new ID computed by Update is not an
// inconsistent result.
func TestPolicyResource_ModifyPlanMarksIDUnknownOnRename(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)
	r := res.New().(resource.ResourceWithModifyPlan)

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_policy1")

	plan := baseModel()
	plan.Name = types.StringValue("my_policy_new")
	plan.ID = types.StringValue("my_test_metalake.my_policy1")

	resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)}}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var planned res.PolicyResourceModel
	if diags := resp.Plan.Get(ctx, &planned); diags.HasError() {
		t.Fatalf("failed to read plan: %v", diags)
	}
	if !planned.ID.IsUnknown() {
		t.Fatalf("expected the planned id to be unknown on rename, got %v", planned.ID)
	}

	// Without a rename the prior ID is kept.
	plan = baseModel()
	plan.ID = types.StringValue("my_test_metalake.my_policy1")
	resp = &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)}}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if diags := resp.Plan.Get(ctx, &planned); diags.HasError() {
		t.Fatalf("failed to read plan: %v", diags)
	}
	if planned.ID.ValueString() != "my_test_metalake.my_policy1" {
		t.Fatalf("expected the planned id to be unchanged, got %v", planned.ID)
	}
}

// TestPolicyResource_UpdateRenameAndEnable proves the two requests that a
// rename plus an enable toggle require: the alterPolicy request targets the
// current name, the setPolicy request the new one.
func TestPolicyResource_UpdateRenameAndEnable(t *testing.T) {
	ctx := context.Background()
	s := policySchema(t)

	var calls []string
	var patchBody []byte
	_, r := newServer(t, func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		if req.Method == http.MethodPatch {
			patchBody, _ = io.ReadAll(req.Body)
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{
  "code": 0,
  "policy": {
    "name": "my_policy_new",
    "comment": "This is a test policy",
    "policyType": "custom",
    "enabled": false,
    "content": {
      "customRules": {"rule1": 123},
      "supportedObjectTypes": ["SCHEMA", "TABLE", "MODEL", "TOPIC", "FILESET", "CATALOG"],
      "properties": {"key1": "value1"}
    },
    "inherited": null,
    "audit": {"creator": "anonymous", "createTime": "2025-08-04T10:29:23.463Z"}
  }
}`)
	})

	state := baseModel()
	state.ID = types.StringValue("my_test_metalake.my_policy1")

	plan := baseModel()
	plan.Name = types.StringValue("my_policy_new")
	plan.Enabled = types.BoolValue(true)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	wantCalls := []string{
		"PUT /api/metalakes/my_test_metalake/policies/my_policy1",
		"PATCH /api/metalakes/my_test_metalake/policies/my_policy_new",
	}
	if strings.Join(calls, "\n") != strings.Join(wantCalls, "\n") {
		t.Fatalf("unexpected calls:\n want %v\n  got %v", wantCalls, calls)
	}
	assertJSONEqual(t, map[string]any{"enable": true}, decodeBody(t, patchBody))

	var newState res.PolicyResourceModel
	if diags := resp.State.Get(ctx, &newState); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if !newState.Enabled.ValueBool() {
		t.Fatal("expected the toggled value to be recorded in state")
	}
	if newState.ID.ValueString() != "my_test_metalake.my_policy_new" {
		t.Fatalf("unexpected id: %s", newState.ID.ValueString())
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
