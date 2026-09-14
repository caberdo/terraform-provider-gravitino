package schema_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSchemaResource_PropertiesUpdateInPlace(t *testing.T) {
	now := time.Now().UTC()
	var mu sync.Mutex
	props := map[string]string{}
	var updateTypes []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas":
			var req models.SchemaCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			props = map[string]string{}
			for k, v := range req.Properties {
				props[k] = v
			}
			mu.Unlock()
			json.NewEncoder(w).Encode(models.SchemaResponse{
				Code:   0,
				Schema: models.Schema{Name: req.Name, Comment: req.Comment, Properties: props, Audit: schemaAuditModel(now)},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/upd":
			mu.Lock()
			p := map[string]string{}
			for k, v := range props {
				p[k] = v
			}
			mu.Unlock()
			json.NewEncoder(w).Encode(models.SchemaResponse{
				Code:   0,
				Schema: models.Schema{Name: "upd", Properties: p, Audit: schemaAuditModel(now)},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/upd":
			var req models.SchemaUpdateRequest
			json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			for _, u := range req.Updates {
				m, _ := u.(map[string]interface{})
				typ, _ := m["@type"].(string)
				updateTypes = append(updateTypes, typ)
				switch typ {
				case "setProperty":
					props[m["property"].(string)] = m["value"].(string)
				case "removeProperty":
					delete(props, m["property"].(string))
				}
			}
			p := map[string]string{}
			for k, v := range props {
				p[k] = v
			}
			mu.Unlock()
			json.NewEncoder(w).Encode(models.SchemaResponse{
				Code:   0,
				Schema: models.Schema{Name: "upd", Properties: p, Audit: schemaAuditModel(now)},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/upd":
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
  metalake   = "ml"
  catalog    = "cat"
  name       = "upd"
  properties = { "env" = "dev" }
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_schema.this", "properties.env", "dev"),
			},
			{
				Config: `
resource "gravitino_schema" "this" {
  metalake   = "ml"
  catalog    = "cat"
  name       = "upd"
  properties = { "env" = "dev", "region" = "eu" }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_schema.this", "properties.%", "2"),
					resource.TestCheckResourceAttr("gravitino_schema.this", "properties.region", "eu"),
				),
			},
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if len(updateTypes) == 0 {
		t.Fatal("expected an in-place schema update to be sent")
	}
	for _, typ := range updateTypes {
		if typ != "setProperty" && typ != "removeProperty" {
			t.Errorf("schema update sent unsupported @type %q (Gravitino only supports setProperty/removeProperty)", typ)
		}
	}
}

func TestAccSchemaResource_CommentChangeReplaces(t *testing.T) {
	now := time.Now().UTC()
	var mu sync.Mutex
	comment := ""
	putCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas":
			var req models.SchemaCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			comment = req.Comment
			mu.Unlock()
			json.NewEncoder(w).Encode(models.SchemaResponse{
				Code:   0,
				Schema: models.Schema{Name: req.Name, Comment: req.Comment, Audit: schemaAuditModel(now)},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/rep":
			mu.Lock()
			c := comment
			mu.Unlock()
			json.NewEncoder(w).Encode(models.SchemaResponse{
				Code:   0,
				Schema: models.Schema{Name: "rep", Comment: c, Audit: schemaAuditModel(now)},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/rep":
			mu.Lock()
			putCalled = true
			mu.Unlock()
			http.Error(w, "in-place update not supported for schema comment", http.StatusBadRequest)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/rep":
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
  name     = "rep"
  comment  = "one"
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_schema.this", "comment", "one"),
			},
			{
				Config: `
resource "gravitino_schema" "this" {
  metalake = "ml"
  catalog  = "cat"
  name     = "rep"
  comment  = "two"
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_schema.this", "comment", "two"),
			},
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if putCalled {
		t.Error("comment change must force replacement; provider sent an in-place schema update")
	}
}
