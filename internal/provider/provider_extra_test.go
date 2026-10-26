package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// unsetProviderEnv clears every GRAVITINO_* variable the provider reads, so a
// test cannot pick up the developer's environment.
func unsetProviderEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"GRAVITINO_URI", "GRAVITINO_AUTH", "GRAVITINO_TIMEOUT", "GRAVITINO_USERNAME", "GRAVITINO_PASSWORD",
		"GRAVITINO_OAUTH_TOKEN", "GRAVITINO_OAUTH_CLIENT_ID", "GRAVITINO_OAUTH_CLIENT_SECRET",
		"GRAVITINO_OAUTH_SERVER_URI", "GRAVITINO_OAUTH_TOKEN_PATH", "GRAVITINO_OAUTH_SCOPE",
		"GRAVITINO_KERBEROS_PRINCIPAL", "GRAVITINO_KERBEROS_KEYTAB",
		"GRAVITINO_KERBEROS_USE_TICKET_CACHE",
	} {
		t.Setenv(name, "")
	}
}

func providerSchema(t *testing.T, p provider.Provider) providerschema.Schema {
	t.Helper()
	resp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func providerConfig(t *testing.T, p provider.Provider, model GravitinoProviderModel) tfsdk.Config {
	t.Helper()
	s := providerSchema(t, p)
	obj, diags := types.ObjectValueFrom(context.Background(), s.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("failed to build provider config: %v", diags)
	}
	raw, err := obj.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("failed to convert provider config: %v", err)
	}
	return tfsdk.Config{Schema: s, Raw: raw}
}

// emptyProviderModel is the provider block with every argument unset.
func emptyProviderModel() GravitinoProviderModel {
	return GravitinoProviderModel{
		URI:                    types.StringNull(),
		Auth:                   types.StringNull(),
		Timeout:                types.StringNull(),
		Username:               types.StringNull(),
		Password:               types.StringNull(),
		OAuthToken:             types.StringNull(),
		OAuthClientID:          types.StringNull(),
		OAuthClientSecret:      types.StringNull(),
		OAuthServerURI:         types.StringNull(),
		OAuthTokenPath:         types.StringNull(),
		OAuthScope:             types.StringNull(),
		KerberosPrincipal:      types.StringNull(),
		KerberosKeytab:         types.StringNull(),
		KerberosUseTicketCache: types.BoolNull(),
	}
}

func newVersionServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.URL.Path == "/api/version" {
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.0","compileDate":"","gitCommit":""}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestProvider_NewAndMetadata(t *testing.T) {
	p := New("1.2.3")()

	resp := &provider.MetadataResponse{}
	p.Metadata(context.Background(), provider.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino" {
		t.Errorf("type name = %q", resp.TypeName)
	}
	if resp.Version != "1.2.3" {
		t.Errorf("version = %q", resp.Version)
	}
}

func TestProvider_Schema(t *testing.T) {
	s := providerSchema(t, New("test")())

	for _, name := range []string{
		"uri", "auth", "timeout", "username", "password", "oauth_token", "oauth_client_id",
		"oauth_client_secret", "oauth_server_uri", "oauth_token_path", "oauth_scope",
		"kerberos_principal", "kerberos_keytab", "kerberos_use_ticket_cache",
	} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("attribute %q is missing", name)
		}
	}
	for _, name := range []string{"password", "oauth_token", "oauth_client_secret", "kerberos_keytab"} {
		if !s.Attributes[name].IsSensitive() {
			t.Errorf("attribute %q must be sensitive", name)
		}
	}
}

func TestProvider_ConfigureNeedsURI(t *testing.T) {
	unsetProviderEnv(t)
	p := New("test")()

	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: providerConfig(t, p, emptyProviderModel())}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic when neither the uri attribute nor GRAVITINO_URI is set")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Missing Gravitino URI") {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func TestProvider_ConfigureFromAttribute(t *testing.T) {
	unsetProviderEnv(t)
	server := newVersionServer(t)

	p := New("test")()
	model := emptyProviderModel()
	model.URI = types.StringValue(server.URL)

	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: providerConfig(t, p, model)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	c, ok := resp.DataSourceData.(*client.Client)
	if !ok {
		t.Fatalf("DataSourceData is %T, want *client.Client", resp.DataSourceData)
	}
	if resp.ResourceData != c {
		t.Error("DataSourceData and ResourceData must be the same client")
	}
	if got := c.ServerVersion(); got != "1.3.0" {
		t.Errorf("detected version = %q", got)
	}
}

// TestProvider_ConfigureFromEnvironment covers the fallback to GRAVITINO_URI.
func TestProvider_ConfigureFromEnvironment(t *testing.T) {
	unsetProviderEnv(t)
	server := newVersionServer(t)
	t.Setenv("GRAVITINO_URI", server.URL)

	p := New("test")()
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: providerConfig(t, p, emptyProviderModel())}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.DataSourceData.(*client.Client); !ok {
		t.Fatalf("DataSourceData is %T, want *client.Client", resp.DataSourceData)
	}
}

func TestProvider_ConfigureRejectsInvalidKerberosCacheValue(t *testing.T) {
	unsetProviderEnv(t)
	server := newVersionServer(t)
	t.Setenv("GRAVITINO_KERBEROS_USE_TICKET_CACHE", "notabool")

	p := New("test")()
	model := emptyProviderModel()
	model.URI = types.StringValue(server.URL)

	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: providerConfig(t, p, model)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for an unparsable GRAVITINO_KERBEROS_USE_TICKET_CACHE")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Invalid GRAVITINO_KERBEROS_USE_TICKET_CACHE value") {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func TestProvider_ConfigureRejectsInvalidAuth(t *testing.T) {
	unsetProviderEnv(t)
	server := newVersionServer(t)

	p := New("test")()
	model := emptyProviderModel()
	model.URI = types.StringValue(server.URL)
	model.Auth = types.StringValue("basic")

	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: providerConfig(t, p, model)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for basic authentication without a username")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Invalid authentication configuration") {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func TestProvider_ConfigureRejectsInvalidURI(t *testing.T) {
	unsetProviderEnv(t)

	p := New("test")()
	model := emptyProviderModel()
	model.URI = types.StringValue("://not-a-url")

	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: providerConfig(t, p, model)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for an invalid uri")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Failed to create client") {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

// TestProvider_ConfigureSurvivesA refusedVersionProbe asserts a failing version
// probe is not fatal: it only logs, leaving the version unknown.
func TestProvider_ConfigureSurvivesAVersionProbeFailure(t *testing.T) {
	unsetProviderEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	p := New("test")()
	model := emptyProviderModel()
	model.URI = types.StringValue(server.URL)

	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: providerConfig(t, p, model)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a failed version probe must not be fatal: %v", resp.Diagnostics)
	}
	c := resp.DataSourceData.(*client.Client)
	if got := c.ServerVersion(); got != "" {
		t.Errorf("version = %q, want unknown", got)
	}
}

func TestReadConfigString(t *testing.T) {
	t.Setenv("GRAVITINO_TEST_VALUE", "from-env")

	if got := readConfigString(types.StringValue("from-config"), "GRAVITINO_TEST_VALUE"); got != "from-config" {
		t.Errorf("readConfigString = %q, want the configured value", got)
	}
	if got := readConfigString(types.StringNull(), "GRAVITINO_TEST_VALUE"); got != "from-env" {
		t.Errorf("readConfigString = %q, want the environment value", got)
	}
	if got := readConfigString(types.StringNull(), "GRAVITINO_TEST_UNSET"); got != "" {
		t.Errorf("readConfigString = %q, want an empty string when neither is set", got)
	}
}

func TestEnvBool(t *testing.T) {
	for value, want := range map[string]bool{"1": true, "true": true, "0": false, "notabool": false, "": false} {
		t.Setenv("GRAVITINO_TEST_BOOL", value)
		if got := envBool("GRAVITINO_TEST_BOOL"); got != want {
			t.Errorf("envBool(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestNewClientFromEnv(t *testing.T) {
	unsetProviderEnv(t)
	t.Setenv("GRAVITINO_AUTH", "basic")
	t.Setenv("GRAVITINO_USERNAME", "tester")

	c, err := NewClientFromEnv("http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("NewClientFromEnv() error = %v", err)
	}
	if c == nil {
		t.Fatal("NewClientFromEnv() returned a nil client")
	}

	t.Setenv("GRAVITINO_AUTH", "kerberos")
	t.Setenv("GRAVITINO_KERBEROS_PRINCIPAL", "")
	if _, err := NewClientFromEnv("http://127.0.0.1:1"); err == nil {
		t.Fatal("NewClientFromEnv() must report an invalid authentication configuration")
	}
}

func TestBuildAuthProvider_OAuth(t *testing.T) {
	if ap, err := buildAuthProvider("oauth", "", "", "token", "", "", "", "", "", "", "", false); err != nil || ap == nil {
		t.Fatalf("static oauth: ap=%v err=%v", ap, err)
	}
	if ap, err := buildAuthProvider("oauth", "", "", "", "id", "secret", "http://server", "/token", "scope", "", "", false); err != nil || ap == nil {
		t.Fatalf("client credentials oauth: ap=%v err=%v", ap, err)
	}
	if ap, err := buildAuthProvider("oauth", "", "", "", "id", "", "", "", "", "", "", false); err == nil || ap != nil {
		t.Fatalf("incomplete oauth must fail, got ap=%v err=%v", ap, err)
	}
}

func TestProvider_Registration(t *testing.T) {
	p := New("test")()
	ctx := context.Background()

	dsNames := map[string]bool{}
	dsConstructors := p.DataSources(ctx)
	if len(dsConstructors) == 0 {
		t.Fatal("the provider registers no data sources")
	}
	for _, constructor := range dsConstructors {
		d := constructor()
		if d == nil {
			t.Fatal("a data source constructor returned nil")
		}
		resp := &datasource.MetadataResponse{}
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "gravitino"}, resp)
		if resp.TypeName == "" {
			t.Fatal("a data source has an empty type name")
		}
		if dsNames[resp.TypeName] {
			t.Errorf("data source %q is registered twice", resp.TypeName)
		}
		dsNames[resp.TypeName] = true
	}

	rNames := map[string]bool{}
	rsConstructors := p.Resources(ctx)
	if len(rsConstructors) == 0 {
		t.Fatal("the provider registers no resources")
	}
	for _, constructor := range rsConstructors {
		r := constructor()
		if r == nil {
			t.Fatal("a resource constructor returned nil")
		}
		resp := &resource.MetadataResponse{}
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "gravitino"}, resp)
		if resp.TypeName == "" {
			t.Fatal("a resource has an empty type name")
		}
		if rNames[resp.TypeName] {
			t.Errorf("resource %q is registered twice", resp.TypeName)
		}
		rNames[resp.TypeName] = true
	}
}

func TestResolveTimeout(t *testing.T) {
	tests := []struct {
		name    string
		value   types.String
		env     string
		want    time.Duration
		wantErr bool
	}{
		{name: "default", value: types.StringNull(), want: client.DefaultTimeout},
		{name: "attribute", value: types.StringValue("45s"), want: 45 * time.Second},
		{name: "environment", value: types.StringNull(), env: "90s", want: 90 * time.Second},
		{name: "attribute wins over environment", value: types.StringValue("45s"), env: "90s", want: 45 * time.Second},
		{name: "invalid attribute", value: types.StringValue("soon"), wantErr: true},
		{name: "invalid environment", value: types.StringNull(), env: "soon", wantErr: true},
		{name: "zero", value: types.StringValue("0s"), wantErr: true},
		{name: "negative", value: types.StringValue("-1s"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GRAVITINO_TIMEOUT", tt.env)
			got, err := resolveTimeout(tt.value, "GRAVITINO_TIMEOUT")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveTimeout() = %s, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveTimeout() failed: %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveTimeout() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestProvider_ConfigureAppliesTimeoutAndUserAgent(t *testing.T) {
	unsetProviderEnv(t)

	var userAgent atomic.Value
	userAgent.Store("")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent.Store(r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case "/api/version":
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.0","compileDate":"","gitCommit":""}}`)
		case "/api/slow":
			time.Sleep(100 * time.Millisecond)
			fmt.Fprint(w, `{}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	p := New("9.9.9")()
	model := emptyProviderModel()
	model.URI = types.StringValue(server.URL)
	model.Timeout = types.StringValue("25ms")

	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: providerConfig(t, p, model)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure() failed: %v", resp.Diagnostics)
	}

	if got := userAgent.Load().(string); got != "terraform-provider-gravitino/9.9.9" {
		t.Errorf("User-Agent = %q, want the provider version", got)
	}

	c := resp.DataSourceData.(*client.Client)
	var result map[string]interface{}
	err := c.Get(context.Background(), "/slow", &result)
	if err == nil || !strings.Contains(err.Error(), "timed out after 25ms") {
		t.Fatalf("Get() = %v, want the configured 25ms timeout", err)
	}
}

func TestProvider_ConfigureRejectsInvalidTimeout(t *testing.T) {
	unsetProviderEnv(t)
	server := newVersionServer(t)

	p := New("test")()
	model := emptyProviderModel()
	model.URI = types.StringValue(server.URL)
	model.Timeout = types.StringValue("soon")

	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: providerConfig(t, p, model)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for an invalid timeout")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Invalid timeout") {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func TestDurationValidator(t *testing.T) {
	v := durationValidator{}
	if got := v.Description(context.Background()); !strings.Contains(got, "duration") {
		t.Errorf("Description() = %q, want it to mention a duration", got)
	}
	if got := v.MarkdownDescription(context.Background()); got != v.Description(context.Background()) {
		t.Errorf("MarkdownDescription() = %q, want the plain description", got)
	}

	tests := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{name: "null", value: types.StringNull()},
		{name: "unknown", value: types.StringUnknown()},
		{name: "valid", value: types.StringValue("45s")},
		{name: "invalid", value: types.StringValue("soon"), wantErr: true},
		{name: "zero", value: types.StringValue("0s"), wantErr: true},
		{name: "negative", value: types.StringValue("-1s"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			v.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("timeout"),
				ConfigValue: tt.value,
			}, resp)
			if resp.Diagnostics.HasError() != tt.wantErr {
				t.Errorf("ValidateString(%s) diagnostics = %v, want error = %t", tt.value, resp.Diagnostics, tt.wantErr)
			}
		})
	}
}

func TestNewClientFromEnvRejectsInvalidTimeout(t *testing.T) {
	unsetProviderEnv(t)
	t.Setenv("GRAVITINO_TIMEOUT", "soon")

	if _, err := NewClientFromEnv("http://localhost:8090"); err == nil {
		t.Fatal("NewClientFromEnv() = nil error, want a diagnostic for the invalid timeout")
	}
}
