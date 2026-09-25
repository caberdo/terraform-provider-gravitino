package authentication_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func principalTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// TestAccPrincipalDataSource_SpecExample runs a real provider/plugin protocol
// round trip against a mock that serves the spec's AuthMeResponse example. It
// applies the shipped example
// (examples/data-sources/gravitino_principal/data-source.tf) verbatim, proving
// the example is valid HCL for the current schema, and fails hard if `name` is
// left unknown after apply.
func TestAccPrincipalDataSource_SpecExample(t *testing.T) {
	config, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "data-sources", "gravitino_principal", "data-source.tf"))
	if err != nil {
		t.Fatalf("failed to read the example: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.URL.Path != "/api/authn/me" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(authMeResponseExample))
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: principalTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: string(config),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_principal.current", "name", "admin"),
					resource.TestCheckNoResourceAttr("data.gravitino_principal.current", "roles"),
					resource.TestCheckNoResourceAttr("data.gravitino_principal.current", "roles.#"),
				),
			},
		},
	})
}
