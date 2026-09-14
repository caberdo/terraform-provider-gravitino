package schema_test

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

func schemaTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func TestAccSchemaResource_CreateWithoutComment(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas":
			var req models.SchemaCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.SchemaResponse{
				Code: 0,
				Schema: models.Schema{
					Name:    req.Name,
					Comment: req.Comment,
					Audit:   schemaAuditModel(now),
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch":
			// Real Gravitino omits/returns an empty comment when none was set.
			json.NewEncoder(w).Encode(models.SchemaResponse{
				Code: 0,
				Schema: models.Schema{
					Name:    "sch",
					Comment: "",
					Audit:   schemaAuditModel(now),
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: schemaTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_schema" "this" {
  metalake = "ml"
  catalog  = "cat"
  name     = "sch"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_schema.this", "name", "sch"),
					resource.TestCheckResourceAttr("gravitino_schema.this", "id", "ml.cat.sch"),
					resource.TestCheckNoResourceAttr("gravitino_schema.this", "comment"),
				),
			},
		},
	})
}

func TestAccSchemaResource_CreateWithComment(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas":
			var req models.SchemaCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.SchemaResponse{
				Code: 0,
				Schema: models.Schema{
					Name:    req.Name,
					Comment: req.Comment,
					Audit:   schemaAuditModel(now),
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/with_comment":
			json.NewEncoder(w).Encode(models.SchemaResponse{
				Code: 0,
				Schema: models.Schema{
					Name:    "with_comment",
					Comment: "a schema comment",
					Audit:   schemaAuditModel(now),
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/with_comment":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: schemaTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_schema" "this" {
  metalake = "ml"
  catalog  = "cat"
  name     = "with_comment"
  comment  = "a schema comment"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_schema.this", "name", "with_comment"),
					resource.TestCheckResourceAttr("gravitino_schema.this", "comment", "a schema comment"),
				),
			},
		},
	})
}

func schemaAuditModel(t time.Time) *models.Audit {
	return &models.Audit{
		Creator:          "admin",
		CreateTime:       &t,
		LastModifier:     "admin",
		LastModifiedTime: &t,
	}
}