package catalog_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func connectionTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// TestAccCatalogConnectionTestDataSource_SpecExample runs real provider protocol
// round trips (through Terraform Core) against a mock that serves the
// catalogs.yaml examples. Core fails the step when a computed attribute stays
// unknown or drifts from the planned value.
func TestAccCatalogConnectionTestDataSource_SpecExample(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")

		switch r.URL.Path {
		case "/api/version":
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.1","compileDate":"2026-01-01","gitCommit":"abc"}}`)
		case "/api/metalakes/test_metalake/catalogs/testConnection":
			// The proposed-config endpoint: the spec's ConnectionFailedException
			// example is returned for anything but the "my_hive_catalog" probe.
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("failed to decode request body: %v", err)
			}
			if body.Name == "my_hive_catalog" {
				fmt.Fprint(w, connectionTestSuccessExample)
				return
			}
			fmt.Fprint(w, connectionTestFailedExample)
		case "/api/metalakes/test_metalake/catalogs/my_hive_catalog/testConnection":
			fmt.Fprint(w, connectionTestSuccessExample)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: connectionTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_catalog_connection_test" "proposed_ok" {
  metalake = "test_metalake"
  name     = "my_hive_catalog"
  type     = "relational"
  catalog_provider = "hive"
  comment          = "This is my hive catalog"
  properties = {
    "metastore.uris" = "thrift://127.0.0.1:9083"
    "key1"           = "value1"
  }
}

data "gravitino_catalog_connection_test" "proposed_failed" {
  metalake = "test_metalake"
  name     = "my_broken_catalog"
  type     = "relational"
  catalog_provider = "hive"
  properties = {
    "metastore.uris" = "thrift://dead-host:9083"
  }
}

data "gravitino_catalog_connection_test" "existing" {
  metalake = "test_metalake"
  catalog  = "my_hive_catalog"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_catalog_connection_test.proposed_ok", "success", "true"),
					resource.TestCheckResourceAttr("data.gravitino_catalog_connection_test.proposed_ok", "message", ""),
					resource.TestCheckResourceAttr("data.gravitino_catalog_connection_test.proposed_failed", "success", "false"),
					resource.TestCheckResourceAttr("data.gravitino_catalog_connection_test.proposed_failed", "message",
						"Failed to run getAllDatabases in Hive Metastore: Failed to connect to Hive Metastore"),
					resource.TestCheckResourceAttr("data.gravitino_catalog_connection_test.existing", "success", "true"),
					resource.TestCheckResourceAttr("data.gravitino_catalog_connection_test.existing", "message", ""),
				),
			},
		},
	})
}

// TestAccCatalogConnectionTestDataSource_ConfigValidation proves the config
// validator is wired into the provider: Terraform Core rejects a configuration
// that mixes the two variants before any request is sent.
func TestAccCatalogConnectionTestDataSource_ConfigValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request expected, got %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: connectionTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_catalog_connection_test" "mixed" {
  metalake = "test_metalake"
  catalog  = "my_hive_catalog"
  name     = "my_hive_catalog"
  type     = "relational"
}
`,
				ExpectError: regexp.MustCompile(`Conflicting catalog connection test configuration`),
			},
		},
	})
}

// TestAccCatalogConnectionTestDataSource_ExistingCatalogVersionGate: on a 1.3.0
// server the existing-catalog variant must be rejected before any request is
// sent to the unsupported endpoint.
func TestAccCatalogConnectionTestDataSource_ExistingCatalogVersionGate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.URL.Path == "/api/version" {
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.0","compileDate":"2026-01-01","gitCommit":"abc"}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: connectionTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_catalog_connection_test" "existing" {
  metalake = "test_metalake"
  catalog  = "my_hive_catalog"
}
`,
				ExpectError: regexp.MustCompile(`Unsupported Gravitino version`),
			},
		},
	})
}
