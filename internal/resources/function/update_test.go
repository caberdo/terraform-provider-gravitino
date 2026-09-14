package function_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFunctionResource_CommentUpdateInPlace(t *testing.T) {
	now := time.Now().UTC()
	var mu sync.Mutex
	comment := ""
	var updateTypes []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/functions":
			var req models.FunctionCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			comment = req.Comment
			mu.Unlock()
			json.NewEncoder(w).Encode(models.FunctionResponse{
				Code:     0,
				Function: models.Function{Name: req.Name, Comment: req.Comment, FunctionBody: req.FunctionBody, Audit: functionAuditModel(now)},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/functions/upd":
			mu.Lock()
			c := comment
			mu.Unlock()
			json.NewEncoder(w).Encode(models.FunctionResponse{
				Code:     0,
				Function: models.Function{Name: "upd", Comment: c, FunctionBody: "echo 1", Audit: functionAuditModel(now)},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/functions/upd":
			var req models.FunctionUpdateRequest
			json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			for _, u := range req.Updates {
				m, _ := u.(map[string]interface{})
				typ, _ := m["@type"].(string)
				updateTypes = append(updateTypes, typ)
				if typ == "updateComment" {
					comment, _ = m["newComment"].(string)
				}
			}
			c := comment
			mu.Unlock()
			json.NewEncoder(w).Encode(models.FunctionResponse{
				Code:     0,
				Function: models.Function{Name: "upd", Comment: c, FunctionBody: "echo 1", Audit: functionAuditModel(now)},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/functions/upd":
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
  name          = "upd"
  function_body = "echo 1"
  comment       = "one"
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_function.this", "comment", "one"),
			},
			{
				Config: `
resource "gravitino_function" "this" {
  metalake      = "ml"
  catalog       = "cat"
  schema        = "sch"
  name          = "upd"
  function_body = "echo 1"
  comment       = "two"
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_function.this", "comment", "two"),
			},
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if len(updateTypes) == 0 {
		t.Fatal("expected an in-place function update to be sent")
	}
	for _, typ := range updateTypes {
		if typ != "updateComment" {
			t.Errorf("function update sent unsupported @type %q (Gravitino only supports updateComment/definition/impl changes)", typ)
		}
	}
}

func TestAccFunctionResource_PropertiesChangeReplaces(t *testing.T) {
	now := time.Now().UTC()
	var mu sync.Mutex
	putCalled := false
	store := map[string]map[string]string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/functions":
			var req models.FunctionCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			p := map[string]string{}
			for k, v := range req.Properties {
				p[k] = v
			}
			store[req.Name] = p
			mu.Unlock()
			json.NewEncoder(w).Encode(models.FunctionResponse{
				Code:     0,
				Function: models.Function{Name: req.Name, Comment: req.Comment, FunctionBody: req.FunctionBody, Properties: p, Audit: functionAuditModel(now)},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/metalakes/ml/catalogs/cat/schemas/sch/functions/"):
			name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			mu.Lock()
			p := map[string]string{}
			for k, v := range store[name] {
				p[k] = v
			}
			mu.Unlock()
			json.NewEncoder(w).Encode(models.FunctionResponse{
				Code:     0,
				Function: models.Function{Name: name, FunctionBody: "echo 1", Properties: p, Audit: functionAuditModel(now)},
			})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/metalakes/ml/catalogs/cat/schemas/sch/functions/"):
			mu.Lock()
			putCalled = true
			mu.Unlock()
			http.Error(w, "in-place property update not expected", http.StatusBadRequest)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/metalakes/ml/catalogs/cat/schemas/sch/functions/"):
			name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			mu.Lock()
			delete(store, name)
			mu.Unlock()
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
  name          = "rep"
  function_body = "echo 1"
  properties    = { "env" = "dev" }
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_function.this", "properties.env", "dev"),
			},
			{
				Config: `
resource "gravitino_function" "this" {
  metalake      = "ml"
  catalog       = "cat"
  schema        = "sch"
  name          = "rep"
  function_body = "echo 1"
  properties    = { "env" = "dev", "region" = "eu" }
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_function.this", "properties.region", "eu"),
			},
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if putCalled {
		t.Error("function property change must force replacement; provider sent an in-place update")
	}
}
