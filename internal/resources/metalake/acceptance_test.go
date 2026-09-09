package metalake_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func testAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func TestAccMetalakeResource_CreateWithAudit(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes":
			var req models.MetalakeCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:       req.Name,
					Comment:    req.Comment,
					Properties: req.Properties,
					Audit: &models.Audit{
						Creator:          "admin",
						CreateTime:       timePtr(now),
						LastModifier:     "admin",
						LastModifiedTime: timePtr(now),
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/audit_ml":
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:       "audit_ml",
					Comment:    "metalake with audit",
					Properties: map[string]string{},
					Audit: &models.Audit{
						Creator:          "admin",
						CreateTime:       timePtr(now),
						LastModifier:     "admin",
						LastModifiedTime: timePtr(now),
					},
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/audit_ml":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_metalake" "this" {
  name    = "audit_ml"
  comment = "metalake with audit"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_metalake.this", "name", "audit_ml"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "comment", "metalake with audit"),
					resource.TestCheckResourceAttrSet("gravitino_metalake.this", "audit.creator"),
					resource.TestCheckResourceAttrSet("gravitino_metalake.this", "audit.create_time"),
				),
			},
		},
	})
}

func TestAccMetalakeResource_CreateWithEmptyProperties(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes":
			var req models.MetalakeCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:       req.Name,
					Comment:    req.Comment,
					Properties: req.Properties,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/props_ml":
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:    "props_ml",
					Comment: "empty props",
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/props_ml":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_metalake" "this" {
  name       = "props_ml"
  comment    = "empty props"
  properties = {}
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_metalake.this", "name", "props_ml"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.%", "0"),
				),
			},
		},
	})
}

func timePtr(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}
