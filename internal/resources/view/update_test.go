package view_test

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

func TestAccViewResource_PropertiesUpdateInPlace(t *testing.T) {
	now := time.Now().UTC()
	var mu sync.Mutex
	props := map[string]string{}
	var updateTypes []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views":
			var req models.ViewCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			props = map[string]string{}
			for k, v := range req.Properties {
				props[k] = v
			}
			mu.Unlock()
			json.NewEncoder(w).Encode(models.ViewResponse{
				Code: 0,
				View: models.View{Name: req.Name, Comment: req.Comment, ViewDef: req.ViewDef, Properties: props, Audit: viewAuditModel(now)},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/upd":
			mu.Lock()
			p := map[string]string{}
			for k, v := range props {
				p[k] = v
			}
			mu.Unlock()
			json.NewEncoder(w).Encode(models.ViewResponse{
				Code: 0,
				View: models.View{Name: "upd", ViewDef: "SELECT 1", Properties: p, Audit: viewAuditModel(now)},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/upd":
			var req models.ViewUpdateRequest
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
			json.NewEncoder(w).Encode(models.ViewResponse{
				Code: 0,
				View: models.View{Name: "upd", ViewDef: "SELECT 1", Properties: p, Audit: viewAuditModel(now)},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/upd":
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
  metalake   = "ml"
  catalog    = "cat"
  schema     = "sch"
  name       = "upd"
  view_def   = "SELECT 1"
  properties = { "env" = "dev" }
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_view.this", "properties.env", "dev"),
			},
			{
				Config: `
resource "gravitino_view" "this" {
  metalake   = "ml"
  catalog    = "cat"
  schema     = "sch"
  name       = "upd"
  view_def   = "SELECT 1"
  properties = { "env" = "dev", "region" = "eu" }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_view.this", "properties.%", "2"),
					resource.TestCheckResourceAttr("gravitino_view.this", "properties.region", "eu"),
				),
			},
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if len(updateTypes) == 0 {
		t.Fatal("expected an in-place view update to be sent")
	}
	for _, typ := range updateTypes {
		if typ != "setProperty" && typ != "removeProperty" {
			t.Errorf("view update sent unsupported @type %q (Gravitino does not support updateComment for views)", typ)
		}
	}
}

func TestAccViewResource_CommentChangeReplaces(t *testing.T) {
	now := time.Now().UTC()
	var mu sync.Mutex
	comment := ""
	putCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views":
			var req models.ViewCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			comment = req.Comment
			mu.Unlock()
			json.NewEncoder(w).Encode(models.ViewResponse{
				Code: 0,
				View: models.View{Name: req.Name, Comment: req.Comment, ViewDef: req.ViewDef, Audit: viewAuditModel(now)},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/rep":
			mu.Lock()
			c := comment
			mu.Unlock()
			json.NewEncoder(w).Encode(models.ViewResponse{
				Code: 0,
				View: models.View{Name: "rep", Comment: c, ViewDef: "SELECT 1", Audit: viewAuditModel(now)},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/rep":
			mu.Lock()
			putCalled = true
			mu.Unlock()
			http.Error(w, "in-place update not supported for view comment", http.StatusBadRequest)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/rep":
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
  name     = "rep"
  view_def = "SELECT 1"
  comment  = "one"
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_view.this", "comment", "one"),
			},
			{
				Config: `
resource "gravitino_view" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "rep"
  view_def = "SELECT 1"
  comment  = "two"
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_view.this", "comment", "two"),
			},
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if putCalled {
		t.Error("view comment change must force replacement; provider sent an in-place update")
	}
}
