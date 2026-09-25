package idp_group

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

// idpGroupGetResponseExample is the exact IdpGroupGetResponse example of the
// Gravitino v1.3.0 IDP spec (docs/open-api/idp/idp.yaml).
const idpGroupGetResponseExample = `{
  "code": 0,
  "group": {
    "name": "engineers",
    "users": ["alice", "bob"]
  }
}`

// idpGroupNotFoundExample is the exact IdpGroupNotFoundException example of the
// same spec, operation GET.
const idpGroupNotFoundExample = `{
  "code": 1003,
  "type": "NotFoundException",
  "message": "Failed to operate built-in IdP group [missing-group] operation [GET], reason [IdP group not found: missing-group]"
}`

func idpGroupDataSourceSchema(t *testing.T) schema.Schema {
	t.Helper()

	resp := &datasource.SchemaResponse{}
	NewDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func newIdpGroupDataSourceWithServer(t *testing.T, handler http.HandlerFunc) datasource.DataSource {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	d := NewDataSource()
	d.(*IdpGroupDataSource).SetClient(c)
	return d
}

func readIdpGroup(t *testing.T, d datasource.DataSource, name string) (*datasource.ReadResponse, IdpGroupDataSourceModel) {
	t.Helper()

	ctx := context.Background()
	s := idpGroupDataSourceSchema(t)

	config, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), IdpGroupDataSourceModel{
		Name:  types.StringValue(name),
		Users: types.SetNull(types.StringType),
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

	var state IdpGroupDataSourceModel
	if !resp.Diagnostics.HasError() {
		if diags := resp.State.Get(ctx, &state); diags.HasError() {
			t.Fatalf("failed to read state: %v", diags)
		}
	}
	return resp, state
}

func logIdpGroupDataSourceDiagnostics(t *testing.T, resp *datasource.ReadResponse) {
	t.Helper()
	for _, diag := range resp.Diagnostics.Errors() {
		t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
	}
}

func TestIdpGroupDataSource_Metadata(t *testing.T) {
	d := NewDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_idp_group" {
		t.Fatalf("expected gravitino_idp_group, got %s", resp.TypeName)
	}
}

func TestIdpGroupDataSource_Schema(t *testing.T) {
	s := idpGroupDataSourceSchema(t)

	for _, name := range []string{"name", "users"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Fatalf("missing attribute %s", name)
		}
	}

	// IdpGroup has no comment field, so `comment` could never be filled.
	if _, ok := s.Attributes["comment"]; ok {
		t.Error("comment must not exist: the IDP API has no group comment")
	}
	if len(s.Attributes) != 2 {
		t.Errorf("expected exactly 2 attributes, got %d", len(s.Attributes))
	}
}

func TestIdpGroupDataSource_ReadSpecExample(t *testing.T) {
	var gotPath string

	d := newIdpGroupDataSourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(idpGroupGetResponseExample))
	})

	resp, state := readIdpGroup(t, d, "engineers")
	if resp.Diagnostics.HasError() {
		logIdpGroupDataSourceDiagnostics(t, resp)
		t.Fatal("unexpected diagnostics errors")
	}

	if gotPath != "/api/idp/groups/engineers" {
		t.Errorf("expected path /api/idp/groups/engineers, got %s", gotPath)
	}
	if state.Name.ValueString() != "engineers" {
		t.Errorf("name = %q, want engineers", state.Name.ValueString())
	}
	if state.Users.IsNull() || state.Users.IsUnknown() {
		t.Fatalf("users must be a known set, got %v", state.Users)
	}

	users := make([]string, 0, len(state.Users.Elements()))
	for _, v := range state.Users.Elements() {
		users = append(users, v.(types.String).ValueString())
	}
	if len(users) != 2 || users[0] != "alice" || users[1] != "bob" {
		t.Errorf("users = %v, want [alice bob]", users)
	}
}

func TestIdpGroupDataSource_ReadNotFound(t *testing.T) {
	d := newIdpGroupDataSourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(idpGroupNotFoundExample))
	})

	resp, _ := readIdpGroup(t, d, "missing-group")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for a 404 response")
	}
	logIdpGroupDataSourceDiagnostics(t, resp)

	var detail strings.Builder
	for _, diag := range resp.Diagnostics.Errors() {
		detail.WriteString(diag.Detail())
		detail.WriteString("\n")
	}
	for _, want := range []string{"404", "NotFoundException", "IdP group not found: missing-group"} {
		if !strings.Contains(detail.String(), want) {
			t.Errorf("diagnostics must mention %q, got:\n%s", want, detail.String())
		}
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state must not be written when the API call failed")
	}
}

func TestIdpGroupDataSource_ConfigureRejectsWrongProviderData(t *testing.T) {
	d := NewDataSource().(datasource.DataSourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	d.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for unexpected provider data")
	}
}

// TestIdpGroupDataSourceDTOMatchesSpec guards the DTO against the spec example.
func TestIdpGroupDataSourceDTOMatchesSpec(t *testing.T) {
	var result models.IdpGroupResponse
	if err := json.Unmarshal([]byte(idpGroupGetResponseExample), &result); err != nil {
		t.Fatalf("failed to decode the spec example: %v", err)
	}
	if result.Code != 0 {
		t.Errorf("code = %d, want 0", result.Code)
	}
	if result.Group.Name != "engineers" {
		t.Errorf("name = %q, want engineers", result.Group.Name)
	}
	if len(result.Group.Users) != 2 || result.Group.Users[0] != "alice" || result.Group.Users[1] != "bob" {
		t.Errorf("users = %v, want [alice bob]", result.Group.Users)
	}
}
