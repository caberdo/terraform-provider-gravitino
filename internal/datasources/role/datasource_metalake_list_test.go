package role_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/role"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models/modeltest"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMetalakeRolesDataSource_Metadata(t *testing.T) {
	d := ds.NewMetalakeRolesDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_metalake_roles" {
		t.Fatalf("expected gravitino_metalake_roles, got %s", resp.TypeName)
	}
}

func TestMetalakeRolesDataSource_Schema(t *testing.T) {
	d := ds.NewMetalakeRolesDataSource()
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
	if resp.Schema.DeprecationMessage != "" {
		t.Errorf("a new data source must not be deprecated: %q", resp.Schema.DeprecationMessage)
	}
}

func TestMetalakeRolesDataSource_Configure(t *testing.T) {
	d := ds.NewMetalakeRolesDataSource()
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

// TestRolesListDataSource_Deprecated asserts the message that points users at the
// replacement data source.
func TestRolesListDataSource_Deprecated(t *testing.T) {
	d := ds.NewRolesListDataSource()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if !strings.Contains(resp.Schema.DeprecationMessage, "gravitino_metalake_roles") {
		t.Fatalf("expected the deprecation message to name gravitino_metalake_roles, got %q", resp.Schema.DeprecationMessage)
	}
}

func readMetalakeRoles(t *testing.T, handler http.HandlerFunc) (*datasource.ReadResponse, ds.MetalakeRolesDataSourceModel) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	d := ds.NewMetalakeRolesDataSource()
	d.(*ds.MetalakeRolesDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)

	config := ds.MetalakeRolesDataSourceModel{
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

	var state ds.MetalakeRolesDataSourceModel
	if !resp.Diagnostics.HasError() {
		if diags := resp.State.Get(ctx, &state); diags.HasError() {
			t.Fatalf("failed to read state: %v", diags)
		}
	}
	return resp, state
}

// TestMetalakeRolesDataSource_Read decodes the NameListResponse example of
// roles.yaml returned by GET /metalakes/{metalake}/roles.
func TestMetalakeRolesDataSource_Read(t *testing.T) {
	var method, requestPath string
	resp, state := readMetalakeRoles(t, func(w http.ResponseWriter, r *http.Request) {
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
	if got := namesOfList(t, state.Names); len(got) != 2 || got[0] != "role1" || got[1] != "role2" {
		t.Fatalf("expected [role1 role2], got %v", got)
	}
	if state.Metalake.ValueString() != "probe_ml" {
		t.Fatalf("expected metalake probe_ml, got %q", state.Metalake.ValueString())
	}
}

// TestMetalakeRolesDataSource_ReadEmpty asserts an empty name list is not reported
// as a null list.
func TestMetalakeRolesDataSource_ReadEmpty(t *testing.T) {
	resp, state := readMetalakeRoles(t, func(w http.ResponseWriter, r *http.Request) {
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

func TestMetalakeRolesDataSource_ReadError(t *testing.T) {
	resp, _ := readMetalakeRoles(t, func(w http.ResponseWriter, r *http.Request) {
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

// TestMetalakeRolesDataSources_Agree asserts the deprecated and the replacement
// data source return the same name set for the same metalake through the same
// handler.
func TestMetalakeRolesDataSources_Agree(t *testing.T) {
	resp, metalakeState := readMetalakeRoles(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code": 0, "names": ["role1", "role2"]}`))
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	listResp, listState := readRolesList(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code": 0, "names": ["role1", "role2"]}`))
	})
	if listResp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", listResp.Diagnostics)
	}

	got, want := namesOfList(t, metalakeState.Names), namesOfList(t, listState.Names)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("gravitino_metalake_roles returned %v, gravitino_roles_list %v", got, want)
	}
}

func namesOfList(t *testing.T, list types.List) []string {
	t.Helper()

	names := make([]string, 0, len(list.Elements()))
	for _, element := range list.Elements() {
		value, ok := element.(types.String)
		if !ok {
			t.Fatalf("expected a string element, got %T", element)
		}
		names = append(names, value.ValueString())
	}
	sort.Strings(names)
	return names
}

// TestModelsMatchSchemas asserts every model in the package covers exactly the
// attributes of its data source schema.
func TestModelsMatchSchemas(t *testing.T) {
	cases := []struct {
		name  string
		ds    func() datasource.DataSource
		model any
	}{
		{"gravitino_roles", ds.New, ds.RolesDataSourceModel{}},
		{"gravitino_role", ds.NewRoleDataSource, ds.RoleDataSourceModel{}},
		{"gravitino_roles_list", ds.NewRolesListDataSource, ds.RolesListDataSourceModel{}},
		{"gravitino_metalake_roles", ds.NewMetalakeRolesDataSource, ds.MetalakeRolesDataSourceModel{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &datasource.SchemaResponse{}
			tc.ds().Schema(context.Background(), datasource.SchemaRequest{}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
			}
			modeltest.AssertSchemaMatches(t, tc.model, resp.Schema.Attributes, resp.Schema.Blocks)
		})
	}
}
