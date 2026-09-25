package policy_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/policy"

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
