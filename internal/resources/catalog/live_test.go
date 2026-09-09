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
