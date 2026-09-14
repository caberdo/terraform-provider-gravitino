package topic_test

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

func TestAccTopicResource_CommentUpdateInPlace(t *testing.T) {
	now := time.Now().UTC()
	var mu sync.Mutex
	comment := ""
	var updateTypes []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/topics":
			var req models.TopicCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			comment = req.Comment
			mu.Unlock()
			json.NewEncoder(w).Encode(models.TopicResponse{
				Code:  0,
				Topic: models.Topic{Name: req.Name, Comment: req.Comment, Audit: topicAuditModel(now)},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/topics/upd":
			mu.Lock()
			c := comment
			mu.Unlock()
			json.NewEncoder(w).Encode(models.TopicResponse{
				Code:  0,
				Topic: models.Topic{Name: "upd", Comment: c, Audit: topicAuditModel(now)},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/topics/upd":
			var req models.TopicUpdateRequest
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
			json.NewEncoder(w).Encode(models.TopicResponse{
				Code:  0,
				Topic: models.Topic{Name: "upd", Comment: c, Audit: topicAuditModel(now)},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/topics/upd":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: topicTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_topic" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "upd"
  comment  = "one"
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_topic.this", "comment", "one"),
			},
			{
				Config: `
resource "gravitino_topic" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "upd"
  comment  = "two"
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_topic.this", "comment", "two"),
			},
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if len(updateTypes) == 0 {
		t.Fatal("expected an in-place topic update to be sent")
	}
	for _, typ := range updateTypes {
		if typ != "updateComment" && typ != "setProperty" && typ != "removeProperty" {
			t.Errorf("topic update sent unsupported @type %q", typ)
		}
	}
}

func TestAccTopicResource_NameChangeReplaces(t *testing.T) {
	now := time.Now().UTC()
	var mu sync.Mutex
	putCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/topics":
			var req models.TopicCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.TopicResponse{
				Code:  0,
				Topic: models.Topic{Name: req.Name, Comment: req.Comment, Audit: topicAuditModel(now)},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/metalakes/ml/catalogs/cat/schemas/sch/topics/"):
			name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			json.NewEncoder(w).Encode(models.TopicResponse{
				Code:  0,
				Topic: models.Topic{Name: name, Audit: topicAuditModel(now)},
			})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/metalakes/ml/catalogs/cat/schemas/sch/topics/"):
			mu.Lock()
			putCalled = true
			mu.Unlock()
			http.Error(w, "in-place update not expected", http.StatusBadRequest)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/metalakes/ml/catalogs/cat/schemas/sch/topics/"):
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: topicTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_topic" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "old"
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_topic.this", "name", "old"),
			},
			{
				Config: `
resource "gravitino_topic" "this" {
  metalake = "ml"
  catalog  = "cat"
  schema   = "sch"
  name     = "new"
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_topic.this", "name", "new"),
			},
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if putCalled {
		t.Error("topic name change must force replacement; provider sent an in-place update")
	}
}
