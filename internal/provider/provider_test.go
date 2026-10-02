package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestBuildAuthProvider(t *testing.T) {
	type args struct {
		authMethod             string
		username               string
		password               string
		oauthToken             string
		oauthClientID          string
		oauthClientSecret      string
		oauthServerURI         string
		oauthTokenPath         string
		oauthScope             string
		kerberosPrincipal      string
		kerberosKeytab         string
		kerberosUseTicketCache bool
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		wantNil bool
	}{
		{
			name:    "empty auth defaults to nil",
			args:    args{authMethod: ""},
			wantErr: false,
			wantNil: true,
		},
		{
			name:    "none auth defaults to nil",
			args:    args{authMethod: "none"},
			wantErr: false,
			wantNil: true,
		},
		{
			name:    "simple",
			args:    args{authMethod: "simple"},
			wantErr: false,
			wantNil: false,
		},
		{
			name:    "basic with username",
			args:    args{authMethod: "basic", username: "testuser"},
			wantErr: false,
			wantNil: false,
		},
		{
			name:    "basic without username",
			args:    args{authMethod: "basic"},
			wantErr: true,
			wantNil: true,
		},
		{
			name:    "unknown method",
			args:    args{authMethod: "invalid"},
			wantErr: true,
			wantNil: true,
		},
		{
			name:    "kerberos without principal",
			args:    args{authMethod: "kerberos"},
			wantErr: true,
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.args
			ap, err := buildAuthProvider(a.authMethod, a.username, a.password,
				a.oauthToken, a.oauthClientID, a.oauthClientSecret,
				a.oauthServerURI, a.oauthTokenPath, a.oauthScope,
				a.kerberosPrincipal, a.kerberosKeytab, a.kerberosUseTicketCache)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil && ap != nil {
				t.Fatal("expected nil provider")
			}
			if !tt.wantNil && ap == nil && !tt.wantErr {
				t.Fatal("expected non-nil provider")
			}
		})
	}
}

// providerConfigValue renders the provider configuration into the raw value the
// framework hands to Configure.
func providerConfigValue(t *testing.T, ctx context.Context, s schema.Schema, model GravitinoProviderModel) tftypes.Value {
	t.Helper()
	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("failed to build config object: %v", diags)
	}
	v, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert config to terraform value: %v", err)
	}
	return v
}

func configureProvider(t *testing.T, ctx context.Context, p *GravitinoProvider, raw tftypes.Value) *provider.ConfigureResponse {
	t.Helper()
	schemaResp := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}

	resp := &provider.ConfigureResponse{}
	p.Configure(ctx, provider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	return resp
}

// TestGravitinoProvider_ConfigureDetectsServerVersion pins that Configure probes
// GET /api/version and records the result on the client, which the
// version-gated resources rely on.
func TestGravitinoProvider_ConfigureDetectsServerVersion(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.1","compileDate":"2026-01-01","gitCommit":"abc"}}`)
	}))
	defer server.Close()

	p := &GravitinoProvider{version: "test"}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
	raw := providerConfigValue(t, ctx, schemaResp.Schema, GravitinoProviderModel{URI: types.StringValue(server.URL)})
	resp := configureProvider(t, ctx, p, raw)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	c, ok := resp.ResourceData.(*client.Client)
	if !ok {
		t.Fatalf("ResourceData = %T, want *client.Client", resp.ResourceData)
	}
	if got := c.ServerVersion(); got != "1.3.1" {
		t.Fatalf("ServerVersion() = %q, want 1.3.1", got)
	}
	if !c.AtLeast(1, 3, 1) {
		t.Fatal("AtLeast(1,3,1) = false for a 1.3.1 server")
	}
}

// TestGravitinoProvider_ConfigureWithoutVersionEndpoint pins the best-effort
// behaviour: an unreachable version endpoint must not fail Configure, it only
// leaves the version unknown so nothing is rejected client-side.
func TestGravitinoProvider_ConfigureWithoutVersionEndpoint(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()

	p := &GravitinoProvider{version: "test"}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
	raw := providerConfigValue(t, ctx, schemaResp.Schema, GravitinoProviderModel{URI: types.StringValue(server.URL)})
	resp := configureProvider(t, ctx, p, raw)
	if resp.Diagnostics.HasError() {
		t.Fatalf("version detection must be best-effort, got: %v", resp.Diagnostics)
	}

	c, ok := resp.ResourceData.(*client.Client)
	if !ok {
		t.Fatalf("ResourceData = %T, want *client.Client", resp.ResourceData)
	}
	if got := c.ServerVersion(); got != "" {
		t.Fatalf("ServerVersion() = %q, want unknown", got)
	}
}
