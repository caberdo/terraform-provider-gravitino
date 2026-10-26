package policy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/policy"
	"github.com/gravitino/terraform-provider-gravitino/internal/resources/resourcetest"
	"github.com/gravitino/terraform-provider-gravitino/internal/tfutil"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Payloads below are copied literally from the Gravitino v1.3.1 OpenAPI spec
// (docs/open-api/policies.yaml).
const (
	policyAssociationJSONMedia = "application/vnd.gravitino.v1+json"

	policyAssociationBasePath = "/api/metalakes/test_metalake/objects/TABLE/sales.orders/policies"
	policyGetPath             = "/api/metalakes/test_metalake/policies/my_policy1"
	policyMismatchGetPath     = "/api/metalakes/test_metalake/policies/pii_only"

	// components/examples/PolicyAssociate
	specPolicyAssociate = `{
  "policiesToAdd": ["my_policy1", "my_policy2"],
  "policiesToRemove": ["my_policy3"]
}`

	// components/examples/NameListResponse
	specPolicyAssociationNames = `{
  "code": 0,
  "names": ["my_policy1", "my_policy2"]
}`

	// components/examples/PolicyResponse. The supportedObjectTypes list includes
	// TABLE, so my_policy1 can be attached to a TABLE.
	specPolicyAssociationResponse = `{
  "code": 0,
  "policy": {
    "name": "my_policy1",
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
}`

	// components/examples/PolicyResponse with the supportedObjectTypes list
	// narrowed to CATALOG/SCHEMA so a TABLE association is a genuine mismatch.
	specPolicyMismatchResponse = `{
  "code": 0,
  "policy": {
    "name": "pii_only",
    "comment": "This is a test policy",
    "policyType": "custom",
    "enabled": false,
    "content": {
      "customRules": {"rule1": 123},
      "supportedObjectTypes": ["CATALOG", "SCHEMA"],
      "properties": {"key1": "value1"}
    },
    "inherited": null,
    "audit": {"creator": "anonymous", "createTime": "2025-08-04T10:29:23.463Z"}
  }
}`

	// components/examples/PolicyListResponse with one inherited entry added so
	// the Read filter can be exercised. The field is documented nullable by the
	// spec; a direct association reports false/null.
	specPolicyAssociationList = `{
  "code": 0,
  "policies": [
    {
      "name": "my_policy1",
      "comment": "This is a test policy",
      "policyType": "custom",
      "enabled": false,
      "content": {
        "customRules": {"rule1": 123},
        "supportedObjectTypes": ["CATALOG", "SCHEMA", "TABLE"],
        "properties": {"key1": "value1"}
      },
      "inherited": false,
      "audit": {"creator": "anonymous", "createTime": "2025-08-04T10:29:23.463Z"}
    },
    {
      "name": "parent_policy",
      "comment": "Inherited from the catalog",
      "policyType": "custom",
      "enabled": true,
      "content": {
        "customRules": {"rule1": 123},
        "supportedObjectTypes": ["CATALOG", "SCHEMA", "TABLE"],
        "properties": {"key1": "value1"}
      },
      "inherited": true,
      "audit": {"creator": "anonymous", "createTime": "2025-08-04T10:29:23.463Z"}
    }
  ]
}`

	// components/examples/NoSuchPolicyException
	specNoSuchPolicyException = `{
  "code": 1003,
  "type": "NoSuchPolicyException",
  "message": "Failed to operate policy(s) [my_policy] operation [LOAD] under metalake [my_test_metalake], reason [NoSuchPolicyException]",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchPolicyException: Policy my_policy does not exist",
    "..."
  ]
}`
)

type policyAssociationRecorder struct {
	mu       sync.Mutex
	requests []policyAssociationRequest
}

type policyAssociationRequest struct {
	Method   string
	Path     string
	RawQuery string
	Body     map[string]interface{}
}

func (rec *policyAssociationRecorder) record(r *http.Request) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var body map[string]interface{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	rec.requests = append(rec.requests, policyAssociationRequest{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Body:     body,
	})
}

func (rec *policyAssociationRecorder) count(method string) int {
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

func (rec *policyAssociationRecorder) only(t *testing.T, method string) policyAssociationRequest {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var found []policyAssociationRequest
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

func newPolicyAssociation(t *testing.T, rec *policyAssociationRecorder, handler func(w http.ResponseWriter, r *http.Request)) *res.PolicyAssociationResource {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", policyAssociationJSONMedia)
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	r := res.NewAssociation()
	r.(*res.PolicyAssociationResource).SetClient(c)
	return r.(*res.PolicyAssociationResource)
}

func policyAssociationSchema(t *testing.T) schema.Schema {
	t.Helper()
	r := res.NewAssociation()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func policyAssociationModel(t *testing.T, objectType, objectFullName string, policies ...string) res.PolicyAssociationResourceModel {
	t.Helper()
	set, diags := tfutil.StringsToSet(policies)
	if diags.HasError() {
		t.Fatalf("policies set: %v", diags)
	}
	return res.PolicyAssociationResourceModel{
		ID:             types.StringUnknown(),
		Metalake:       types.StringValue("test_metalake"),
		ObjectType:     types.StringValue(objectType),
		ObjectFullName: types.StringValue(objectFullName),
		Policies:       set,
	}
}

func policyAssociationValue(t *testing.T, s schema.Schema, model res.PolicyAssociationResourceModel) tftypes.Value {
	t.Helper()
	return resourcetest.TFValue(t, context.Background(), s, model)
}

func TestPolicyAssociationResource_Metadata(t *testing.T) {
	r := res.NewAssociation()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_policy_association" {
		t.Fatalf("type name = %q, want gravitino_policy_association", resp.TypeName)
	}
}

func TestPolicyAssociationResource_Schema(t *testing.T) {
	s := policyAssociationSchema(t)
	for _, name := range []string{"id", "metalake", "object_type", "object_full_name", "policies"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	policies, ok := s.Attributes["policies"].(schema.SetAttribute)
	if !ok {
		t.Fatal("policies must be a set attribute")
	}
	if !policies.Required {
		t.Error("policies must be required")
	}

	for _, name := range []string{"metalake", "object_type", "object_full_name"} {
		attr, ok := s.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s must be a string attribute", name)
		}
		if len(attr.PlanModifiers) == 0 {
			t.Errorf("%s must carry RequiresReplace", name)
		}
	}

	id, ok := s.Attributes["id"].(schema.StringAttribute)
	if !ok || len(id.PlanModifiers) != 1 {
		t.Fatalf("id must carry exactly one plan modifier, got %#v", s.Attributes["id"])
	}
	modifier, ok := id.PlanModifiers[0].(models.CompoundIDPlanModifier)
	if !ok {
		t.Fatalf("id plan modifier = %T, want models.CompoundIDPlanModifier", id.PlanModifiers[0])
	}
	if got := modifier.Components; len(got) != 3 || got[0] != "metalake" || got[1] != "object_type" || got[2] != "object_full_name" {
		t.Errorf("id components = %v", got)
	}
}

func TestPolicyAssociationResource_Create_ChecksPolicyAndSendsPoliciesToAdd(t *testing.T) {
	rec := &policyAssociationRecorder{}
	r := newPolicyAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && (req.URL.Path == policyGetPath || req.URL.Path == "/api/metalakes/test_metalake/policies/my_policy2"):
			io.WriteString(w, specPolicyAssociationResponse)
		case req.Method == http.MethodPost && req.URL.Path == policyAssociationBasePath:
			io.WriteString(w, specPolicyAssociationNames)
		default:
			http.NotFound(w, req)
		}
	})

	s := policyAssociationSchema(t)
	ctx := context.Background()
	plan := policyAssociationModel(t, "TABLE", "sales.orders", "my_policy1")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: policyAssociationValue(t, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	sent := rec.only(t, http.MethodPost)
	want := map[string]interface{}{"policiesToAdd": []interface{}{"my_policy1"}}
	resourcetest.AssertJSONEqualUnordered(t, string(mustJSON(t, want)), mustJSON(t, sent.Body))

	var got res.PolicyAssociationResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.ID.ValueString() != "test_metalake.TABLE.sales.orders" {
		t.Errorf("id = %q", got.ID.ValueString())
	}
	if names := tfutil.SetToStrings(got.Policies); !sameSet(names, []string{"my_policy1"}) {
		t.Errorf("policies = %v", names)
	}
}

func TestPolicyAssociationResource_Create_RejectsMismatchedObjectType(t *testing.T) {
	rec := &policyAssociationRecorder{}
	r := newPolicyAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == policyMismatchGetPath {
			io.WriteString(w, specPolicyMismatchResponse)
			return
		}
		http.NotFound(w, req)
	})

	s := policyAssociationSchema(t)
	ctx := context.Background()
	plan := policyAssociationModel(t, "TABLE", "sales.orders", "pii_only")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: policyAssociationValue(t, s, plan)}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a mismatch diagnostic")
	}
	if n := rec.count(http.MethodPost); n != 0 {
		t.Errorf("the association endpoint must not be called on a mismatch, got %d POSTs", n)
	}
	if !diagnosticContains(resp.Diagnostics, "CATALOG, SCHEMA") {
		t.Errorf("diagnostics = %v, want the policy's allowed types named", resp.Diagnostics)
	}
}

func TestPolicyAssociationResource_Create_PolicyNotFound(t *testing.T) {
	rec := &policyAssociationRecorder{}
	r := newPolicyAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, specNoSuchPolicyException)
	})

	s := policyAssociationSchema(t)
	ctx := context.Background()
	plan := policyAssociationModel(t, "TABLE", "sales.orders", "missing_policy")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: policyAssociationValue(t, s, plan)}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a policy-not-found diagnostic")
	}
	if n := rec.count(http.MethodPost); n != 0 {
		t.Errorf("the association endpoint must not be called, got %d POSTs", n)
	}
}

func TestPolicyAssociationResource_Update_SendsDiff(t *testing.T) {
	rec := &policyAssociationRecorder{}
	r := newPolicyAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && (req.URL.Path == policyGetPath || req.URL.Path == "/api/metalakes/test_metalake/policies/my_policy2"):
			io.WriteString(w, specPolicyAssociationResponse)
		case req.Method == http.MethodPost && req.URL.Path == policyAssociationBasePath:
			io.WriteString(w, specPolicyAssociationNames)
		default:
			http.NotFound(w, req)
		}
	})

	s := policyAssociationSchema(t)
	ctx := context.Background()
	state := policyAssociationModel(t, "TABLE", "sales.orders", "my_policy3")
	plan := policyAssociationModel(t, "TABLE", "sales.orders", "my_policy1", "my_policy2")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: policyAssociationValue(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: policyAssociationValue(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	sent := rec.only(t, http.MethodPost)
	// The state/plan diff is exactly the spec's PolicyAssociate example.
	resourcetest.AssertJSONEqualUnordered(t, specPolicyAssociate, mustJSON(t, sent.Body))
}

func TestPolicyAssociationResource_Read_IgnoresInheritedPolicies(t *testing.T) {
	rec := &policyAssociationRecorder{}
	r := newPolicyAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == policyAssociationBasePath {
			io.WriteString(w, specPolicyAssociationList)
			return
		}
		http.NotFound(w, req)
	})

	s := policyAssociationSchema(t)
	ctx := context.Background()
	state := policyAssociationModel(t, "TABLE", "sales.orders", "my_policy1", "parent_policy")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: policyAssociationValue(t, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: policyAssociationValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	get := rec.only(t, http.MethodGet)
	if get.RawQuery != "details=true" {
		t.Errorf("query = %q, want details=true so inherited can be told apart", get.RawQuery)
	}

	var got res.PolicyAssociationResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if names := tfutil.SetToStrings(got.Policies); !sameSet(names, []string{"my_policy1"}) {
		t.Errorf("policies = %v, want only the direct policy my_policy1", names)
	}
}

func TestPolicyAssociationResource_Read_NotFoundRemovesState(t *testing.T) {
	rec := &policyAssociationRecorder{}
	r := newPolicyAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, specNoSuchPolicyException)
	})

	s := policyAssociationSchema(t)
	ctx := context.Background()
	state := policyAssociationModel(t, "TABLE", "sales.orders", "my_policy1")
	raw := policyAssociationValue(t, s, state)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 read must not error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("404 read must remove the resource from state")
	}
}

func TestPolicyAssociationResource_Delete_RemovesEveryPolicyInState(t *testing.T) {
	rec := &policyAssociationRecorder{}
	r := newPolicyAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			io.WriteString(w, `{"code":0,"names":[]}`)
			return
		}
		http.NotFound(w, req)
	})

	s := policyAssociationSchema(t)
	ctx := context.Background()
	state := policyAssociationModel(t, "TABLE", "sales.orders", "my_policy1", "my_policy2")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: policyAssociationValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", resp.Diagnostics)
	}

	sent := rec.only(t, http.MethodPost)
	want := map[string]interface{}{"policiesToRemove": []interface{}{"my_policy1", "my_policy2"}}
	resourcetest.AssertJSONEqualUnordered(t, string(mustJSON(t, want)), mustJSON(t, sent.Body))
}

func TestPolicyAssociationResource_Delete_NotFoundIsSuccess(t *testing.T) {
	rec := &policyAssociationRecorder{}
	r := newPolicyAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, specNoSuchPolicyException)
	})

	s := policyAssociationSchema(t)
	ctx := context.Background()
	state := policyAssociationModel(t, "TABLE", "sales.orders", "my_policy1")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: policyAssociationValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 delete must be treated as success: %v", resp.Diagnostics)
	}
}

func TestPolicyAssociationResource_ImportState(t *testing.T) {
	r := res.NewAssociation().(resource.ResourceWithImportState)
	ctx := context.Background()
	s := policyAssociationSchema(t)
	nullModel := policyAssociationModel(t, "", "")
	nullModel.ID = types.StringNull()

	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: policyAssociationValue(t, s, nullModel)}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "my_metalake.TABLE.sales.orders"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	var got res.PolicyAssociationResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.ID.ValueString() != "my_metalake.TABLE.sales.orders" {
		t.Errorf("id = %q", got.ID.ValueString())
	}
	if got.ObjectFullName.ValueString() != "sales.orders" {
		t.Errorf("object_full_name = %q", got.ObjectFullName.ValueString())
	}
}

func TestPolicyAssociationResource_ImportState_Invalid(t *testing.T) {
	for _, id := range []string{"no_dots", "my_metalake.TABLE", "my_metalake.COLUMN.a", "my_metalake.ROLE.a", "my_metalake.TABLE."} {
		t.Run(id, func(t *testing.T) {
			r := res.NewAssociation().(resource.ResourceWithImportState)
			ctx := context.Background()
			s := policyAssociationSchema(t)
			nullModel := policyAssociationModel(t, "", "")
			nullModel.ID = types.StringNull()

			resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: policyAssociationValue(t, s, nullModel)}}
			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error for invalid import ID %q", id)
			}
		})
	}
}

func TestPolicyAssociationResource_Create_RejectsViewOn130Server(t *testing.T) {
	rec := &policyAssociationRecorder{}
	r := newPolicyAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/api/version" {
			io.WriteString(w, `{"code":0,"version":{"version":"1.3.0"}}`)
			return
		}
		http.NotFound(w, req)
	})

	s := policyAssociationSchema(t)
	ctx := context.Background()
	plan := policyAssociationModel(t, "VIEW", "sales.orders_view", "my_policy1")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: policyAssociationValue(t, s, plan)}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a version diagnostic for VIEW on a 1.3.0 server")
	}
	if n := rec.count(http.MethodPost); n != 0 {
		t.Errorf("the association endpoint must not be called, got %d POSTs", n)
	}
	if !diagnosticContains(resp.Diagnostics, "requires Gravitino 1.3.1") {
		t.Errorf("diagnostics = %v, want the required version named", resp.Diagnostics)
	}
}

func diagnosticContains(diags diag.Diagnostics, substr string) bool {
	for _, d := range diags {
		if strings.Contains(d.Summary(), substr) || strings.Contains(d.Detail(), substr) {
			return true
		}
	}
	return false
}

func mustJSON(t *testing.T, value interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	return strings.Join(g, "\x00") == strings.Join(w, "\x00")
}
