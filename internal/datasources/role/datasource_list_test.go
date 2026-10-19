package role_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/role"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRolesListDataSource_Metadata(t *testing.T) {
	d := ds.NewRolesListDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_roles_list" {
		t.Fatalf("expected gravitino_roles_list, got %s", resp.TypeName)
	}
}

func TestRolesListDataSource_Schema(t *testing.T) {
	d := ds.NewRolesListDataSource()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range []string{"metalake", "names"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Fatalf("attribute %q is missing", name)
		}
	}
	if !resp.Schema.Attributes["metalake"].IsRequired() {
		t.Error("metalake must be required")
	}
	if !resp.Schema.Attributes["names"].IsComputed() {
		t.Error("names must be computed")
	}
}

func TestRolesListDataSource_Configure(t *testing.T) {
	d := ds.NewRolesListDataSource()
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
}

func readRolesList(t *testing.T, handler http.HandlerFunc) (*datasource.ReadResponse, ds.RolesListDataSourceModel) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	d := ds.NewRolesListDataSource()
	d.(*ds.RolesListDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)

	config := ds.RolesListDataSourceModel{
		Metalake: types.StringValue("probe_ml"),
		Names:    types.ListNull(types.StringType),
	}
	obj, diags := types.ObjectValueFrom(ctx, schemaResp.Schema.Type().(types.ObjectType).AttributeTypes(), config)
	if diags.HasError() {
		t.Fatalf("failed to create config object: %v", diags)
	}
	raw, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert config: %v", err)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)

	var state ds.RolesListDataSourceModel
	if !resp.Diagnostics.HasError() {
		if diags := resp.State.Get(ctx, &state); diags.HasError() {
			t.Fatalf("failed to read state: %v", diags)
		}
	}
	return resp, state
}

// TestRolesListDataSource_Read decodes the NameListResponse example of roles.yaml
// returned by GET /metalakes/{metalake}/roles.
func TestRolesListDataSource_Read(t *testing.T) {
	var method, requestPath string
	resp, state := readRolesList(t, func(w http.ResponseWriter, r *http.Request) {
		method, requestPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code": 0, "names": ["role1", "role2"]}`))
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if method != http.MethodGet || requestPath != "/api/metalakes/probe_ml/roles" {
		t.Fatalf("expected GET /api/metalakes/probe_ml/roles, got %s %s", method, requestPath)
	}
	if len(state.Names.Elements()) != 2 {
		t.Fatalf("expected 2 names, got %v", state.Names.Elements())
	}
}

// TestRolesListDataSource_ReadEmpty asserts an empty name list is not reported as
// a null list.
func TestRolesListDataSource_ReadEmpty(t *testing.T) {
	resp, state := readRolesList(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code": 0, "names": []}`))
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if state.Names.IsNull() {
		t.Fatal("an empty result must not be a null list")
	}
	if len(state.Names.Elements()) != 0 {
		t.Fatalf("expected no names, got %v", state.Names.Elements())
	}
}

func TestRolesListDataSource_ReadError(t *testing.T) {
	resp, _ := readRolesList(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":1100,"type":"InternalServerError","message":"boom","stack":["..."]}`))
	})

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed listing roles in "probe_ml"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}
