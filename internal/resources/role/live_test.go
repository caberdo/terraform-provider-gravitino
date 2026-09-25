package role_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLiveAccRoleResource exercises the role resource against a real Gravitino server.
// It requires the server to run with authorization enabled
// (GRAVITINO_AUTHORIZATION_ENABLE=true); without it every role/owner endpoint answers
// HTTP 405 UnsupportedOperationException.
//
// The assertions on securable_objects and privileges double as a regression test for
// case handling: Gravitino accepts enum values case-insensitively on input but always
// serialises them in lowercase ("metalake", "create_catalog", "allow"). The provider
// must expose them in the canonical uppercase form it also validates, otherwise every
// apply fails with "Provider produced inconsistent result after apply".
func TestLiveAccRoleResource(t *testing.T) {
	mlName := acceptance.UniqueName("roleml")
	catName := acceptance.UniqueName("rolecat")
	roleName := acceptance.UniqueName("role")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live role metalake"
}

resource "gravitino_catalog" "this" {
  metalake         = gravitino_metalake.this.name
  name             = %[2]q
  type             = "fileset"
  catalog_provider = "fileset"
}

resource "gravitino_role" "this" {
  metalake = gravitino_metalake.this.name
  name     = %[3]q

  securable_objects = [
    {
      full_name = gravitino_metalake.this.name
      type      = "METALAKE"
      privileges = [
        { name = "CREATE_CATALOG", condition = "ALLOW" },
      ]
    },
  ]
}
`, mlName, catName, roleName)

	cfgWithPrivilege := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live role metalake"
}

resource "gravitino_catalog" "this" {
  metalake         = gravitino_metalake.this.name
  name             = %[2]q
  type             = "fileset"
  catalog_provider = "fileset"
}

resource "gravitino_role" "this" {
  metalake = gravitino_metalake.this.name
  name     = %[3]q

  securable_objects = [
    {
      full_name = gravitino_metalake.this.name
      type      = "METALAKE"
      privileges = [
        { name = "CREATE_CATALOG", condition = "ALLOW" },
        { name = "USE_CATALOG", condition = "ALLOW" },
      ]
    },
    {
      # Securable object full names are relative to the metalake: the metalake name
      # itself for METALAKE, otherwise the path inside the metalake (e.g. the catalog
      # name). Prefixing the metalake makes Gravitino reject the request with
      # IllegalNamespaceException.
      full_name = "%[2]s"
      type      = "CATALOG"
      privileges = [
        { name = "USE_CATALOG", condition = "ALLOW" },
      ]
    },
  ]
}
`, mlName, catName, roleName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_role.this", "name", roleName),
					resource.TestCheckResourceAttr("gravitino_role.this", "id", mlName+"."+roleName),
					resource.TestCheckResourceAttr("gravitino_role.this", "securable_objects.#", "1"),
					// securable_objects is a set: index order is not stable, so match on
					// the element contents instead of on an index.
					resource.TestCheckTypeSetElemNestedAttrs("gravitino_role.this", "securable_objects.*", map[string]string{
						"type":      "METALAKE",
						"full_name": mlName,
					}),
					resource.TestCheckResourceAttrSet("gravitino_role.this", "audit.creator"),
				),
			},
			{
				// Adding a second securable object goes through the privilege override
				// endpoint and must survive the server's lowercase serialisation.
				Config: cfgWithPrivilege,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_role.this", "securable_objects.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("gravitino_role.this", "securable_objects.*", map[string]string{
						"type":      "METALAKE",
						"full_name": mlName,
					}),
					resource.TestCheckTypeSetElemNestedAttrs("gravitino_role.this", "securable_objects.*", map[string]string{
						"type":      "CATALOG",
						"full_name": catName,
					}),
				),
			},
		},
	})
}
