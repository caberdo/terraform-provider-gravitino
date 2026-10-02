package iceberg_rest_service_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func icebergTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func icebergExampleConfig(t *testing.T) string {
	t.Helper()
	config, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "data-sources", "gravitino_iceberg_rest_service", "data-source.tf"))
	if err != nil {
		t.Fatalf("failed to read the example: %v", err)
	}
	return string(config)
}

// versionedIcebergServer serves /api/version with the given version and the
// v1.3.1 iceberg-rest endpoint with the available spec example.
func versionedIcebergServer(t *testing.T, version string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case "/api/version":
			_, _ = w.Write([]byte(`{"code":0,"version":{"version":"` + version + `","compileDate":"01/01/2026","gitCommit":"abc"}}`))
		case "/api/system/iceberg-rest":
			_, _ = w.Write([]byte(icebergAvailableExample))
		default:
			http.NotFound(w, r)
		}
	}))
}

// TestAccIcebergRESTServiceDataSource_SpecExample runs a real provider/plugin
// protocol round trip against a mock that serves the spec's
// IcebergRESTServiceAvailable example. It applies the shipped example
// (examples/data-sources/gravitino_iceberg_rest_service/data-source.tf) verbatim,
// proving the example is valid HCL for the current schema.
func TestAccIcebergRESTServiceDataSource_SpecExample(t *testing.T) {
	server := versionedIcebergServer(t, "1.3.1")
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: icebergTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: icebergExampleConfig(t),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_iceberg_rest_service.current", "metalake", "my_metalake"),
					resource.TestCheckResourceAttr("data.gravitino_iceberg_rest_service.current", "uri", "http://gravitino-host:9001/iceberg"),
				),
			},
		},
	})
}

// TestAccIcebergRESTServiceDataSource_VersionGate applies the same example
// against a mock reporting Gravitino 1.3.0; the endpoint does not exist there, so
// the read must fail with the version requirement instead of an opaque HTTP 404.
func TestAccIcebergRESTServiceDataSource_VersionGate(t *testing.T) {
	server := versionedIcebergServer(t, "1.3.0")
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: icebergTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      icebergExampleConfig(t),
				ExpectError: regexp.MustCompile(`(?s)requires Gravitino >= 1\.3\.1`),
			},
		},
	})
}
