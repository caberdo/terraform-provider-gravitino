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
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// specNoSuchPolicyExceptionDS is the verbatim `NoSuchPolicyException` example of
// policies.yaml, returned by the Gravitino server with HTTP status 404.
const specNoSuchPolicyExceptionDS = `{
  "code": 1003,
  "type": "NoSuchPolicyException",
  "message": "Failed to operate policy(s) [my_policy] operation [LOAD] under metalake [my_test_metalake], reason [NoSuchPolicyException]",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchPolicyException: Policy my_policy does not exist",
    "..."
  ]
}`

func policyDataSourceSchema(t *testing.T, d datasource.DataSource) schema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func newPolicyObjectDataSource(t *testing.T, d datasource.DataSource, handler http.HandlerFunc) datasource.DataSource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	if configured, ok := d.(datasource.DataSourceWithConfigure); ok {
		resp := &datasource.ConfigureResponse{}
		configured.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: c}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("configure diagnostics: %v", resp.Diagnostics)
		}
	}
	return d
}

func TestObjectPoliciesDataSource_Metadata(t *testing.T) {
	d := ds.NewObjectPoliciesDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_object_policies" {
		t.Fatalf("type name = %q, want gravitino_object_policies", resp.TypeName)
	}
}

func TestObjectPoliciesDataSource_Schema(t *testing.T) {
	s := policyDataSourceSchema(t, ds.NewObjectPoliciesDataSource())
	for _, name := range []string{"metalake", "object_type", "object_full_name", "policies"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

func TestObjectPoliciesDataSource_Read(t *testing.T) {
	const path = "/api/metalakes/my_test_metalake/objects/TABLE/sales.orders/policies"

	var gotQuery string
	d := newPolicyObjectDataSource(t, ds.NewObjectPoliciesDataSource(), func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == path {
			gotQuery = req.URL.RawQuery
			fmt.Fprint(w, policyListResponse)
			return
		}
		http.NotFound(w, req)
	})

	s := policyDataSourceSchema(t, d)
	ctx := context.Background()
	config := ds.ObjectPoliciesDataSourceModel{
		Metalake:       types.StringValue("my_test_metalake"),
		ObjectType:     types.StringValue("TABLE"),
		ObjectFullName: types.StringValue("sales.orders"),
		Policies:       types.ListNull(types.StringType),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: resourcetest.TFValue(t, ctx, s, config)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if gotQuery != "details=true" {
		t.Errorf("query = %q, want details=true", gotQuery)
	}

	var got ds.ObjectPoliciesDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	names := policyNames(t, got)
	if len(names) != 1 || names[0] != "my_policy1" {
		t.Errorf("policies = %v, want the spec's my_policy1", names)
	}
}

func TestObjectPoliciesDataSource_Read_NotFound(t *testing.T) {
	d := newPolicyObjectDataSource(t, ds.NewObjectPoliciesDataSource(), func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, specNoSuchPolicyExceptionDS)
	})

	s := policyDataSourceSchema(t, d)
	ctx := context.Background()
	config := ds.ObjectPoliciesDataSourceModel{
		Metalake:       types.StringValue("my_test_metalake"),
		ObjectType:     types.StringValue("TABLE"),
		ObjectFullName: types.StringValue("sales.orders"),
		Policies:       types.ListNull(types.StringType),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: resourcetest.TFValue(t, ctx, s, config)}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a not-found diagnostic")
	}
}

func policyNames(t *testing.T, model ds.ObjectPoliciesDataSourceModel) []string {
	t.Helper()
	if model.Policies.IsNull() || model.Policies.IsUnknown() {
		return nil
	}
	names := make([]string, 0, len(model.Policies.Elements()))
	for _, element := range model.Policies.Elements() {
		names = append(names, element.(types.String).ValueString())
	}
	return names
}

func TestObjectPoliciesDataSource_SetClient(t *testing.T) {
	c, err := client.New("http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	ds.NewObjectPoliciesDataSource().(*ds.ObjectPoliciesDataSource).SetClient(c)
}

func TestObjectPoliciesDataSource_Read_ServerError(t *testing.T) {
	d := newPolicyObjectDataSource(t, ds.NewObjectPoliciesDataSource(), func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalServerError","message":"boom","stack":["..."]}`)
	})

	s := policyDataSourceSchema(t, d)
	ctx := context.Background()
	config := ds.ObjectPoliciesDataSourceModel{
		Metalake:       types.StringValue("my_test_metalake"),
		ObjectType:     types.StringValue("TABLE"),
		ObjectFullName: types.StringValue("sales.orders"),
		Policies:       types.ListNull(types.StringType),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: resourcetest.TFValue(t, ctx, s, config)}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a server-error diagnostic")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Failed listing policies of") {
		t.Errorf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}
