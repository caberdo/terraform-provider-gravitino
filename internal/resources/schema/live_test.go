package schema_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLiveAccSchemaResourceProperties runs against a real Gravitino server and
// proves the update policy of a schema:
//
//   - properties = {}            -> properties.% == 0
//   - properties = { "k" = "v" }  -> an in-place alterSchema with setProperty
//   - back to {}                  -> an in-place alterSchema with removeProperty
//
// The implicit post-apply plan check of every step proves that the property set
// in state matches the configuration (no server-side key leaks into state) and
// that the refreshed audit values do not make Terraform plan a change.
func TestLiveAccSchemaResourceProperties(t *testing.T) {
	mlName := acceptance.UniqueName("schml")
	catName := acceptance.UniqueName("schcat")
	schName := acceptance.UniqueName("sch")

	config := func(properties string) string {
		return fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live schema metalake"
}

resource "gravitino_catalog" "this" {
  metalake         = gravitino_metalake.this.name
  name             = %[2]q
  type             = "fileset"
  catalog_provider = "fileset"
}

resource "gravitino_schema" "this" {
  metalake   = gravitino_metalake.this.name
  catalog    = gravitino_catalog.this.name
  name       = %[3]q
  comment    = "live schema"
  properties = %[4]s
}
`, mlName, catName, schName, properties)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("{}"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_schema.this", "name", schName),
					resource.TestCheckResourceAttr("gravitino_schema.this", "comment", "live schema"),
					resource.TestCheckResourceAttr("gravitino_schema.this", "properties.%", "0"),
					resource.TestCheckResourceAttr("gravitino_schema.this", "id",
						mlName+"."+catName+"."+schName),
					resource.TestCheckResourceAttrSet("gravitino_schema.this", "audit.create_time"),
				),
			},
			{
				Config: config(`{ "k" = "v" }`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_schema.this", "properties.%", "1"),
					resource.TestCheckResourceAttr("gravitino_schema.this", "properties.k", "v"),
					resource.TestCheckResourceAttrSet("gravitino_schema.this", "audit.last_modified_time"),
				),
			},
			{
				Config: config("{}"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_schema.this", "properties.%", "0"),
				),
			},
		},
	})
}

// TestLiveAccSchemaResourceCommentReplaces proves that changing the comment
// destroys and recreates the schema: the Gravitino schema update API only
// supports property updates.
func TestLiveAccSchemaResourceCommentReplaces(t *testing.T) {
	mlName := acceptance.UniqueName("replml")
	catName := acceptance.UniqueName("replcat")
	schName := acceptance.UniqueName("sch")

	config := func(comment string) string {
		return fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live schema metalake"
}

resource "gravitino_catalog" "this" {
  metalake         = gravitino_metalake.this.name
  name             = %[2]q
  type             = "fileset"
  catalog_provider = "fileset"
}

resource "gravitino_schema" "this" {
  metalake = gravitino_metalake.this.name
  catalog  = gravitino_catalog.this.name
  name     = %[3]q
  comment  = %[4]q
}
`, mlName, catName, schName, comment)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("first comment"),
				Check:  resource.TestCheckResourceAttr("gravitino_schema.this", "comment", "first comment"),
			},
			{
				Config: config("second comment"),
				Check:  resource.TestCheckResourceAttr("gravitino_schema.this", "comment", "second comment"),
			},
		},
	})
}
