package role_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/role"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// RoleResponse example of roles.yaml.
const specRoleResponse = `{
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
      }`

// specNoSuchRoleExceptionBody is the NoSuchRoleException example of roles.yaml, the
// body a real Gravitino server returns for a role that does not exist.
const specNoSuchRoleExceptionBody = `{
        "code": 1003,
        "type": "NoSuchRoleException",
        "message": "Role does not exist",
        "stack": [
          "org.apache.gravitino.exceptions.NoSuchRoleException: Role does not exist",
          "..."
        ]
      }`

func TestRoleDataSource_Schema(t *testing.T) {
	d := ds.NewRoleDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_role" {
		t.Fatalf("expected gravitino_role, got %s", resp.TypeName)
	}
}

func readRoleDataSource(t *testing.T, handler http.Handler, name string) (ds.RoleDataSourceModel, *datasource.ReadResponse) {
	t.Helper()

	server := httptest.NewServer(handler)
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	d := ds.NewRoleDataSource()
	d.(*ds.RoleDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	soObjType := types.ObjectType{AttrTypes: ds.RoleSecurableObjectAttrTypes}
	attrTypes := map[string]attr.Type{
		"metalake":          types.StringType,
		"name":              types.StringType,
		"properties":        types.MapType{ElemType: types.StringType},
		"securable_objects": types.SetType{ElemType: soObjType},
		"audit":             types.ObjectType{AttrTypes: ds.RoleAuditAttrTypes},
	}

	configModel := ds.RoleDataSourceModel{
		Metalake:         types.StringValue("probe_ml"),
		Name:             types.StringValue(name),
		Properties:       types.MapNull(types.StringType),
		SecurableObjects: types.SetNull(soObjType),
		Audit:            types.ObjectNull(ds.RoleAuditAttrTypes),
	}

	configObj, diags := types.ObjectValueFrom(ctx, attrTypes, configModel)
	if diags.HasError() {
		t.Fatalf("failed to create config object: %v", diags)
	}
	raw, err := configObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}

	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: raw}}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}

	d.Read(ctx, req, resp)

	var state ds.RoleDataSourceModel
	if !resp.Diagnostics.HasError() {
		if d := resp.State.Get(ctx, &state); d.HasError() {
			t.Fatalf("failed to read state: %v", d)
		}
	}
	return state, resp
}

func TestRoleDataSource_Read(t *testing.T) {
	var method, requestPath string

	state, resp := readRoleDataSource(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(specRoleResponse))
	}), "role1")

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	if method != http.MethodGet || requestPath != "/api/metalakes/probe_ml/roles/role1" {
		t.Fatalf("expected GET /api/metalakes/probe_ml/roles/role1, got %s %s", method, requestPath)
	}

	properties := map[string]string{}
	for k, v := range state.Properties.Elements() {
		properties[k] = v.(types.String).ValueString()
	}
	if properties["k1"] != "v1" {
		t.Fatalf("expected property k1=v1, got %v", properties)
	}

	elements := state.SecurableObjects.Elements()
	if len(elements) != 1 {
		t.Fatalf("expected 1 securable object, got %d", len(elements))
	}

	obj := elements[0].(types.Object)
	attrs := obj.Attributes()
	if attrs["full_name"].(types.String).ValueString() != "catalog1.schema1.table1" {
		t.Fatalf("unexpected full_name: %v", attrs["full_name"])
	}
	if attrs["type"].(types.String).ValueString() != "TABLE" {
		t.Fatalf("unexpected type: %v", attrs["type"])
	}

	privs := attrs["privileges"].(types.Set).Elements()
	if len(privs) != 1 {
		t.Fatalf("expected 1 privilege, got %d", len(privs))
	}
	privAttrs := privs[0].(types.Object).Attributes()
	if privAttrs["name"].(types.String).ValueString() != "SELECT_TABLE" {
		t.Fatalf("unexpected privilege name: %v", privAttrs["name"])
	}
	if privAttrs["condition"].(types.String).ValueString() != "ALLOW" {
		t.Fatalf("unexpected privilege condition: %v", privAttrs["condition"])
	}
}

// TestRoleDataSource_Read_WithAudit covers the audit block that the v1.3.0 server
// always returns (RoleDTO.audit) although roles.yaml does not list it.
func TestRoleDataSource_Read_WithAudit(t *testing.T) {
	state, resp := readRoleDataSource(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"role": {
				"name": "role1",
				"properties": {"k1": "v1"},
				"securableObjects": [],
				"audit": {
					"creator": "admin",
					"createTime": "2026-01-02T03:04:05.000Z",
					"lastModifier": "admin",
					"lastModifiedTime": "2026-01-02T03:04:05.000Z"
				}
			}
		}`))
	}), "role1")

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	if state.Audit.IsNull() {
		t.Fatal("expected audit to be populated")
	}
	audit := state.Audit.Attributes()
	if audit["creator"].(types.String).ValueString() != "admin" {
		t.Fatalf("unexpected audit creator: %v", audit["creator"])
	}

	// An empty array must not turn into a null set.
	if state.SecurableObjects.IsNull() {
		t.Fatal("expected an empty set for an empty securableObjects array")
	}
	if len(state.SecurableObjects.Elements()) != 0 {
		t.Fatalf("expected no securable objects, got %d", len(state.SecurableObjects.Elements()))
	}
}

func TestRoleDataSource_Read_NotFound(t *testing.T) {
	_, resp := readRoleDataSource(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(specNoSuchRoleExceptionBody))
	}), "role1")

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a missing role")
	}
}
