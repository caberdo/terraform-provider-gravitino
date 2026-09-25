package idp_user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// idpUserGetResponseExample is the exact IdpUserGetResponse example of the
// Gravitino v1.3.0 IDP spec (docs/open-api/idp/idp.yaml).
const idpUserGetResponseExample = `{
  "code": 0,
  "user": {
    "name": "alice",
    "groups": ["engineers"]
  }
}`

// idpUserNotFoundExample is the exact IdpNotFoundException example of the same
// spec, operation GET.
const idpUserNotFoundExample = `{
  "code": 1003,
  "type": "NotFoundException",
  "message": "Failed to operate built-in IdP user [missing-user] operation [GET], reason [IdP user not found: missing-user]"
}`

func idpUserDataSourceSchema(t *testing.T) schema.Schema {
	t.Helper()

	resp := &datasource.SchemaResponse{}
	NewDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func newIdpUserDataSourceWithServer(t *testing.T, handler http.HandlerFunc) datasource.DataSource {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	d := NewDataSource()
	d.(*IdpUserDataSource).SetClient(c)
	return d
}

func readIdpUser(t *testing.T, d datasource.DataSource, name string) (*datasource.ReadResponse, IdpUserDataSourceModel) {
	t.Helper()

	ctx := context.Background()
	s := idpUserDataSourceSchema(t)

	config, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), IdpUserDataSourceModel{
		Name:   types.StringValue(name),
		Groups: types.SetNull(types.StringType),
	})
	if diags.HasError() {
		t.Fatalf("failed to build config: %v", diags)
	}
	raw, err := config.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert config: %v", err)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: raw}}, resp)

	var state IdpUserDataSourceModel
	if !resp.Diagnostics.HasError() {
		if diags := resp.State.Get(ctx, &state); diags.HasError() {
			t.Fatalf("failed to read state: %v", diags)
		}
	}
	return resp, state
}

func logIdpUserDataSourceDiagnostics(t *testing.T, resp *datasource.ReadResponse) {
	t.Helper()
	for _, diag := range resp.Diagnostics.Errors() {
		t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
	}
}

func TestIdpUserDataSource_Metadata(t *testing.T) {
	d := NewDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_idp_user" {
		t.Fatalf("expected gravitino_idp_user, got %s", resp.TypeName)
	}
}

func TestIdpUserDataSource_Schema(t *testing.T) {
	s := idpUserDataSourceSchema(t)

	for _, name := range []string{"name", "groups"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Fatalf("missing attribute %s", name)
		}
	}

	// IdpUser has no enable/disable flag, so `enabled` could never be filled.
	if _, ok := s.Attributes["enabled"]; ok {
		t.Error("enabled must not exist: the IDP API has no enable/disable flag")
	}
	if len(s.Attributes) != 2 {
		t.Errorf("expected exactly 2 attributes, got %d", len(s.Attributes))
	}
}

func TestIdpUserDataSource_ReadSpecExample(t *testing.T) {
	var gotPath string

	d := newIdpUserDataSourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(idpUserGetResponseExample))
	})

	resp, state := readIdpUser(t, d, "alice")
	if resp.Diagnostics.HasError() {
		logIdpUserDataSourceDiagnostics(t, resp)
		t.Fatal("unexpected diagnostics errors")
	}

	if gotPath != "/api/idp/users/alice" {
		t.Errorf("expected path /api/idp/users/alice, got %s", gotPath)
	}
	if state.Name.ValueString() != "alice" {
		t.Errorf("name = %q, want alice", state.Name.ValueString())
	}
	if state.Groups.IsNull() || state.Groups.IsUnknown() {
		t.Fatalf("groups must be a known set, got %v", state.Groups)
	}
	groups := state.Groups.Elements()
	if len(groups) != 1 || groups[0].(types.String).ValueString() != "engineers" {
		t.Errorf("groups = %v, want [engineers]", groups)
	}
}

func TestIdpUserDataSource_ReadNotFound(t *testing.T) {
	d := newIdpUserDataSourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(idpUserNotFoundExample))
	})

	resp, _ := readIdpUser(t, d, "missing-user")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for a 404 response")
	}
	logIdpUserDataSourceDiagnostics(t, resp)

	var detail strings.Builder
	for _, diag := range resp.Diagnostics.Errors() {
		detail.WriteString(diag.Detail())
		detail.WriteString("\n")
	}
	for _, want := range []string{"404", "NotFoundException", "IdP user not found: missing-user"} {
		if !strings.Contains(detail.String(), want) {
			t.Errorf("diagnostics must mention %q, got:\n%s", want, detail.String())
		}
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state must not be written when the API call failed")
	}
}

func TestIdpUserDataSource_ConfigureRejectsWrongProviderData(t *testing.T) {
	d := NewDataSource().(datasource.DataSourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	d.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for unexpected provider data")
	}
}

// TestIdpUserDataSourceDTOMatchesSpec guards the DTO against the spec example.
func TestIdpUserDataSourceDTOMatchesSpec(t *testing.T) {
	var result models.IdpUserResponse
	if err := json.Unmarshal([]byte(idpUserGetResponseExample), &result); err != nil {
		t.Fatalf("failed to decode the spec example: %v", err)
	}
	if result.Code != 0 {
		t.Errorf("code = %d, want 0", result.Code)
	}
	if result.User.Name != "alice" {
		t.Errorf("name = %q, want alice", result.User.Name)
	}
	if len(result.User.Groups) != 1 || result.User.Groups[0] != "engineers" {
		t.Errorf("groups = %v, want [engineers]", result.User.Groups)
	}
}
