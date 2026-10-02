package credential_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func credentialTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// TestAccCredentialsDataSource_SpecExample runs a real provider/plugin protocol
// round trip against a mock that serves the spec's CredentialResponse example.
// It fails hard ("Provider produced inconsistent result after apply" / "invalid
// result object after apply") if any computed attribute is left unknown, which
// is the regression this resource family is prone to.
func TestAccCredentialsDataSource_SpecExample(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case "/api/metalakes/test_metalake/objects/CATALOG/test_catalog/credentials":
			_, _ = w.Write([]byte(credentialResponseExample))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: credentialTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_credentials" "example" {
  metalake      = "test_metalake"
  resource_type = "CATALOG"
  resource      = "test_catalog"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.#", "2"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.0.credential_type", "s3-token"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.0.expire_time_in_ms", "1735891948411"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.0.credential_info.%", "3"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.0.credential_info.s3-access-key-id", "value1"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.0.credential_info.s3-secret-access-key", "value2"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.0.credential_info.s3-session-token", "value3"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.1.credential_type", "s3-secret-key"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.1.expire_time_in_ms", "0"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.1.credential_info.%", "2"),
				),
			},
		},
	})
}

// TestAccCredentialsDataSource_EmptyResponse proves an empty credentials list
// is applied as a known empty list instead of an unknown value.
func TestAccCredentialsDataSource_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code": 0, "credentials": []}`))
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: credentialTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_credentials" "example" {
  metalake      = "test_metalake"
  resource_type = "SCHEMA"
  resource      = "test_catalog.test_schema"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.#", "0"),
				),
			},
		},
	})
}

// TestAccCredentialsDataSource_VersionGate goes through Terraform Core: the
// VIEW/FUNCTION metadata object types were added by Gravitino 1.3.1, so a VIEW
// read against a 1.3.0 server must fail with the explicit version diagnostic
// while the same read against a 1.3.1 server must apply.
func TestAccCredentialsDataSource_VersionGate(t *testing.T) {
	newServer := func(version string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
			switch {
			case r.URL.Path == "/api/version":
				_, _ = fmt.Fprintf(w, `{"code":0,"version":{"version":%q}}`, version)
			case strings.HasSuffix(r.URL.Path, "/credentials"):
				_, _ = w.Write([]byte(credentialResponseExample))
			default:
				http.NotFound(w, r)
			}
		}))
	}

	const config = `
data "gravitino_credentials" "example" {
  metalake      = "test_metalake"
  resource_type = "VIEW"
  resource      = "test_catalog.test_schema.test_view"
}
`

	oldServer := newServer("1.3.0")
	defer oldServer.Close()
	t.Setenv("GRAVITINO_URI", oldServer.URL)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: credentialTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`requires Gravitino 1\.3\.1`),
			},
		},
	})

	newerServer := newServer("1.3.1")
	defer newerServer.Close()
	t.Setenv("GRAVITINO_URI", newerServer.URL)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: credentialTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "resource_type", "VIEW"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.#", "2"),
				),
			},
		},
	})
}

// TestAccCredentialsDataSource_ExampleFile applies the shipped example
// (examples/data-sources/gravitino_credentials/data-source.tf) against the
// spec's credential payload, proving the example is valid HCL for the current
// schema.
func TestAccCredentialsDataSource_ExampleFile(t *testing.T) {
	config, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "data-sources", "gravitino_credentials", "data-source.tf"))
	if err != nil {
		t.Fatalf("failed to read the example: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if !strings.HasSuffix(r.URL.Path, "/credentials") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(credentialResponseExample))
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: credentialTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: string(config),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "metalake", "example_metalake"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "resource_type", "TABLE"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.#", "2"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.0.credential_type", "s3-token"),
					resource.TestCheckResourceAttr("data.gravitino_credentials.example", "credentials.0.credential_info.s3-session-token", "value3"),
				),
			},
		},
	})
}
