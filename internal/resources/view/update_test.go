package view_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// mockView is a minimal stateful Gravitino view server for the acceptance
// tests. It applies the update operations it receives, so the provider sees the
// same state Terraform believes in.
type mockView struct {
	mu               sync.Mutex
	name             string
	comment          string
	props            map[string]string
	createTime       time.Time
	columns          []models.Column
	representations  []models.ViewRepresentation
	lastModifier     string
	lastModifiedTime time.Time
	creates          int
	updateBodies     [][]byte
}

func newMockView(name, comment string, props map[string]string) *mockView {
	now := time.Now().UTC().Truncate(time.Second)
	return &mockView{
		name:             name,
		comment:          comment,
		props:            props,
		createTime:       now,
		lastModifiedTime: now,
		columns: []models.Column{
			{Name: "id", Type: models.DataType{Type: "long"}, Comment: "id column", Nullable: true},
		},
		representations: []models.ViewRepresentation{
			{Type: "sql", Dialect: "trino", SQL: "SELECT id FROM t"},
		},
	}
}

func (m *mockView) currentLocked() models.View {
	props := make(map[string]string, len(m.props))
	for k, v := range m.props {
		props[k] = v
	}
	return models.View{
		Name:            m.name,
		Comment:         m.comment,
		Columns:         m.columns,
		Representations: m.representations,
		Properties:      props,
		// Like the real server, audit changes on every modification. An audit
		// attribute with UseStateForUnknown would make Terraform reject the
		// applied value.
		Audit: &models.Audit{
			Creator:          "anonymous",
			CreateTime:       &m.createTime,
			LastModifier:     m.lastModifier,
			LastModifiedTime: &m.lastModifiedTime,
		},
	}
}

func (m *mockView) handler() http.HandlerFunc {
	const collection = "/api/metalakes/ml/catalogs/cat/schemas/sch/views"
	const prefix = collection + "/"

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == collection:
			var req models.ViewCreateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			m.mu.Lock()
			m.creates++
			m.name = req.Name
			m.comment = req.Comment
			m.lastModifier = "anonymous"
			m.lastModifiedTime = time.Now().UTC().Truncate(time.Second)
			m.props = make(map[string]string, len(req.Properties))
			for k, v := range req.Properties {
				m.props[k] = v
			}
			view := m.currentLocked()
			m.mu.Unlock()
			writeJSON(w, models.ViewResponse{Code: 0, View: view})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, prefix):
			m.mu.Lock()
			view := m.currentLocked()
			m.mu.Unlock()
			if strings.TrimPrefix(r.URL.Path, prefix) != view.Name {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(noSuchViewBody))
				return
			}
			writeJSON(w, models.ViewResponse{Code: 0, View: view})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, prefix):
			body, _ := io.ReadAll(r.Body)
			var req models.ViewUpdateRequest
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			m.mu.Lock()
			for _, u := range req.Updates {
				update, _ := u.(map[string]interface{})
				switch update["@type"] {
				case "rename":
					m.name, _ = update["newName"].(string)
				case "setProperty":
					property, _ := update["property"].(string)
					value, _ := update["value"].(string)
					m.props[property] = value
				case "removeProperty":
					property, _ := update["property"].(string)
					delete(m.props, property)
				}
			}
			m.updateBodies = append(m.updateBodies, body)
			m.lastModifier = "anonymous"
			m.lastModifiedTime = time.Now().UTC().Truncate(time.Second)
			view := m.currentLocked()
			m.mu.Unlock()
			writeJSON(w, models.ViewResponse{Code: 0, View: view})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, prefix):
			writeJSON(w, models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}
}

func (m *mockView) start(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(m.handler())
	t.Cleanup(server.Close)
	t.Setenv("GRAVITINO_URI", server.URL)
}

func viewAccConfigWithName(name string) string {
	return `
resource "gravitino_view" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "` + name + `"
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
}

func viewAccConfigWithProps(props string) string {
	return `
resource "gravitino_view" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "upd"
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

  properties = { ` + props + ` }
}
`
}

// TestAccViewResource_RenameInPlace asserts that a name change is sent as the
// rename update and that the compound ID follows the new name.
func TestAccViewResource_RenameInPlace(t *testing.T) {
	mock := newMockView("view1", "This is a view", map[string]string{"key": "value"})
	mock.start(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: viewTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: viewAccConfigWithName("view1"),
				Check:  resource.TestCheckResourceAttr("gravitino_view.this", "id", "ml.cat.sch.view1"),
			},
			{
				Config: viewAccConfigWithName("view2"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_view.this", "name", "view2"),
					resource.TestCheckResourceAttr("gravitino_view.this", "id", "ml.cat.sch.view2"),
				),
			},
		},
	})

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.updateBodies) != 1 {
		t.Fatalf("expected exactly one update request, got %d", len(mock.updateBodies))
	}
	const want = `{"updates": [{"@type": "rename", "newName": "view2"}]}`
	assertJSONEqual(t, want, mock.updateBodies[0])
}

// TestAccViewResource_PropertiesUpdateInPlace asserts the exact setProperty
// payload of an in-place property update.
func TestAccViewResource_PropertiesUpdateInPlace(t *testing.T) {
	mock := newMockView("upd", "This is a view", map[string]string{"env": "dev"})
	mock.start(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: viewTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: viewAccConfigWithProps(`env = "dev"`),
				Check:  resource.TestCheckResourceAttr("gravitino_view.this", "properties.env", "dev"),
			},
			{
				Config: viewAccConfigWithProps("env = \"dev\", region = \"eu\""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_view.this", "properties.%", "2"),
					resource.TestCheckResourceAttr("gravitino_view.this", "properties.region", "eu"),
					resource.TestCheckResourceAttr("gravitino_view.this", "id", "ml.cat.sch.upd"),
				),
			},
		},
	})

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.updateBodies) != 1 {
		t.Fatalf("expected exactly one update request, got %d", len(mock.updateBodies))
	}
	const want = `{"updates": [{"@type": "setProperty", "property": "region", "value": "eu"}]}`
	assertJSONEqual(t, want, mock.updateBodies[0])
}

// TestAccViewResource_CommentChangeReplaces asserts that a comment change
// forces a replacement instead of an unsupported in-place comment update.
func TestAccViewResource_CommentChangeReplaces(t *testing.T) {
	mock := newMockView("rep", "one", map[string]string{"key": "value"})
	mock.start(t)

	config := func(comment string) string {
		return `
resource "gravitino_view" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "rep"
  comment  = "` + comment + `"

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
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: viewTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("one"),
				Check:  resource.TestCheckResourceAttr("gravitino_view.this", "comment", "one"),
			},
			{
				Config: config("two"),
				Check:  resource.TestCheckResourceAttr("gravitino_view.this", "comment", "two"),
			},
		},
	})

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.updateBodies) != 0 {
		t.Errorf("a comment change must not send an update request, got %s", mock.updateBodies)
	}
	if mock.creates != 2 {
		t.Errorf("a comment change must recreate the view: got %d creates, want 2", mock.creates)
	}
}
