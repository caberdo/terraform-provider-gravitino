package policy_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLiveAccPolicyResource exercises the custom policy resource against a real
// Gravitino server.
//
// The assertions on supported_object_types pin the case handling: Gravitino accepts any
// casing on input but always serialises the values in lower case, so the provider has to
// canonicalise what it reads back, otherwise every apply fails with "Provider produced
// inconsistent result after apply".
func TestLiveAccPolicyResource(t *testing.T) {
	mlName := acceptance.UniqueName("polml")
	polName := acceptance.UniqueName("pol")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live policy metalake"
}

resource "gravitino_policy" "this" {
  metalake    = gravitino_metalake.this.name
  name        = %[2]q
  comment     = "live policy"
  policy_type = "custom"
  enabled     = true

  supported_object_types = ["SCHEMA", "TABLE"]
  custom_rules = {
    "effect"   = "allow"
    "subjects" = "analytics-team"
  }
}
`, mlName, polName)

	updatedCfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live policy metalake"
}

resource "gravitino_policy" "this" {
  metalake    = gravitino_metalake.this.name
  name        = %[2]q
  comment     = "live policy updated"
  policy_type = "custom"
  enabled     = false

  supported_object_types = ["SCHEMA"]
  custom_rules = {
    "effect"   = "allow"
    "subjects" = "analytics-team,reporting-users"
  }
}
`, mlName, polName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_policy.this", "name", polName),
					resource.TestCheckResourceAttr("gravitino_policy.this", "policy_type", "custom"),
					resource.TestCheckResourceAttr("gravitino_policy.this", "enabled", "true"),
					resource.TestCheckResourceAttr("gravitino_policy.this", "supported_object_types.#", "2"),
					resource.TestCheckTypeSetElemAttr("gravitino_policy.this", "supported_object_types.*", "SCHEMA"),
					resource.TestCheckTypeSetElemAttr("gravitino_policy.this", "supported_object_types.*", "TABLE"),
					resource.TestCheckResourceAttr("gravitino_policy.this", "custom_rules.effect", "allow"),
					resource.TestCheckResourceAttr("gravitino_policy.this", "id", mlName+"."+polName),
					resource.TestCheckResourceAttrSet("gravitino_policy.this", "audit.creator"),
				),
			},
			{
				Config: updatedCfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_policy.this", "comment", "live policy updated"),
					resource.TestCheckResourceAttr("gravitino_policy.this", "enabled", "false"),
					resource.TestCheckResourceAttr("gravitino_policy.this", "supported_object_types.#", "1"),
					resource.TestCheckTypeSetElemAttr("gravitino_policy.this", "supported_object_types.*", "SCHEMA"),
				),
			},
		},
	})
}
