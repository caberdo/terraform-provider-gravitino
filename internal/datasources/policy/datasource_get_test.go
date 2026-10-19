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
	"github.com/gravitino/terraform-provider-gravitino/internal/resources/resourcetest"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// policyResponse is the verbatim `PolicyResponse` example of policies.yaml
// (components/examples/PolicyResponse).
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

func newPolicyDataSource(t *testing.T, handler http.HandlerFunc) (datasource.DataSource, *datasource.SchemaResponse) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	d := ds.NewDataSource()
	d.(*ds.PolicyDataSource).SetClient(c)

	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	return d, resp
}

func policyGetConfig() ds.PolicyDataSourceModel {
	return ds.PolicyDataSourceModel{
		Metalake:             types.StringValue("my_test_metalake"),
		Name:                 types.StringValue("my_policy1"),
		Comment:              types.StringNull(),
		PolicyType:           types.StringNull(),
		Enabled:              types.BoolNull(),
		SupportedObjectTypes: types.SetNull(types.StringType),
		Properties:           types.MapNull(types.StringType),
		CustomRules:          types.MapNull(types.StringType),
		Audit:                types.ObjectNull(ds.AuditAttrTypes),
	}
}

func readPolicyGet(t *testing.T, d datasource.DataSource, schemaResp *datasource.SchemaResponse, config ds.PolicyDataSourceModel) *datasource.ReadResponse {
	t.Helper()
	ctx := context.Background()
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: resourcetest.TFValue(t, ctx, schemaResp.Schema, config)},
	}, resp)
	return resp
}

func TestPolicyDataSource_Metadata(t *testing.T) {
	d := ds.NewDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_policy" {
		t.Fatalf("expected gravitino_policy, got %s", resp.TypeName)
	}
}

func TestPolicyDataSource_Schema(t *testing.T) {
	_, schemaResp := newPolicyDataSource(t, func(w http.ResponseWriter, r *http.Request) {})

	for _, name := range []string{"metalake", "name", "comment", "policy_type", "enabled", "supported_object_types", "properties", "custom_rules", "audit"} {
		if _, ok := schemaResp.Schema.Attributes[name]; !ok {
			t.Fatalf("attribute %q is missing", name)
		}
	}
	for _, name := range []string{"metalake", "name"} {
		if !schemaResp.Schema.Attributes[name].IsRequired() {
			t.Errorf("attribute %q must be required", name)
		}
	}
}

func TestPolicyDataSource_Configure(t *testing.T) {
	d := ds.NewDataSource()
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

// TestPolicyDataSource_Read decodes the PolicyResponse example of policies.yaml,
// including the numeric customRules value, which the string-typed schema renders
// as its JSON encoding.
func TestPolicyDataSource_Read(t *testing.T) {
	var requestPath string
	d, schemaResp := newPolicyDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, policyResponse)
	})

	resp := readPolicyGet(t, d, schemaResp, policyGetConfig())
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if want := "/api/metalakes/my_test_metalake/policies/my_policy1"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}

	ctx := context.Background()
	var state ds.PolicyDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if got := state.Comment.ValueString(); got != "This is a test policy" {
		t.Errorf("comment = %q", got)
	}
	if got := state.PolicyType.ValueString(); got != "custom" {
		t.Errorf("policy_type = %q", got)
	}
	if state.Enabled.ValueBool() {
		t.Error("expected enabled=false from the spec example")
	}
	if got := state.CustomRules.Elements()["rule1"]; !got.Equal(types.StringValue("123")) {
		t.Errorf("custom_rules[rule1] = %v, want \"123\"", got)
	}
	if got := state.Properties.Elements()["key1"]; !got.Equal(types.StringValue("value1")) {
		t.Errorf("properties[key1] = %v", got)
	}
	if len(state.SupportedObjectTypes.Elements()) != 6 {
		t.Errorf("supported_object_types = %v", state.SupportedObjectTypes.Elements())
	}
	if state.Audit.IsNull() {
		t.Error("audit must be set from the response")
	}
}

// TestPolicyDataSource_ReadWithoutContent covers a policy whose content is
// absent: the content-derived attributes must still be set.
func TestPolicyDataSource_ReadWithoutContent(t *testing.T) {
	d, schemaResp := newPolicyDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{"code":0,"policy":{"name":"my_policy1","comment":"","policyType":"custom","enabled":true}}`)
	})

	resp := readPolicyGet(t, d, schemaResp, policyGetConfig())
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	ctx := context.Background()
	var state ds.PolicyDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if !state.Enabled.ValueBool() {
		t.Error("enabled = false, want true")
	}
	if !state.Properties.IsNull() {
		t.Errorf("properties = %v, want null", state.Properties)
	}
	if !state.CustomRules.IsNull() {
		t.Errorf("custom_rules = %v, want null", state.CustomRules)
	}
}

func TestPolicyDataSource_ReadNotFound(t *testing.T) {
	d, schemaResp := newPolicyDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"code":1003,"type":"NoSuchPolicyException","message":"Policy does not exist","stack":["..."]}`)
	})

	resp := readPolicyGet(t, d, schemaResp, policyGetConfig())
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a missing policy")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Policy not found" {
		t.Fatalf("unexpected summary: %s", got)
	}
}

func TestPolicyDataSource_ReadServerError(t *testing.T) {
	d, schemaResp := newPolicyDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalServerError","message":"boom","stack":["..."]}`)
	})

	resp := readPolicyGet(t, d, schemaResp, policyGetConfig())
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed reading policy "my_policy1"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}
