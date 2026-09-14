package topic_test

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

func topicTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func TestAccTopicResource_CreateWithoutComment(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/topics":
			var req models.TopicCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.TopicResponse{
				Code: 0,
				Topic: models.Topic{
					Name:    req.Name,
					Comment: req.Comment,
					Audit:   topicAuditModel(now),
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/topics/mytopic":
			// Real Gravitino omits/returns an empty comment when none was set.
			json.NewEncoder(w).Encode(models.TopicResponse{
				Code: 0,
				Topic: models.Topic{
					Name:    "mytopic",
					Comment: "",
					Audit:   topicAuditModel(now),
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/topics/mytopic":
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
  name     = "mytopic"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_topic.this", "name", "mytopic"),
					resource.TestCheckResourceAttr("gravitino_topic.this", "id", "ml.cat.sch.mytopic"),
					resource.TestCheckNoResourceAttr("gravitino_topic.this", "comment"),
				),
			},
		},
	})
}

func TestAccTopicResource_CreateWithComment(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/topics":
			var req models.TopicCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.TopicResponse{
				Code: 0,
				Topic: models.Topic{
					Name:    req.Name,
					Comment: req.Comment,
					Audit:   topicAuditModel(now),
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/topics/with_comment":
			json.NewEncoder(w).Encode(models.TopicResponse{
				Code: 0,
				Topic: models.Topic{
					Name:    "with_comment",
					Comment: "a topic comment",
					Audit:   topicAuditModel(now),
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/catalogs/cat/schemas/sch/topics/with_comment":
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
  name     = "with_comment"
  comment  = "a topic comment"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_topic.this", "name", "with_comment"),
					resource.TestCheckResourceAttr("gravitino_topic.this", "comment", "a topic comment"),
				),
			},
		},
	})
}

func topicAuditModel(t time.Time) *models.Audit {
	return &models.Audit{
		Creator:          "admin",
		CreateTime:       &t,
		LastModifier:     "admin",
		LastModifiedTime: &t,
	}
}