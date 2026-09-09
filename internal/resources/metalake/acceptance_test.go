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
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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

func TestAccMetalakeResource_NoDriftWithServerDroppedProperty(t *testing.T) {
	currentProps := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes":
			var req models.MetalakeCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			currentProps = req.Properties
			// Real Gravitino does not echo back the special "in-use" property.
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:    req.Name,
					Comment: req.Comment,
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/metalakes/inuse_ml":
			var req models.MetalakeUpdateRequest
			json.NewDecoder(r.Body).Decode(&req)
			for _, u := range req.Updates {
				if m, ok := u.(map[string]interface{}); ok {
					switch m["@type"] {
					case "removeProperty":
						delete(currentProps, m["property"].(string))
					case "setProperty":
						currentProps[m["property"].(string)] = m["value"].(string)
					}
				}
			}
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:       "inuse_ml",
					Comment:    "in-use metalake",
					Properties: currentProps,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/inuse_ml":
			// Simulate a server that drops one configured property (region) but
			// keeps another (env). The provider must not drop region from state.
			props := make(map[string]string)
			for k, v := range currentProps {
				if k != "region" {
					props[k] = v
				}
			}
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:       "inuse_ml",
					Comment:    "in-use metalake",
					Properties: props,
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/inuse_ml":
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
  name       = "inuse_ml"
  comment    = "in-use metalake"
  properties = { "env" = "dev", "region" = "eu" }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_metalake.this", "name", "inuse_ml"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.%", "2"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.env", "dev"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.region", "eu"),
				),
			},
			{
				Config: `
resource "gravitino_metalake" "this" {
  name       = "inuse_ml"
  comment    = "in-use metalake"
  properties = { "env" = "dev" }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_metalake.this", "name", "inuse_ml"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.%", "1"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.env", "dev"),
				),
			},
		},
	})
}

func TestAccMetalakeResource_ImportWithServerOnlyProperties(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes":
			var req models.MetalakeCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:    req.Name,
					Comment: req.Comment,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/srvprops_ml":
			// Server returns a property that is absent from the config/state;
			// Read must merge it in without panicking on a nil map.
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:       "srvprops_ml",
					Comment:    "server props",
					Properties: map[string]string{"env": "dev"},
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/srvprops_ml":
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
  name    = "srvprops_ml"
  comment = "server props"
}
`,
			},
			{
				ImportState:       true,
				ResourceName:      "gravitino_metalake.this",
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"comment",
					"properties",
				},
			},
		},
	})
}

func TestAccMetalakeResource_UpdateDoesNotRemoveReservedProperty(t *testing.T) {
	var reservedRemovalAttempted bool
	currentComment := "initial comment"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes":
			var req models.MetalakeCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			currentComment = req.Comment
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:       req.Name,
					Comment:    req.Comment,
					Properties: req.Properties,
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/metalakes/upd_ml":
			var req models.MetalakeUpdateRequest
			json.NewDecoder(r.Body).Decode(&req)
			for _, u := range req.Updates {
				if m, ok := u.(map[string]interface{}); ok {
					if m["property"] == "in-use" {
						reservedRemovalAttempted = true
					}
					if m["@type"] == "updateComment" {
						currentComment = m["newComment"].(string)
					}
				}
			}
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:       "upd_ml",
					Comment:    currentComment,
					Properties: map[string]string{"in-use": "true"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/upd_ml":
			json.NewEncoder(w).Encode(models.MetalakeResponse{
				Code: 0,
				Metalake: models.Metalake{
					Name:       "upd_ml",
					Comment:    currentComment,
					Properties: map[string]string{"in-use": "true"},
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/upd_ml":
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
  name    = "upd_ml"
  comment = "initial comment"
}
`,
			},
			{
				Config: `
resource "gravitino_metalake" "this" {
  name    = "upd_ml"
  comment = "updated comment"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_metalake.this", "comment", "updated comment"),
					func(s *terraform.State) error {
						if reservedRemovalAttempted {
							t.Fatal("provider must not send removeProperty for reserved 'in-use'")
						}
						return nil
					},
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
