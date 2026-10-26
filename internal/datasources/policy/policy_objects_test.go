package policy_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/policy"
	"github.com/gravitino/terraform-provider-gravitino/internal/resources/resourcetest"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// components/examples/MetadataObjectListResponse of policies.yaml.
const specPolicyMetadataObjectListResponse = `{
  "code": 0,
  "metadataObjects": [
    {"type": "CATALOG", "fullName": "my_catalog"},
    {"type": "SCHEMA", "fullName": "my_catalog.my_schema"},
    {"type": "TABLE", "fullName": "my_catalog.my_schema.my_table"}
  ]
}`

func TestPolicyObjectsDataSource_Metadata(t *testing.T) {
	d := ds.NewPolicyObjectsDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_policy_objects" {
		t.Fatalf("type name = %q, want gravitino_policy_objects", resp.TypeName)
	}
}

func TestPolicyObjectsDataSource_Schema(t *testing.T) {
	s := policyDataSourceSchema(t, ds.NewPolicyObjectsDataSource())
	for _, name := range []string{"metalake", "policy", "objects"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

func TestPolicyObjectsDataSource_Read(t *testing.T) {
	const path = "/api/metalakes/my_test_metalake/policies/my_policy1/objects"

	d := newPolicyObjectDataSource(t, ds.NewPolicyObjectsDataSource(), func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == path {
			fmt.Fprint(w, specPolicyMetadataObjectListResponse)
			return
		}
		http.NotFound(w, req)
	})

	s := policyDataSourceSchema(t, d)
	ctx := context.Background()
	config := ds.PolicyObjectsDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Policy:   types.StringValue("my_policy1"),
		Objects:  types.ListNull(types.ObjectType{AttrTypes: ds.PolicyObjectItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: resourcetest.TFValue(t, ctx, s, config)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got ds.PolicyObjectsDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	elements := got.Objects.Elements()
	if len(elements) != 3 {
		t.Fatalf("objects has %d entries, want 3", len(elements))
	}
	first := elements[0].(types.Object).Attributes()
	if typeValue := first["type"].(types.String).ValueString(); typeValue != "CATALOG" {
		t.Errorf("type = %q, want CATALOG", typeValue)
	}
	if fullName := first["full_name"].(types.String).ValueString(); fullName != "my_catalog" {
		t.Errorf("full_name = %q, want my_catalog", fullName)
	}
}

func TestPolicyObjectsDataSource_Read_NotFound(t *testing.T) {
	d := newPolicyObjectDataSource(t, ds.NewPolicyObjectsDataSource(), func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, specNoSuchPolicyExceptionDS)
	})

	s := policyDataSourceSchema(t, d)
	ctx := context.Background()
	config := ds.PolicyObjectsDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Policy:   types.StringValue("my_policy1"),
		Objects:  types.ListNull(types.ObjectType{AttrTypes: ds.PolicyObjectItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: resourcetest.TFValue(t, ctx, s, config)}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a not-found diagnostic")
	}
}

func TestPolicyObjectsDataSource_SetClient(t *testing.T) {
	c, err := client.New("http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	ds.NewPolicyObjectsDataSource().(*ds.PolicyObjectsDataSource).SetClient(c)
}

func TestPolicyObjectsDataSource_Read_ServerError(t *testing.T) {
	d := newPolicyObjectDataSource(t, ds.NewPolicyObjectsDataSource(), func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalServerError","message":"boom","stack":["..."]}`)
	})

	s := policyDataSourceSchema(t, d)
	ctx := context.Background()
	config := ds.PolicyObjectsDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Policy:   types.StringValue("my_policy1"),
		Objects:  types.ListNull(types.ObjectType{AttrTypes: ds.PolicyObjectItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: resourcetest.TFValue(t, ctx, s, config)}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a server-error diagnostic")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Failed listing objects of") {
		t.Errorf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}
