package policy_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccPolicyAssociationResource drives create, refresh and destroy through
// Terraform Core, including the provider-side supported_object_types check. The
// mock records the associatePolicies body so the test proves that destroy really
// sends policiesToRemove (the API has no DELETE endpoint).
func TestAccPolicyAssociationResource(t *testing.T) {
	var (
		mu          sync.Mutex
		destroyBody map[string]interface{}
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/version":
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.1","compileDate":"2026-01-01","gitCommit":"abc"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/policies/masking":
			fmt.Fprint(w, `{"code":0,"policy":{"name":"masking","comment":"","policyType":"custom","enabled":true,"content":{"supportedObjectTypes":["TABLE"],"customRules":{"rule":"mask"}},"inherited":null,"audit":{"creator":"anonymous","createTime":"2025-08-04T10:29:23.463Z"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/objects/TABLE/sales.orders/policies":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, removing := body["policiesToRemove"]; removing {
				mu.Lock()
				destroyBody = body
				mu.Unlock()
				fmt.Fprint(w, `{"code":0,"names":[]}`)
				return
			}
			fmt.Fprint(w, `{"code":0,"names":["masking"]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/objects/TABLE/sales.orders/policies":
			fmt.Fprint(w, `{"code":0,"policies":[{"name":"masking","comment":"","policyType":"custom","enabled":true,"content":{"supportedObjectTypes":["TABLE"],"customRules":{"rule":"mask"}},"inherited":false,"audit":{"creator":"anonymous","createTime":"2025-08-04T10:29:23.463Z"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: policyTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_policy_association" "this" {
  metalake         = "ml"
  object_type      = "TABLE"
  object_full_name = "sales.orders"
  policies         = ["masking"]
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_policy_association.this", "id", "ml.TABLE.sales.orders"),
					resource.TestCheckResourceAttr("gravitino_policy_association.this", "policies.#", "1"),
					resource.TestCheckTypeSetElemAttr("gravitino_policy_association.this", "policies.*", "masking"),
				),
			},
		},
		CheckDestroy: func(*terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if destroyBody == nil {
				return fmt.Errorf("destroy did not send an associatePolicies request")
			}
			removed, ok := destroyBody["policiesToRemove"].([]interface{})
			if !ok || len(removed) != 1 || removed[0] != "masking" {
				return fmt.Errorf("destroy body = %v, want policiesToRemove [masking]", destroyBody)
			}
			return nil
		},
	})
}
