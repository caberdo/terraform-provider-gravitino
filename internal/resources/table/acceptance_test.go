package table_test

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

func tableTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func TestAccTableResource_CreateWithAudit(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/tables":
			var req models.TableCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.TableResponse{
				Code: 0,
				Table: models.Table{
					Name:         req.Name,
					Comment:      req.Comment,
					Columns:      req.Columns,
					Distribution: req.Distribution,
					Audit:        auditModel(now),
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/tables/tbl":
			json.NewEncoder(w).Encode(models.TableResponse{
				Code: 0,
				Table: models.Table{
					Name:         "tbl",
					Comment:      "table with audit",
					Columns:      []models.Column{{Name: "id", Type: models.DataType{Type: "long"}, Nullable: true}},
					Distribution: &models.Distribution{Strategy: "hash", Number: 1},
					Audit:        auditModel(now),
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/tables/tbl":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: tableTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_table" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "tbl"
  comment  = "table with audit"

  column {
    name = "id"
    type = "long"
  }

  distribution {
    number = 1
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_table.this", "name", "tbl"),
					resource.TestCheckResourceAttr("gravitino_table.this", "comment", "table with audit"),
					resource.TestCheckResourceAttrSet("gravitino_table.this", "audit.creator"),
					resource.TestCheckResourceAttrSet("gravitino_table.this", "audit.create_time"),
				),
			},
		},
	})
}

func TestAccTableResource_NoDriftWithServerDroppedProperty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/tables":
			var req models.TableCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.TableResponse{
				Code: 0,
				Table: models.Table{
					Name:         req.Name,
					Columns:      req.Columns,
					Distribution: req.Distribution,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/tables/tbl2":
			// Server keeps one configured property but drops the reserved one.
			json.NewEncoder(w).Encode(models.TableResponse{
				Code: 0,
				Table: models.Table{
					Name:         "tbl2",
					Columns:      []models.Column{{Name: "id", Type: models.DataType{Type: "long"}, Nullable: true}},
					Distribution: &models.Distribution{Strategy: "hash", Number: 1},
					Properties:   map[string]string{"env": "dev"},
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/tables/tbl2":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: tableTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_table" "this" {
  metalake   = "ml"
  catalog    = "cat"
  schema     = "sch"
  name       = "tbl2"
  properties = { "env" = "dev", "region" = "eu" }

  column {
    name = "id"
    type = "long"
  }

  distribution {
    number = 1
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_table.this", "name", "tbl2"),
					resource.TestCheckResourceAttr("gravitino_table.this", "properties.%", "2"),
					resource.TestCheckResourceAttr("gravitino_table.this", "properties.env", "dev"),
					resource.TestCheckResourceAttr("gravitino_table.this", "properties.region", "eu"),
				),
			},
		},
	})
}

func TestAccTableResource_CreateWithEmptyProperties(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/tables":
			var req models.TableCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.TableResponse{
				Code: 0,
				Table: models.Table{
					Name:         req.Name,
					Columns:      req.Columns,
					Distribution: req.Distribution,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/tables/tbl3":
			json.NewEncoder(w).Encode(models.TableResponse{
				Code: 0,
				Table: models.Table{
					Name:         "tbl3",
					Columns:      []models.Column{{Name: "id", Type: models.DataType{Type: "long"}, Nullable: true}},
					Distribution: &models.Distribution{Strategy: "hash", Number: 1},
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/tables/tbl3":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: tableTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_table" "this" {
  metalake   = "ml"
  catalog    = "cat"
  schema     = "sch"
  name       = "tbl3"
  properties = {}

  column {
    name = "id"
    type = "long"
  }

  distribution {
    number = 1
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_table.this", "name", "tbl3"),
					resource.TestCheckResourceAttr("gravitino_table.this", "properties.%", "0"),
				),
			},
		},
	})
}

func auditModel(t time.Time) *models.Audit {
	return &models.Audit{
		Creator:          "admin",
		CreateTime:       &t,
		LastModifier:     "admin",
		LastModifiedTime: &t,
	}
}
