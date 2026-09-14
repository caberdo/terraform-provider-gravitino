package view_test

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

func viewTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func TestAccViewResource_CreateWithoutComment(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views":
			var req models.ViewCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.ViewResponse{
				Code: 0,
				View: models.View{
					Name:    req.Name,
					Comment: req.Comment,
					ViewDef: req.ViewDef,
					Audit:   viewAuditModel(now),
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/myview":
			// Real Gravitino omits/returns an empty comment when none was set.
			json.NewEncoder(w).Encode(models.ViewResponse{
				Code: 0,
				View: models.View{
					Name:    "myview",
					Comment: "",
					ViewDef: "SELECT 1",
					Audit:   viewAuditModel(now),
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/myview":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: viewTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_view" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "myview"
  view_def = "SELECT 1"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_view.this", "name", "myview"),
					resource.TestCheckResourceAttr("gravitino_view.this", "id", "ml.cat.sch.myview"),
					resource.TestCheckNoResourceAttr("gravitino_view.this", "comment"),
				),
			},
		},
	})
}

func TestAccViewResource_CreateWithComment(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views":
			var req models.ViewCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.ViewResponse{
				Code: 0,
				View: models.View{
					Name:    req.Name,
					Comment: req.Comment,
					ViewDef: req.ViewDef,
					Audit:   viewAuditModel(now),
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/with_comment":
			json.NewEncoder(w).Encode(models.ViewResponse{
				Code: 0,
				View: models.View{
					Name:    "with_comment",
					Comment: "a view comment",
					ViewDef: "SELECT 1",
					Audit:   viewAuditModel(now),
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/with_comment":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: viewTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_view" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "with_comment"
  view_def = "SELECT 1"
  comment  = "a view comment"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_view.this", "name", "with_comment"),
					resource.TestCheckResourceAttr("gravitino_view.this", "comment", "a view comment"),
				),
			},
		},
	})
}

func viewAuditModel(t time.Time) *models.Audit {
	return &models.Audit{
		Creator:          "admin",
		CreateTime:       &t,
		LastModifier:     "admin",
		LastModifiedTime: &t,
	}
}