package metalake_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAccMetalakeResource(t *testing.T) {
	mlName := acceptance.UniqueName("metalive")
	renamedName := mlName + "_renamed"

	cfgCreate := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name       = %[1]q
  comment    = "live acceptance metalake"
  properties = { "env" = "dev" }
}

data "gravitino_principal" "me" {}
`, mlName)

	// The second step also renames the metalake in place: the id is the name, so
	// the provider must plan the id as unknown (ModifyPlan) or Terraform rejects
	// the apply with "Provider produced inconsistent result after apply".
	cfgUpdate := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name       = %[1]q
  comment    = "live acceptance metalake (updated)"
  properties = { "env" = "prod" }
}

data "gravitino_principal" "me" {}
`, renamedName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfgCreate,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_metalake.this", "name", mlName),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "comment", "live acceptance metalake"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.%", "1"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.env", "dev"),
					resource.TestCheckResourceAttrPair(
						"gravitino_metalake.this", "audit.creator",
						"data.gravitino_principal.me", "name"),
					resource.TestCheckResourceAttrSet("gravitino_metalake.this", "audit.create_time"),
				),
			},
			{
				Config: cfgUpdate,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_metalake.this", "name", renamedName),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "id", renamedName),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "comment", "live acceptance metalake (updated)"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.%", "1"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.env", "prod"),
				),
			},
			{
				ResourceName:      "gravitino_metalake.this",
				ImportState:       true,
				ImportStateId:     renamedName,
				ImportStateVerify: true,
			},
		},
	})
}
