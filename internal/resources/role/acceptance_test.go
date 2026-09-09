package role_test

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

func roleTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func TestAccRoleResource_CreateWithLowercaseServerValues(t *testing.T) {
	now := time.Now().UTC()
	audit := &models.Audit{Creator: "admin", CreateTime: &now, LastModifier: "admin", LastModifiedTime: &now}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/roles":
			var req models.RoleCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.RoleResponse{
				Code: 0,
				Role: models.RoleDetail{
					Name:       req.Name,
					Properties: req.Properties,
					SecurableObjects: []models.SecurableObject{
						{
							FullName: "olympus",
							Type:     "metalake",
							Privileges: []models.Privilege{
								{Name: "create_catalog", Condition: "allow"},
							},
						},
					},
					Audit: audit,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/roles/metalake_reader":
			json.NewEncoder(w).Encode(models.RoleResponse{
				Code: 0,
				Role: models.RoleDetail{
					Name:       "metalake_reader",
					Properties: map[string]string{},
					SecurableObjects: []models.SecurableObject{
						{
							FullName: "olympus",
							Type:     "metalake",
							Privileges: []models.Privilege{
								{Name: "create_catalog", Condition: "allow"},
							},
						},
					},
					Audit: audit,
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/ml/roles/metalake_reader":
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: roleTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_role" "reader" {
  metalake = "ml"
  name     = "metalake_reader"

  securable_objects = [{
    full_name = "olympus"
    type      = "METALAKE"
    privileges = [{
      name      = "CREATE_CATALOG"
      condition = "ALLOW"
    }]
  }]
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_role.reader", "name", "metalake_reader"),
					resource.TestCheckResourceAttr("gravitino_role.reader", "securable_objects.0.type", "METALAKE"),
					resource.TestCheckResourceAttr("gravitino_role.reader", "securable_objects.0.privileges.0.name", "CREATE_CATALOG"),
					resource.TestCheckResourceAttr("gravitino_role.reader", "securable_objects.0.privileges.0.condition", "ALLOW"),
				),
			},
		},
	})
}
