package metalake_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var datasourceTestAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func TestAccDataSourceMetalakes_List(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/metalakes" {
			json.NewEncoder(w).Encode(models.MetalakeListResponse{
				Code: 0,
				Metalakes: []models.Metalake{
					{
						Name:       "ml1",
						Comment:    "first",
						Properties: map[string]string{"k": "v"},
					},
					{
						Name:    "ml2",
						Comment: "second",
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: datasourceTestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "gravitino_metalakes" "test" {}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_metalakes.test", "metalakes.#", "2"),
					resource.TestCheckResourceAttr("data.gravitino_metalakes.test", "metalakes.0.name", "ml1"),
					resource.TestCheckResourceAttr("data.gravitino_metalakes.test", "metalakes.0.comment", "first"),
					resource.TestCheckResourceAttr("data.gravitino_metalakes.test", "metalakes.0.properties.k", "v"),
					resource.TestCheckResourceAttr("data.gravitino_metalakes.test", "metalakes.1.name", "ml2"),
					resource.TestCheckResourceAttr("data.gravitino_metalakes.test", "metalakes.1.comment", "second"),
				),
			},
		},
	})
}

func TestAccDataSourceMetalake_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/test_ml" {
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:       "test_ml",
					Comment:    "a metalake",
					Properties: map[string]string{"env": "dev"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: datasourceTestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_metalake" "test" {
  name = "test_ml"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_metalake.test", "name", "test_ml"),
					resource.TestCheckResourceAttr("data.gravitino_metalake.test", "comment", "a metalake"),
					resource.TestCheckResourceAttr("data.gravitino_metalake.test", "properties.env", "dev"),
				),
			},
		},
	})
}

func TestAccDataSourceMetalake_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(models.ErrorResponse{
			Code:    1002,
			Type:    "NotFound",
			Message: "Metalake not found",
		})
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: datasourceTestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_metalake" "test" {
  name = "nonexistent"
}
`,
				// The data source turns a Gravitino 404 into a targeted diagnostic
				// ("Metalake not found") instead of a raw server error.
				ExpectError: regexp.MustCompile(`(?i)metalake not found`),
			},
		},
	})
}
