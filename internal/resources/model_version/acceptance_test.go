package model_version_test

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

func modelVersionTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// mockModelVersionServer is a minimal Gravitino model version endpoint. It
// implements the payload shapes of the v1.3.0 spec examples, including the plain
// `{"code": 0}` BaseResponse of linkModelVersion and the `versions`/`infos`
// keys of listModelVersions.
type mockModelVersionServer struct {
	mu sync.Mutex

	version    int32
	uri        string
	uris       map[string]string
	aliases    []string
	comment    string
	properties map[string]string

	gotUpdates []interface{}
}

func (m *mockModelVersionServer) modelVersion() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := map[string]interface{}{
		"version":    m.version,
		"uris":       m.uris,
		"aliases":    m.aliases,
		"properties": m.properties,
		"audit": map[string]interface{}{
			"creator":          "anonymous",
			"createTime":       "2021-01-01T00:00:00Z",
			"lastModifier":     "anonymous",
			"lastModifiedTime": "2021-01-02T00:00:00Z",
		},
	}
	if m.uri != "" {
		entry["uri"] = m.uri
	}
	if m.comment != "" {
		entry["comment"] = m.comment
	}
	return entry
}

func (m *mockModelVersionServer) infoListResponse() []byte {
	body, _ := json.Marshal(map[string]interface{}{"code": 0, "infos": []interface{}{m.modelVersion()}})
	return body
}

func (m *mockModelVersionServer) versionResponse() []byte {
	body, _ := json.Marshal(map[string]interface{}{"code": 0, "modelVersion": m.modelVersion()})
	return body
}

func (m *mockModelVersionServer) link(request map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if uris, ok := request["uris"].(map[string]interface{}); ok && len(uris) > 0 {
		m.uris = map[string]string{}
		for name, uri := range uris {
			m.uris[name], _ = uri.(string)
		}
	} else if uri, ok := request["uri"].(string); ok && uri != "" {
		// Gravitino stores the unnamed uri under the reserved "unknown" name.
		m.uri = uri
		m.uris = map[string]string{"unknown": uri}
	}
	if aliases, ok := request["aliases"].([]interface{}); ok {
		m.aliases = []string{}
		for _, alias := range aliases {
			name, _ := alias.(string)
			m.aliases = append(m.aliases, name)
		}
	}
	if m.aliases == nil {
		m.aliases = []string{}
	}
	if comment, ok := request["comment"].(string); ok {
		m.comment = comment
	}
	if properties, ok := request["properties"].(map[string]interface{}); ok {
		m.properties = map[string]string{}
		for key, value := range properties {
			m.properties[key], _ = value.(string)
		}
	}
	if m.properties == nil {
		m.properties = map[string]string{}
	}
}

func (m *mockModelVersionServer) applyUpdates(updates []interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, update := range updates {
		entry, ok := update.(map[string]interface{})
		if !ok {
			continue
		}
		uriName, _ := entry["uriName"].(string)
		switch entry["@type"] {
		case "updateComment":
			m.comment, _ = entry["newComment"].(string)
		case "setProperty":
			property, _ := entry["property"].(string)
			value, _ := entry["value"].(string)
			m.properties[property] = value
		case "removeProperty":
			property, _ := entry["property"].(string)
			delete(m.properties, property)
		case "updateUri":
			newURI, _ := entry["newUri"].(string)
			if uriName == "" {
				m.uri = newURI
				m.uris["unknown"] = newURI
			} else {
				m.uris[uriName] = newURI
			}
		case "addUri":
			uri, _ := entry["uri"].(string)
			m.uris[uriName] = uri
		case "removeUri":
			delete(m.uris, uriName)
		case "updateAliases":
			removals := map[string]struct{}{}
			if remove, ok := entry["aliasesToRemove"].([]interface{}); ok {
				for _, alias := range remove {
					name, _ := alias.(string)
					removals[name] = struct{}{}
				}
			}
			kept := []string{}
			for _, alias := range m.aliases {
				if _, removed := removals[alias]; !removed {
					kept = append(kept, alias)
				}
			}
			if add, ok := entry["aliasesToAdd"].([]interface{}); ok {
				for _, alias := range add {
					name, _ := alias.(string)
					kept = append(kept, name)
				}
			}
			m.aliases = kept
		}
	}
}

func TestAccModelVersionResource_Lifecycle(t *testing.T) {
	mock := &mockModelVersionServer{uris: map[string]string{}, properties: map[string]string{}}
	collection := "/api/metalakes/probe_ml/catalogs/probe_cat/schemas/probe_sch/models/model1/versions"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == collection:
			var request map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&request)
			mock.link(request)
			// linkModelVersion answers with a BaseResponse.
			_, _ = w.Write([]byte(`{"code": 0}`))
		case r.Method == http.MethodPut && r.URL.Path == collection+"/0":
			var body struct {
				Updates []interface{} `json:"updates"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mock.mu.Lock()
			mock.gotUpdates = body.Updates
			mock.mu.Unlock()
			mock.applyUpdates(body.Updates)
			_, _ = w.Write(mock.versionResponse())
		case r.Method == http.MethodGet && r.URL.Path == collection:
			_, _ = w.Write(mock.infoListResponse())
		case r.Method == http.MethodGet && r.URL.Path == collection+"/0":
			_, _ = w.Write(mock.versionResponse())
		case r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(`{"code": 0, "dropped": true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: modelVersionTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_model_version" "this" {
  metalake = "probe_ml"
  catalog  = "probe_cat"
  schema   = "probe_sch"
  model    = "model1"
  uri      = "hdfs://path/to/model"
  aliases  = ["alias1", "alias2"]
  comment  = "This is a comment"
  properties = {
    "key1" = "value1"
    "key2" = "value2"
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					// The version number is assigned by Gravitino.
					resource.TestCheckResourceAttr("gravitino_model_version.this", "version", "0"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "id", "probe_ml.probe_cat.probe_sch.model1.0"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "uri", "hdfs://path/to/model"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "aliases.#", "2"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "comment", "This is a comment"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "properties.%", "2"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "uris.unknown", "hdfs://path/to/model"),
				),
			},
			{
				Config: `
resource "gravitino_model_version" "this" {
  metalake = "probe_ml"
  catalog  = "probe_cat"
  schema   = "probe_sch"
  model    = "model1"
  uri      = "s3://path/to/model"
  aliases  = ["alias2", "alias3"]
  comment  = "This is a new comment"
  properties = {
    "key1" = "value1"
    "key2" = "value2"
    "key3" = "value3"
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					checkModelVersionUpdates(t, mock, []interface{}{
						map[string]interface{}{"@type": "updateComment", "newComment": "This is a new comment"},
						map[string]interface{}{"@type": "setProperty", "property": "key3", "value": "value3"},
						map[string]interface{}{"@type": "updateAliases", "aliasesToAdd": []interface{}{"alias3"}, "aliasesToRemove": []interface{}{"alias1"}},
						map[string]interface{}{"@type": "updateUri", "newUri": "s3://path/to/model"},
					}),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "uri", "s3://path/to/model"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "comment", "This is a new comment"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "aliases.#", "2"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "properties.%", "3"),
				),
			},
			{
				ResourceName:      "gravitino_model_version.this",
				ImportState:       true,
				ImportStateId:     "probe_ml.probe_cat.probe_sch.model1.0",
				ImportStateVerify: true,
			},
		},
	})
}

func checkModelVersionUpdates(t *testing.T, mock *mockModelVersionServer, want []interface{}) resource.TestCheckFunc {
	return func(*terraform.State) error {
		mock.mu.Lock()
		defer mock.mu.Unlock()
		if !reflect.DeepEqual(mock.gotUpdates, want) {
			t.Errorf("unexpected update payload:\ngot  %#v\nwant %#v", mock.gotUpdates, want)
		}
		return nil
	}
}
