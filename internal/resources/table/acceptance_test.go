package table_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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

// tableMock is a small in-memory Gravitino table API used by the update
// acceptance tests. It records the update requests it received, so a test can
// assert the payload the provider sent.
type tableMock struct {
	mu    sync.Mutex
	table map[string]interface{}
	// seed holds the values the fake server assigns itself (e.g. an index name it
	// derives, or a default it fills in). A create response is the seed with the
	// request's fields applied on top, like a real server returns the effective object.
	seed    map[string]interface{}
	updates []map[string]interface{}
	creates int
	deletes int
}

func newTableMock(name string, columns []map[string]interface{}, extra map[string]interface{}) *tableMock {
	table := map[string]interface{}{
		"name":    name,
		"columns": toInterfaceSlice(columns),
		"audit": map[string]interface{}{
			"creator":    "admin",
			"createTime": "2023-12-08T11:07:46.938Z",
		},
	}
	for key, value := range extra {
		table[key] = value
	}
	return &tableMock{table: table, seed: deepCopyMap(table)}
}

// deepCopyMap copies a JSON-shaped map so mock state cannot be mutated through a
// shared slice or map while a test runs.
func deepCopyMap(in map[string]interface{}) map[string]interface{} {
	raw, err := json.Marshal(in)
	if err != nil {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

// assignIndexNames fills in the index names the fake server assigns itself, by
// position, when the request did not provide one.
func assignIndexNames(table, seed map[string]interface{}) {
	indexes, _ := table["indexes"].([]interface{})
	seedIndexes, _ := seed["indexes"].([]interface{})
	for i, raw := range indexes {
		if i >= len(seedIndexes) {
			break
		}
		index, _ := raw.(map[string]interface{})
		seedIndex, _ := seedIndexes[i].(map[string]interface{})
		if index == nil || seedIndex == nil {
			continue
		}
		if name, _ := index["name"].(string); name == "" {
			if seedName, _ := seedIndex["name"].(string); seedName != "" {
				index["name"] = seedName
			}
		}
	}
}

func toInterfaceSlice(values []map[string]interface{}) []interface{} {
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func (m *tableMock) server(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")

		m.mu.Lock()
		defer m.mu.Unlock()

		switch r.Method {
		case http.MethodPost:
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)

			m.creates++
			table := deepCopyMap(m.seed)
			table["name"] = body["name"]
			table["columns"] = body["columns"]
			table["audit"] = m.seed["audit"]
			for _, key := range []string{"comment", "properties", "sortOrders", "distribution", "partitioning", "indexes"} {
				if value, ok := body[key]; ok {
					table[key] = value
				}
			}
			// A catalog assigns an index name when the request omits it.
			assignIndexNames(table, m.seed)
			m.table = table
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "table": m.table})
		case http.MethodPut:
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)

			rawUpdates, _ := body["updates"].([]interface{})
			for _, raw := range rawUpdates {
				update, _ := raw.(map[string]interface{})
				m.updates = append(m.updates, update)
				m.applyUpdate(update)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "table": m.table})
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "table": m.table})
		case http.MethodDelete:
			m.deletes++
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "dropped": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func (m *tableMock) applyUpdate(update map[string]interface{}) {
	fieldName := ""
	if raw, ok := update["fieldName"].([]interface{}); ok && len(raw) == 1 {
		fieldName, _ = raw[0].(string)
	}

	columns, _ := m.table["columns"].([]interface{})
	for _, raw := range columns {
		column, _ := raw.(map[string]interface{})
		if column["name"] != fieldName {
			continue
		}

		switch update["@type"] {
		case "updateColumnComment":
			column["comment"] = update["newComment"]
		case "updateColumnType":
			column["type"] = update["newType"]
		case "updateColumnNullability":
			column["nullable"] = update["nullable"]
		case "updateColumnDefaultValue":
			if update["newDefaultValue"] == nil {
				delete(column, "defaultValue")
				break
			}
			column["defaultValue"] = update["newDefaultValue"]
		}
	}

	switch update["@type"] {
	case "rename":
		m.table["name"] = update["newName"]
	case "updateComment":
		m.table["comment"] = update["newComment"]
	case "setProperty":
		properties, _ := m.table["properties"].(map[string]interface{})
		if properties == nil {
			properties = map[string]interface{}{}
		}
		properties[update["property"].(string)] = update["value"]
		m.table["properties"] = properties
	case "removeProperty":
		if properties, ok := m.table["properties"].(map[string]interface{}); ok {
			delete(properties, update["property"].(string))
		}
	}
}

func (m *tableMock) recordedUpdates() []map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]map[string]interface{}, len(m.updates))
	copy(result, m.updates)
	return result
}

func (m *tableMock) createCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.creates
}

// TestAccTableResource_ColumnCommentUpdateConverges is the regression test for
// the update path: a column change is applied with the updateColumnComment
// request of tables.yaml and the applied state is consistent, so Terraform does
// not report "Provider produced inconsistent result after apply".
func TestAccTableResource_ColumnCommentUpdateConverges(t *testing.T) {
	mock := newTableMock("acc_tbl", []map[string]interface{}{
		{
			"name": "id", "type": "long", "comment": "id column comment",
			"nullable": true, "autoIncrement": false,
		},
	}, map[string]interface{}{
		"comment":    "table comment",
		"properties": map[string]interface{}{"format": "ORC"},
	})
	server := mock.server(t)
	t.Setenv("GRAVITINO_URI", server.URL)

	config := func(comment string) string {
		return `
resource "gravitino_table" "this" {
  metalake   = "ml"
  catalog    = "cat"
  schema     = "sch"
  name       = "acc_tbl"
  comment    = "table comment"
  properties = { "format" = "ORC" }

  column {
    name    = "id"
    type    = "long"
    comment = "` + comment + `"
  }
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: tableTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("id column comment"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_table.this", "column.0.comment", "id column comment"),
					func(state *terraform.State) error {
						if got := mock.createCount(); got != 1 {
							return fmt.Errorf("expected one create, got %d", got)
						}
						return nil
					},
				),
			},
			{
				Config: config("id column comment v2"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_table.this", "column.0.comment", "id column comment v2"),
					func(state *terraform.State) error {
						if got := mock.createCount(); got != 1 {
							return fmt.Errorf("the table must not be replaced, got %d creates", got)
						}

						updates := mock.recordedUpdates()
						if len(updates) != 1 {
							return fmt.Errorf("expected one update request, got %v", updates)
						}
						update := updates[0]
						if update["@type"] != "updateColumnComment" {
							return fmt.Errorf("update type = %v", update["@type"])
						}
						if update["newComment"] != "id column comment v2" {
							return fmt.Errorf("newComment = %v", update["newComment"])
						}
						fieldName, ok := update["fieldName"].([]interface{})
						if !ok || len(fieldName) != 1 || fieldName[0] != "id" {
							return fmt.Errorf("fieldName = %v", update["fieldName"])
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccTableResource_ImmutableBlockChangeReplacesTable asserts a change to
// partitioning, which Gravitino cannot update in place, turns into a
// replacement instead of being ignored.
func TestAccTableResource_ImmutableBlockChangeReplacesTable(t *testing.T) {
	mock := newTableMock("acc_part_tbl", []map[string]interface{}{
		{"name": "id", "type": "long", "nullable": true, "autoIncrement": false},
		{"name": "dt", "type": "date", "nullable": true, "autoIncrement": false},
	}, map[string]interface{}{
		"partitioning": []interface{}{
			map[string]interface{}{"strategy": "identity", "fieldName": []interface{}{"dt"}},
		},
	})
	server := mock.server(t)
	t.Setenv("GRAVITINO_URI", server.URL)

	config := func(field string) string {
		return `
resource "gravitino_table" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "acc_part_tbl"

  column {
    name = "id"
    type = "long"
  }

  column {
    name = "dt"
    type = "date"
  }

  partitioning {
    strategy   = "identity"
    field_name = ["` + field + `"]
  }
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: tableTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("dt"),
				Check:  resource.TestCheckResourceAttr("gravitino_table.this", "partitioning.0.field_name.0", "dt"),
			},
			{
				Config: config("id"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_table.this", "partitioning.0.field_name.0", "id"),
					func(state *terraform.State) error {
						if got := mock.createCount(); got != 2 {
							return fmt.Errorf("expected the table to be replaced, got %d creates", got)
						}
						if updates := mock.recordedUpdates(); len(updates) != 0 {
							return fmt.Errorf("no update request may be sent for partitioning, got %v", updates)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccTableResource_ServerAssignedBlockValues asserts the values Gravitino
// assigns inside sort_order and index blocks (the default null ordering and the
// index name) are stored instead of leaving an unknown value in the state.
func TestAccTableResource_ServerAssignedBlockValues(t *testing.T) {
	mock := newTableMock("acc_srv_tbl", []map[string]interface{}{
		{"name": "id", "type": "long", "nullable": true, "autoIncrement": false},
	}, map[string]interface{}{
		"sortOrders": []interface{}{
			map[string]interface{}{
				"sortTerm":     map[string]interface{}{"type": "field", "fieldName": []interface{}{"id"}},
				"direction":    "asc",
				"nullOrdering": "nulls_first",
			},
		},
		"indexes": []interface{}{
			map[string]interface{}{
				"indexType":  "PRIMARY_KEY",
				"name":       "PRIMARY",
				"fieldNames": []interface{}{[]interface{}{"id"}},
			},
		},
	})
	server := mock.server(t)
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
  name     = "acc_srv_tbl"

  column {
    name = "id"
    type = "long"
  }

  sort_order {
    field_name = ["id"]
  }

  index {
    index_type  = "primary_key"
    field_names = [["id"]]
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_table.this", "sort_order.0.null_ordering", "nulls_first"),
					resource.TestCheckResourceAttr("gravitino_table.this", "index.0.name", "PRIMARY"),
					resource.TestCheckResourceAttr("gravitino_table.this", "index.0.index_type", "primary_key"),
				),
			},
		},
	})
}
