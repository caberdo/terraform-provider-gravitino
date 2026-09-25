package group_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLiveAccGroupResource exercises the group resource against a real Gravitino
// server. Requires the server to run with authorization enabled
// (GRAVITINO_AUTHORIZATION_ENABLE=true), otherwise /metalakes/{m}/groups answers HTTP 405.
func TestLiveAccGroupResource(t *testing.T) {
	mlName := acceptance.UniqueName("grpml")
	groupName := acceptance.UniqueName("grp")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live group metalake"
}

resource "gravitino_group" "this" {
  metalake = gravitino_metalake.this.name
  name     = %[2]q
}
`, mlName, groupName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_group.this", "name", groupName),
					resource.TestCheckResourceAttr("gravitino_group.this", "id", mlName+"."+groupName),
					resource.TestCheckResourceAttrSet("gravitino_group.this", "audit.creator"),
				),
			},
			{
				RefreshState: true,
				Check:        resource.TestCheckResourceAttr("gravitino_group.this", "name", groupName),
			},
		},
	})
}
