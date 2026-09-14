package model_test

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

func modelTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func TestAccModelResource_CreateWithoutComment(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/models":
			var req models.ModelCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.ModelResponse{
				Code: 0,
				Model: models.Model{
					Name:     req.Name,
					Comment:  req.Comment,
					ModelURI: req.ModelURI,
					Audit:    modelAuditModel(now),
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/models/mymodel":
			// Real Gravitino omits/returns an empty comment when none was set.
			json.NewEncoder(w).Encode(models.ModelResponse{
				Code: 0,
				Model: models.Model{
					Name:     "mymodel",
					Comment:  "",
					ModelURI: "file:///tmp/model",
					Audit:    modelAuditModel(now),
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/models/mymodel":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: modelTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_model" "this" {
  metalake  = "ml"
  catalog   = "cat"
  schema    = "sch"
  name      = "mymodel"
  model_uri = "file:///tmp/model"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_model.this", "name", "mymodel"),
					resource.TestCheckResourceAttr("gravitino_model.this", "id", "ml.cat.sch.mymodel"),
					resource.TestCheckNoResourceAttr("gravitino_model.this", "comment"),
				),
			},
		},
	})
}

func TestAccModelResource_CreateWithComment(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/models":
			var req models.ModelCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.ModelResponse{
				Code: 0,
				Model: models.Model{
					Name:     req.Name,
					Comment:  req.Comment,
					ModelURI: req.ModelURI,
					Audit:    modelAuditModel(now),
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/models/with_comment":
			json.NewEncoder(w).Encode(models.ModelResponse{
				Code: 0,
				Model: models.Model{
					Name:     "with_comment",
					Comment:  "a model comment",
					ModelURI: "file:///tmp/model",
					Audit:    modelAuditModel(now),
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/models/with_comment":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: modelTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_model" "this" {
  metalake  = "ml"
  catalog   = "cat"
  schema    = "sch"
  name      = "with_comment"
  model_uri = "file:///tmp/model"
  comment   = "a model comment"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_model.this", "name", "with_comment"),
					resource.TestCheckResourceAttr("gravitino_model.this", "comment", "a model comment"),
				),
			},
		},
	})
}

func modelAuditModel(t time.Time) *models.Audit {
	return &models.Audit{
		Creator:          "admin",
		CreateTime:       &t,
		LastModifier:     "admin",
		LastModifiedTime: &t,
	}
}