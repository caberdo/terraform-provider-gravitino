package tag_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func tagTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// TestAccTagAssociationResource drives create, refresh and destroy through
// Terraform Core. The mock records the associateTags body so the test proves
// that destroy really sends tagsToRemove (the API has no DELETE endpoint).
func TestAccTagAssociationResource(t *testing.T) {
	var (
		mu          sync.Mutex
		destroyBody map[string]interface{}
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/version":
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.1","compileDate":"2026-01-01","gitCommit":"abc"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/objects/TABLE/sales.orders/tags":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, removing := body["tagsToRemove"]; removing {
				mu.Lock()
				destroyBody = body
				mu.Unlock()
				fmt.Fprint(w, `{"code":0,"names":[]}`)
				return
			}
			fmt.Fprint(w, `{"code":0,"names":["PII"]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/objects/TABLE/sales.orders/tags":
			fmt.Fprint(w, `{"code":0,"tags":[{"name":"PII","comment":"","properties":{},"audit":{"creator":"gravitino","createTime":"2023-12-08T03:41:25.595Z"},"inherited":false}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: tagTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_tag_association" "this" {
  metalake         = "ml"
  object_type      = "TABLE"
  object_full_name = "sales.orders"
  tags             = ["PII"]
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_tag_association.this", "id", "ml.TABLE.sales.orders"),
					resource.TestCheckResourceAttr("gravitino_tag_association.this", "tags.#", "1"),
					resource.TestCheckTypeSetElemAttr("gravitino_tag_association.this", "tags.*", "PII"),
				),
			},
		},
		CheckDestroy: func(*terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if destroyBody == nil {
				return fmt.Errorf("destroy did not send an associateTags request")
			}
			removed, ok := destroyBody["tagsToRemove"].([]interface{})
			if !ok || len(removed) != 1 || removed[0] != "PII" {
				return fmt.Errorf("destroy body = %v, want tagsToRemove [PII]", destroyBody)
			}
			return nil
		},
	})
}
