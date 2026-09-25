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

func TestRolesDataSource_Schema(t *testing.T) {
	d := ds.New()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_roles" {
		t.Fatalf("expected gravitino_roles, got %s", resp.TypeName)
	}
}

// TestRolesDataSource_Read proves the data source decodes the NameListResponse that
// GET /metalakes/{metalake}/objects/{metadataObjectType}/{metadataObjectFullName}/roles
// actually returns (roles.yaml), instead of the role objects of a non-existing
// `roles` list.
func TestRolesDataSource_Read(t *testing.T) {
	var method, requestPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		requestPath = r.URL.Path

		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		// NameListResponse example of roles.yaml.
		_, _ = w.Write([]byte(`{"code": 0, "names": [ "user1", "user2" ]}`))
	}))
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	d := ds.New()
	d.(*ds.RolesDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	attrTypes := map[string]attr.Type{
		"metalake":      types.StringType,
		"resource_type": types.StringType,
		"resource":      types.StringType,
		"names":         types.ListType{ElemType: types.StringType},
	}

	configModel := ds.RolesDataSourceModel{
		Metalake: types.StringValue("probe_ml"),
		// The `metadataObjectType` path parameter is the MetadataObject.Type enum:
		// upper-case singular (openapi.yaml), e.g. CATALOG, SCHEMA, TABLE.
		ResourceType: types.StringValue("CATALOG"),
		Resource:     types.StringValue("probe_cat"),
		Names:        types.ListNull(types.StringType),
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

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	if method != http.MethodGet || requestPath != "/api/metalakes/probe_ml/objects/CATALOG/probe_cat/roles" {
		t.Fatalf("expected GET /api/metalakes/probe_ml/objects/CATALOG/probe_cat/roles, got %s %s", method, requestPath)
	}

	var state ds.RolesDataSourceModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("failed to read state: %v", d)
	}

	if len(state.Names.Elements()) != 2 {
		t.Fatalf("expected 2 role names, got %d", len(state.Names.Elements()))
	}
	if state.Names.Elements()[0].(types.String).ValueString() != "user1" ||
		state.Names.Elements()[1].(types.String).ValueString() != "user2" {
		t.Fatalf("unexpected role names: %v", state.Names.Elements())
	}
}
