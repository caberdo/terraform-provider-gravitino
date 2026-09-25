package user_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLiveAccUserResource exercises the user resource against a real Gravitino server.
// Requires the server to run with authorization enabled
// (GRAVITINO_AUTHORIZATION_ENABLE=true), otherwise /metalakes/{m}/users answers HTTP 405.
func TestLiveAccUserResource(t *testing.T) {
	mlName := acceptance.UniqueName("userml")
	userName := acceptance.UniqueName("user")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live user metalake"
}

resource "gravitino_user" "this" {
  metalake = gravitino_metalake.this.name
  name     = %[2]q
}
`, mlName, userName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_user.this", "name", userName),
					resource.TestCheckResourceAttr("gravitino_user.this", "id", mlName+"."+userName),
					resource.TestCheckResourceAttrSet("gravitino_user.this", "audit.creator"),
				),
			},
			{
				RefreshState: true,
				Check:        resource.TestCheckResourceAttr("gravitino_user.this", "name", userName),
			},
		},
	})
}
