package view_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

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

// viewAccConfig is the smallest valid view configuration against the v1.3.0
// schema (columns + representations, no view_def).
const viewAccConfig = `
resource "gravitino_view" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "view1"
  comment  = "This is a view"

  column = [{
    name    = "id"
    type    = "long"
    comment = "id column"
  }]

  representation = [{
    type    = "sql"
    dialect = "trino"
    sql     = "SELECT id FROM t"
  }]

  properties = { key = "value" }
}
`

func writeJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
	_ = json.NewEncoder(w).Encode(value)
}

// TestAccViewResource_CreateSendsSpecPayload asserts the exact create body and
// that every computed attribute is known after apply.
func TestAccViewResource_CreateSendsSpecPayload(t *testing.T) {
	var mu sync.Mutex
	var createBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views":
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			createBody = body
			mu.Unlock()
			writeJSON(w, testViewResponse("view1"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/view1":
			writeJSON(w, testViewResponse("view1"))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/view1":
			writeJSON(w, models.DropResponse{Code: 0, Dropped: true})
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
				Config: viewAccConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_view.this", "id", "ml.cat.sch.view1"),
					resource.TestCheckResourceAttr("gravitino_view.this", "comment", "This is a view"),
					resource.TestCheckResourceAttr("gravitino_view.this", "column.#", "1"),
					resource.TestCheckResourceAttr("gravitino_view.this", "column.0.name", "id"),
					resource.TestCheckResourceAttr("gravitino_view.this", "column.0.type", "long"),
					resource.TestCheckResourceAttr("gravitino_view.this", "column.0.comment", "id column"),
					resource.TestCheckResourceAttr("gravitino_view.this", "column.0.nullable", "true"),
					resource.TestCheckResourceAttr("gravitino_view.this", "representation.#", "1"),
					resource.TestCheckResourceAttr("gravitino_view.this", "representation.0.type", "sql"),
					resource.TestCheckResourceAttr("gravitino_view.this", "representation.0.dialect", "trino"),
					resource.TestCheckResourceAttr("gravitino_view.this", "representation.0.sql", "SELECT id FROM t"),
					resource.TestCheckResourceAttr("gravitino_view.this", "properties.key", "value"),
					resource.TestCheckResourceAttrSet("gravitino_view.this", "audit.creator"),
					resource.TestCheckResourceAttrSet("gravitino_view.this", "audit.create_time"),
				),
			},
		},
	})

	mu.Lock()
	defer mu.Unlock()

	// The expected body is the ViewCreateRequest example of views.yaml; the
	// shared Column model always serialises nullable and autoIncrement, as
	// tables.yaml#/HiveTableCreate shows.
	const want = `{
		"name": "view1",
		"comment": "This is a view",
		"columns": [
			{"name": "id", "type": "long", "comment": "id column", "nullable": true, "autoIncrement": false}
		],
		"representations": [
			{"type": "sql", "dialect": "trino", "sql": "SELECT id FROM t"}
		],
		"properties": {"key": "value"}
	}`
	assertJSONEqual(t, want, createBody)
}

// TestAccViewResource_RecreatedAfterExternalDeletion proves Read removes a view
// that the server reports as gone (HTTP 404), so Terraform recreates it.
func TestAccViewResource_RecreatedAfterExternalDeletion(t *testing.T) {
	var mu sync.Mutex
	deleted := false
	createCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views":
			mu.Lock()
			createCalls++
			deleted = false
			mu.Unlock()
			writeJSON(w, testViewResponse("view1"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/view1":
			mu.Lock()
			gone := deleted
			mu.Unlock()
			if gone {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(noSuchViewBody))
				return
			}
			writeJSON(w, testViewResponse("view1"))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/view1":
			writeJSON(w, models.DropResponse{Code: 0, Dropped: true})
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
				Config: viewAccConfig,
				Check:  resource.TestCheckResourceAttr("gravitino_view.this", "id", "ml.cat.sch.view1"),
			},
			{
				PreConfig: func() {
					mu.Lock()
					deleted = true
					mu.Unlock()
				},
				Config: viewAccConfig,
				Check:  resource.TestCheckResourceAttr("gravitino_view.this", "id", "ml.cat.sch.view1"),
			},
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if createCalls != 2 {
		t.Errorf("expected the view to be created twice (recreated after the external 404), got %d creates", createCalls)
	}
}

// TestAccViewResource_Import covers importing a view with the dot separated ID.
func TestAccViewResource_Import(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views":
			writeJSON(w, testViewResponse("view1"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/view1":
			writeJSON(w, testViewResponse("view1"))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/views/view1":
			writeJSON(w, models.DropResponse{Code: 0, Dropped: true})
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
				Config: viewAccConfig,
			},
			{
				ResourceName:      "gravitino_view.this",
				ImportState:       true,
				ImportStateId:     "ml.cat.sch.view1",
				ImportStateVerify: true,
			},
		},
	})
}
