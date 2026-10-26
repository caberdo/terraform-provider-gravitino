package policy_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/policy"
	"github.com/gravitino/terraform-provider-gravitino/internal/models/modeltest"
	"github.com/gravitino/terraform-provider-gravitino/internal/resources/resourcetest"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// policyListResponse is the verbatim `PolicyListResponse` example of
// policies.yaml (components/examples/PolicyListResponse). Its customRules hold
// the JSON number 123, which the string-typed schema renders as "123".
const policyListResponse = `{
  "code": 0,
  "policies": [
    {
      "name": "my_policy1",
      "comment": "This is a test policy",
      "policyType": "custom",
      "enabled": false,
      "content": {
        "customRules": {
          "rule1": 123
        },
        "supportedObjectTypes": [
          "CATALOG",
          "SCHEMA",
          "TABLE",
          "FILESET",
          "TOPIC",
          "MODEL"
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
  ]
}`

// noSuchMetalakeError is the verbatim `NoSuchMetalakeException` example of
// metalakes.yaml, returned by the Gravitino server with HTTP status 404.
const noSuchMetalakeError = `{
  "code": 1003,
  "type": "NoSuchMetalakeException",
  "message": "Metalake my_test_metalake does not exist",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchMetalakeException: Metalake my_test_metalake does not exist",
    "..."
  ]
}`

func policiesSchema(t *testing.T) schema.Schema {
	t.Helper()
	d := ds.NewListDataSource()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func tfValue(t *testing.T, ctx context.Context, s schema.Schema, model ds.PoliciesDataSourceModel) tftypes.Value {
	return resourcetest.TFValue(t, ctx, s, model)
}

func TestPoliciesDataSource_Metadata(t *testing.T) {
	d := ds.NewListDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_policies" {
		t.Fatalf("expected gravitino_policies, got %s", resp.TypeName)
	}
}

func TestPoliciesDataSource_Read(t *testing.T) {
	ctx := context.Background()
	s := policiesSchema(t)

	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, policyListResponse)
	}))
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	d := ds.NewListDataSource()
	d.(*ds.PoliciesDataSource).SetClient(c)

	config := ds.PoliciesDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Policies: types.ListNull(types.ObjectType{AttrTypes: ds.PolicyItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tfValue(t, ctx, s, config)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if want := "/api/metalakes/my_test_metalake/policies"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}

	var state ds.PoliciesDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	items := state.Policies.Elements()
	if len(items) != 1 {
		t.Fatalf("expected one policy, got %d", len(items))
	}
	attrs := items[0].(types.Object).Attributes()
	if got := attrs["name"].(types.String).ValueString(); got != "my_policy1" {
		t.Fatalf("unexpected name: %s", got)
	}
	if got := attrs["policy_type"].(types.String).ValueString(); got != "custom" {
		t.Fatalf("unexpected policy_type: %s", got)
	}
	if attrs["enabled"].(types.Bool).ValueBool() {
		t.Fatal("expected enabled=false from the spec example")
	}
	// The numeric customRules value of the spec example must decode (and render
	// as its JSON encoding) instead of failing the whole conversion.
	if got := attrs["custom_rules"].(types.Map).Elements()["rule1"]; !got.Equal(types.StringValue("123")) {
		t.Fatalf("expected the numeric custom rule to render as \"123\", got %v", got)
	}
	if got := attrs["properties"].(types.Map).Elements()["key1"]; !got.Equal(types.StringValue("value1")) {
		t.Fatalf("unexpected properties: %v", got)
	}
	if attrs["audit"].(types.Object).IsNull() {
		t.Fatal("audit must be set from the response")
	}
}

func TestPoliciesDataSource_ReadMetalakeNotFound(t *testing.T) {
	ctx := context.Background()
	s := policiesSchema(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchMetalakeError)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewListDataSource()
	d.(*ds.PoliciesDataSource).SetClient(c)

	config := ds.PoliciesDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Policies: types.ListNull(types.ObjectType{AttrTypes: ds.PolicyItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tfValue(t, ctx, s, config)}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic when the metalake does not exist")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Metalake not found") {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func TestPoliciesDataSource_ReadServerError(t *testing.T) {
	ctx := context.Background()
	s := policiesSchema(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalServerError","message":"boom","stack":["..."]}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewListDataSource()
	d.(*ds.PoliciesDataSource).SetClient(c)

	config := ds.PoliciesDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Policies: types.ListNull(types.ObjectType{AttrTypes: ds.PolicyItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tfValue(t, ctx, s, config)}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed listing policies "my_test_metalake"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func TestPoliciesDataSource_Configure(t *testing.T) {
	d := ds.NewListDataSource()
	configured := d.(datasource.DataSourceWithConfigure)

	resp := &datasource.ConfigureResponse{}
	configured.Configure(context.Background(), datasource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("nil provider data must configure without diagnostics: %v", resp.Diagnostics)
	}

	resp = &datasource.ConfigureResponse{}
	configured.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for unexpected provider data")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Unexpected provider data type") {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func readPolicies(t *testing.T, handler http.HandlerFunc) ds.PoliciesDataSourceModel {
	t.Helper()
	ctx := context.Background()
	s := policiesSchema(t)

	server := httptest.NewServer(handler)
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewListDataSource()
	d.(*ds.PoliciesDataSource).SetClient(c)

	config := ds.PoliciesDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Policies: types.ListNull(types.ObjectType{AttrTypes: ds.PolicyItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tfValue(t, ctx, s, config)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state ds.PoliciesDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return state
}

// TestPoliciesDataSource_ReadEmpty asserts an empty policies array is reported as
// an empty list rather than a null one.
func TestPoliciesDataSource_ReadEmpty(t *testing.T) {
	state := readPolicies(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{"code":0,"policies":[]}`)
	})

	if state.Policies.IsNull() {
		t.Fatal("an empty result must not be a null list")
	}
	if len(state.Policies.Elements()) != 0 {
		t.Fatalf("expected no policies, got %d", len(state.Policies.Elements()))
	}
}

// TestPoliciesDataSource_ReadPolicyWithoutContent covers a policy that carries no
// content object.
func TestPoliciesDataSource_ReadPolicyWithoutContent(t *testing.T) {
	state := readPolicies(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{"code":0,"policies":[{"name":"my_policy1","comment":"","policyType":"custom","enabled":true}]}`)
	})

	elements := state.Policies.Elements()
	if len(elements) != 1 {
		t.Fatalf("expected one policy, got %d", len(elements))
	}
	attrs := elements[0].(types.Object).Attributes()
	if !attrs["enabled"].(types.Bool).ValueBool() {
		t.Error("enabled = false, want true")
	}
	if !attrs["properties"].(types.Map).IsNull() {
		t.Errorf("properties = %v, want null", attrs["properties"])
	}
	if !attrs["custom_rules"].(types.Map).IsNull() {
		t.Errorf("custom_rules = %v, want null", attrs["custom_rules"])
	}
}

// TestPolicyDataSourceModelsMatchSchema keeps the models in lockstep with the
// gravitino_policy, gravitino_policies, gravitino_object_policies and
// gravitino_policy_objects schemas.
func TestPolicyDataSourceModelsMatchSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	ds.NewDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	modeltest.AssertSchemaMatches(t, ds.PolicyDataSourceModel{}, resp.Schema.Attributes, resp.Schema.Blocks)

	resp = &datasource.SchemaResponse{}
	ds.NewListDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	modeltest.AssertSchemaMatches(t, ds.PoliciesDataSourceModel{}, resp.Schema.Attributes, resp.Schema.Blocks)

	policies, ok := resp.Schema.Attributes["policies"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("policies is %T, want a ListNestedAttribute", resp.Schema.Attributes["policies"])
	}
	if got, want := modeltest.Keys(policies.NestedObject.Attributes), modeltest.Keys(ds.PolicyItemAttrTypes); !slices.Equal(got, want) {
		t.Errorf("policies entry attributes = %v, PolicyItemAttrTypes = %v", got, want)
	}

	resp = &datasource.SchemaResponse{}
	ds.NewObjectPoliciesDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	modeltest.AssertSchemaMatches(t, ds.ObjectPoliciesDataSourceModel{}, resp.Schema.Attributes, resp.Schema.Blocks)

	resp = &datasource.SchemaResponse{}
	ds.NewPolicyObjectsDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	modeltest.AssertSchemaMatches(t, ds.PolicyObjectsDataSourceModel{}, resp.Schema.Attributes, resp.Schema.Blocks)

	objects, ok := resp.Schema.Attributes["objects"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("objects is %T, want a ListNestedAttribute", resp.Schema.Attributes["objects"])
	}
	if got, want := modeltest.Keys(objects.NestedObject.Attributes), modeltest.Keys(ds.PolicyObjectItemAttrTypes); !slices.Equal(got, want) {
		t.Errorf("objects entry attributes = %v, PolicyObjectItemAttrTypes = %v", got, want)
	}
}
