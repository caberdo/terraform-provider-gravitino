package function_test

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

func functionTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func TestAccFunctionResource_CreateWithoutComment(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/functions":
			var req models.FunctionCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.FunctionResponse{
				Code: 0,
				Function: models.Function{
					Name:         req.Name,
					Comment:      req.Comment,
					FunctionBody: req.FunctionBody,
					Audit:        functionAuditModel(now),
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/functions/myfunc":
			// Real Gravitino omits/returns an empty comment when none was set.
			json.NewEncoder(w).Encode(models.FunctionResponse{
				Code: 0,
				Function: models.Function{
					Name:         "myfunc",
					Comment:      "",
					FunctionBody: "echo 1",
					Audit:        functionAuditModel(now),
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/functions/myfunc":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: functionTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_function" "this" {
  metalake      = "ml"
  catalog       = "cat"
  schema        = "sch"
  name          = "myfunc"
  function_body = "echo 1"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_function.this", "name", "myfunc"),
					resource.TestCheckResourceAttr("gravitino_function.this", "id", "ml.cat.sch.myfunc"),
					resource.TestCheckNoResourceAttr("gravitino_function.this", "comment"),
				),
			},
		},
	})
}

func TestAccFunctionResource_CreateWithComment(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/functions":
			var req models.FunctionCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.FunctionResponse{
				Code: 0,
				Function: models.Function{
					Name:         req.Name,
					Comment:      req.Comment,
					FunctionBody: req.FunctionBody,
					Audit:        functionAuditModel(now),
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/functions/with_comment":
			json.NewEncoder(w).Encode(models.FunctionResponse{
				Code: 0,
				Function: models.Function{
					Name:         "with_comment",
					Comment:      "a function comment",
					FunctionBody: "echo 1",
					Audit:        functionAuditModel(now),
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/functions/with_comment":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: functionTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_function" "this" {
  metalake      = "ml"
  catalog       = "cat"
  schema        = "sch"
  name          = "with_comment"
  function_body = "echo 1"
  comment       = "a function comment"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_function.this", "name", "with_comment"),
					resource.TestCheckResourceAttr("gravitino_function.this", "comment", "a function comment"),
				),
			},
		},
	})
}

func functionAuditModel(t time.Time) *models.Audit {
	return &models.Audit{
		Creator:          "admin",
		CreateTime:       &t,
		LastModifier:     "admin",
		LastModifiedTime: &t,
	}
}