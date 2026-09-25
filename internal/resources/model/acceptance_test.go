package model_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func modelTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// mockModelServer is a minimal Gravitino model endpoint. It answers with the
// payload shapes of the v1.3.0 spec examples and applies the update requests the
// provider sends, so a Terraform apply can be round tripped through the real
// plugin protocol.
type mockModelServer struct {
	mu sync.Mutex

	name          string
	comment       string
	properties    map[string]string
	latestVersion int

	gotUpdates []interface{}
}

func (m *mockModelServer) response() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()

	payload := map[string]interface{}{
		"code": 0,
		"model": map[string]interface{}{
			"name":          m.name,
			"latestVersion": m.latestVersion,
			"properties":    m.properties,
			"audit": map[string]interface{}{
				"creator":          "anonymous",
				"createTime":       "2021-01-01T00:00:00Z",
				"lastModifier":     "anonymous",
				"lastModifiedTime": "2021-01-02T00:00:00Z",
			},
		},
	}
	if m.comment != "" {
		payload["model"].(map[string]interface{})["comment"] = m.comment
	}
	body, _ := json.Marshal(payload)
	return body
}

func (m *mockModelServer) applyUpdates(updates []interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, update := range updates {
		entry, ok := update.(map[string]interface{})
		if !ok {
			continue
		}
		switch entry["@type"] {
		case "rename":
			m.name, _ = entry["newName"].(string)
		case "updateComment":
			m.comment, _ = entry["newComment"].(string)
		case "setProperty":
			property, _ := entry["property"].(string)
			value, _ := entry["value"].(string)
			if m.properties == nil {
				m.properties = map[string]string{}
			}
			m.properties[property] = value
		case "removeProperty":
			property, _ := entry["property"].(string)
			delete(m.properties, property)
		}
	}
}

func TestAccModelResource_Lifecycle(t *testing.T) {
	mock := &mockModelServer{properties: map[string]string{}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		collection := "/api/metalakes/probe_ml/catalogs/probe_cat/schemas/probe_sch/models"

		switch {
		case r.Method == http.MethodPost && r.URL.Path == collection:
			var request struct {
				Name       string            `json:"name"`
				Comment    string            `json:"comment"`
				Properties map[string]string `json:"properties"`
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			mock.mu.Lock()
			mock.name, mock.comment, mock.properties = request.Name, request.Comment, request.Properties
			mock.mu.Unlock()
			_, _ = w.Write(mock.response())
		case r.Method == http.MethodGet && r.URL.Path == collection+"/"+mock.currentName():
			_, _ = w.Write(mock.response())
		case r.Method == http.MethodPut && r.URL.Path == collection+"/"+mock.currentName():
			var body struct {
				Updates []interface{} `json:"updates"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mock.mu.Lock()
			mock.gotUpdates = body.Updates
			mock.mu.Unlock()
			mock.applyUpdates(body.Updates)
			_, _ = w.Write(mock.response())
		case r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(`{"code": 0, "dropped": true}`))
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
  metalake   = "probe_ml"
  catalog    = "probe_cat"
  schema     = "probe_sch"
  name       = "model1"
  comment    = "This is a comment"
  properties = {
    "key1" = "value1"
    "key2" = "value2"
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_model.this", "name", "model1"),
					resource.TestCheckResourceAttr("gravitino_model.this", "id", "probe_ml.probe_cat.probe_sch.model1"),
					resource.TestCheckResourceAttr("gravitino_model.this", "comment", "This is a comment"),
					// latest_version is computed: Gravitino reports it.
					resource.TestCheckResourceAttr("gravitino_model.this", "latest_version", "0"),
					resource.TestCheckResourceAttr("gravitino_model.this", "properties.%", "2"),
					resource.TestCheckResourceAttr("gravitino_model.this", "audit.creator", "anonymous"),
				),
			},
			{
				Config: `
resource "gravitino_model" "this" {
  metalake   = "probe_ml"
  catalog    = "probe_cat"
  schema     = "probe_sch"
  name       = "my_model_new"
  comment    = "This is a new comment"
  properties = {
    "key2" = "value2"
    "key3" = "value3"
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					checkModelUpdates(t, mock, []interface{}{
						map[string]interface{}{"@type": "rename", "newName": "my_model_new"},
						map[string]interface{}{"@type": "updateComment", "newComment": "This is a new comment"},
						map[string]interface{}{"@type": "removeProperty", "property": "key1"},
						map[string]interface{}{"@type": "setProperty", "property": "key3", "value": "value3"},
					}),
					resource.TestCheckResourceAttr("gravitino_model.this", "name", "my_model_new"),
					resource.TestCheckResourceAttr("gravitino_model.this", "id", "probe_ml.probe_cat.probe_sch.my_model_new"),
					resource.TestCheckResourceAttr("gravitino_model.this", "comment", "This is a new comment"),
					resource.TestCheckResourceAttr("gravitino_model.this", "properties.%", "2"),
				),
			},
			{
				ResourceName:      "gravitino_model.this",
				ImportState:       true,
				ImportStateId:     "probe_ml.probe_cat.probe_sch.my_model_new",
				ImportStateVerify: true,
			},
		},
	})
}

func (m *mockModelServer) currentName() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.name
}

func checkModelUpdates(t *testing.T, mock *mockModelServer, want []interface{}) resource.TestCheckFunc {
	return func(*terraform.State) error {
		mock.mu.Lock()
		defer mock.mu.Unlock()
		if !reflect.DeepEqual(mock.gotUpdates, want) {
			t.Errorf("unexpected update payload:\ngot  %#v\nwant %#v", mock.gotUpdates, want)
		}
		return nil
	}
}
