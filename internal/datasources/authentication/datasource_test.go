package authentication_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/authentication"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// authMeResponseExample is the exact `AuthMeResponse` example of the Gravitino
// v1.3.0 authn spec (docs/open-api/authn.yaml,
// components/examples/AuthMeResponse).
const authMeResponseExample = `{
  "code": 0,
  "principal": "admin"
}`

var principalAttrTypes = map[string]attr.Type{
	"name": types.StringType,
}

func newPrincipalSchema(t *testing.T, d datasource.DataSource) schema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func readPrincipal(t *testing.T, d datasource.DataSource) (*datasource.ReadResponse, ds.PrincipalDataSourceModel) {
	t.Helper()

	ctx := context.Background()
	schemaObj := newPrincipalSchema(t, d)

	configObj, diags := types.ObjectValueFrom(ctx, principalAttrTypes, ds.PrincipalDataSourceModel{
		Name: types.StringNull(),
	})
	if diags.HasError() {
		t.Fatalf("failed to create config object: %v", diags)
	}
	tfVal, err := configObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: tfVal}}, resp)

	var state ds.PrincipalDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			t.Fatalf("failed to read state: %v", resp.Diagnostics)
		}
	}
	return resp, state
}

func logPrincipalDiagnostics(t *testing.T, resp *datasource.ReadResponse) {
	t.Helper()
	for _, diag := range resp.Diagnostics.Errors() {
		t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
	}
}

func TestPrincipalDataSource_Metadata(t *testing.T) {
	d := ds.New()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_principal" {
		t.Fatalf("expected gravitino_principal, got %s", resp.TypeName)
	}
}

func TestPrincipalDataSource_Schema(t *testing.T) {
	s := newPrincipalSchema(t, ds.New())

	nameAttr, ok := s.Attributes["name"]
	if !ok {
		t.Fatal("missing attribute name")
	}
	if !nameAttr.IsComputed() {
		t.Error("name must be computed")
	}

	// /authn/me returns `{code, principal}` only, so the data source must not
	// expose a permanently empty `roles` attribute.
	if _, ok := s.Attributes["roles"]; ok {
		t.Error("roles must not exist: GET /api/authn/me never returns roles")
	}
	if len(s.Attributes) != 1 {
		t.Errorf("expected exactly one attribute, got %d", len(s.Attributes))
	}
}

func TestPrincipalDataSource_ReadSpecExample(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(authMeResponseExample))
	}))
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	d := ds.New()
	d.(*ds.PrincipalDataSource).SetClient(c)

	resp, state := readPrincipal(t, d)
	if resp.Diagnostics.HasError() {
		logPrincipalDiagnostics(t, resp)
		t.Fatal("unexpected diagnostics errors")
	}

	if requestedPath != "/api/authn/me" {
		t.Fatalf("expected path /api/authn/me, got %s", requestedPath)
	}
	if state.Name.IsNull() || state.Name.IsUnknown() {
		t.Fatalf("name must be a known string, got %v", state.Name)
	}
	if state.Name.ValueString() != "admin" {
		t.Fatalf("expected principal admin, got %q", state.Name.ValueString())
	}
}

func TestPrincipalDataSource_ReadEmptyPrincipal(t *testing.T) {
	// A 200 with no principal must still produce a known value (empty string),
	// never an unknown one.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code": 0}`))
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.New()
	d.(*ds.PrincipalDataSource).SetClient(c)

	resp, state := readPrincipal(t, d)
	if resp.Diagnostics.HasError() {
		logPrincipalDiagnostics(t, resp)
		t.Fatal("unexpected diagnostics errors")
	}
	if state.Name.IsNull() || state.Name.IsUnknown() {
		t.Fatalf("name must be known, got %v", state.Name)
	}
	if state.Name.ValueString() != "" {
		t.Fatalf("expected empty principal, got %q", state.Name.ValueString())
	}
}

func TestPrincipalDataSource_ReadUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":1001,"type":"UnauthorizedException","message":"Authentication credentials are missing or invalid","stack":["org.apache.gravitino.exceptions.UnauthorizedException"]}`))
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.New()
	d.(*ds.PrincipalDataSource).SetClient(c)

	resp, _ := readPrincipal(t, d)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for a 401 response")
	}
	logPrincipalDiagnostics(t, resp)

	var detail strings.Builder
	for _, diag := range resp.Diagnostics.Errors() {
		detail.WriteString(diag.Detail())
		detail.WriteString("\n")
	}
	if !strings.Contains(detail.String(), "401") {
		t.Errorf("diagnostics must mention the HTTP status, got:\n%s", detail.String())
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state must not be written when the API call failed")
	}
}

// TestAuthMeResponseDTOMatchesSpec guards the JSON keys of the DTO against
// the spec example.
func TestAuthMeResponseDTOMatchesSpec(t *testing.T) {
	var result models.AuthMeResponse
	if err := json.Unmarshal([]byte(authMeResponseExample), &result); err != nil {
		t.Fatalf("failed to decode the spec example: %v", err)
	}
	if result.Code != 0 {
		t.Errorf("code = %d, want 0", result.Code)
	}
	if result.Principal != "admin" {
		t.Errorf("principal = %q, want %q", result.Principal, "admin")
	}
}
