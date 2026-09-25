package catalog_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAccCatalogResource(t *testing.T) {
	mlName := acceptance.UniqueName("catml")
	catName := acceptance.UniqueName("hive")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live catalog metalake"
}

resource "gravitino_catalog" "this" {
  metalake         = gravitino_metalake.this.name
  name             = %[2]q
  type             = "relational"
  catalog_provider = "hive"
  comment          = "live hive catalog"
  properties = {
    "metastore.uris" = "thrift://live-dummy:9083"
  }
}

data "gravitino_principal" "me" {}
`, mlName, catName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_catalog.this", "name", catName),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "type", "relational"),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "catalog_provider", "hive"),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "comment", "live hive catalog"),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "id", mlName+"."+catName),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "properties.metastore.uris", "thrift://live-dummy:9083"),
					resource.TestCheckResourceAttrPair(
						"gravitino_catalog.this", "audit.creator",
						"data.gravitino_principal.me", "name"),
					resource.TestCheckResourceAttrSet("gravitino_catalog.this", "audit.create_time"),
				),
			},
		},
	})
}

// TestLiveAccCatalogResourceUpdateProperties exercises the parts of the catalog
// resource that the Hive-based live test cannot reach on a server without a
// Hive metastore: a fileset-provider catalog with properties, an in-place
// comment/property update, a rename and an import. The server adds its own
// "in-use" property, which must never leak into state.
func TestLiveAccCatalogResourceUpdateProperties(t *testing.T) {
	mlName := acceptance.UniqueName("catuml")
	catName := acceptance.UniqueName("fileset")
	newCatName := catName + "_renamed"

	cfgCreate := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live catalog update metalake"
}

resource "gravitino_catalog" "this" {
  metalake         = gravitino_metalake.this.name
  name             = %[2]q
  type             = "fileset"
  catalog_provider = "fileset"
  comment          = "live fileset catalog"
  properties = {
    env = "dev"
  }
}
`, mlName, catName)

	cfgUpdate := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live catalog update metalake"
}

resource "gravitino_catalog" "this" {
  metalake         = gravitino_metalake.this.name
  name             = %[2]q
  type             = "fileset"
  catalog_provider = "fileset"
  comment          = "live fileset catalog (updated)"
  properties = {
    env   = "prod"
    extra = "value"
  }
}
`, mlName, newCatName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfgCreate,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_catalog.this", "id", mlName+"."+catName),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "comment", "live fileset catalog"),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "properties.%", "1"),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "properties.env", "dev"),
					resource.TestCheckNoResourceAttr("gravitino_catalog.this", "properties.in-use"),
				),
			},
			{
				Config: cfgUpdate,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_catalog.this", "id", mlName+"."+newCatName),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "name", newCatName),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "comment", "live fileset catalog (updated)"),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "properties.%", "2"),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "properties.env", "prod"),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "properties.extra", "value"),
					resource.TestCheckNoResourceAttr("gravitino_catalog.this", "properties.in-use"),
				),
			},
			{
				ResourceName:      "gravitino_catalog.this",
				ImportState:       true,
				ImportStateId:     mlName + "." + newCatName,
				ImportStateVerify: true,
			},
		},
	})
}
