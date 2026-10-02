package policy_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func policyTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// policyVersion131Response is what the mock returns after a create on a 1.3.1
// server: the content echoes the two object types added by 1.3.1, so the apply
// result is consistent with the plan.
const policyVersion131Response = `{
  "code": 0,
  "policy": {
    "name": "pol",
    "comment": "",
    "policyType": "custom",
    "enabled": true,
    "content": {
      "supportedObjectTypes": ["VIEW", "FUNCTION"]
    },
    "inherited": null,
    "audit": {"creator": "anonymous", "createTime": "2025-08-04T10:29:23.463Z"}
  }
}`

// TestAccPolicyResource_RejectsViewOn130Server proves the whole chain end to
// end through Terraform Core: provider Configure reads /api/version, records
// 1.3.0 on the client, and the policy create then fails with the version-gate
// diagnostic instead of an opaque API error. The mock serves no policy
// endpoint, so any request other than /api/version would surface a different
// error and fail the assertion.
func TestAccPolicyResource_RejectsViewOn130Server(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.URL.Path == "/api/version" {
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.0","compileDate":"2026-01-01","gitCommit":"abc"}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: policyTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_policy" "this" {
  metalake               = "ml"
  name                   = "pol"
  supported_object_types = ["CATALOG", "VIEW"]
}
`,
				ExpectError: regexp.MustCompile(`require[s]? Gravitino >= 1\.3\.1`),
			},
		},
	})
}

// TestAccPolicyResource_ViewAndFunctionOn131Server proves the accepted case end
// to end: a server that reports 1.3.1 lets VIEW/FUNCTION through create, read
// and destroy.
func TestAccPolicyResource_ViewAndFunctionOn131Server(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/version":
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.1","compileDate":"2026-01-01","gitCommit":"abc"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/policies":
			fmt.Fprint(w, policyVersion131Response)
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/policies/pol":
			fmt.Fprint(w, policyVersion131Response)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/policies/pol":
			fmt.Fprint(w, `{"code":0,"dropped":true}`)
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
resource "gravitino_policy" "this" {
  metalake               = "ml"
  name                   = "pol"
  supported_object_types = ["VIEW", "FUNCTION"]
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_policy.this", "id", "ml.pol"),
					resource.TestCheckResourceAttr("gravitino_policy.this", "supported_object_types.#", "2"),
					resource.TestCheckTypeSetElemAttr("gravitino_policy.this", "supported_object_types.*", "VIEW"),
					resource.TestCheckTypeSetElemAttr("gravitino_policy.this", "supported_object_types.*", "FUNCTION"),
				),
			},
		},
	})
}
