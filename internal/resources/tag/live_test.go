package tag_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAccTagResource(t *testing.T) {
	mlName := acceptance.UniqueName("tagml")
	tagName := acceptance.UniqueName("tag")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live tag metalake"
}

resource "gravitino_tag" "this" {
  metalake  = gravitino_metalake.this.name
  name      = %[2]q
  comment   = "live acceptance tag"
  properties = {}
}

data "gravitino_principal" "me" {}
`, mlName, tagName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_tag.this", "name", tagName),
					resource.TestCheckResourceAttr("gravitino_tag.this", "comment", "live acceptance tag"),
					resource.TestCheckResourceAttrPair(
						"gravitino_tag.this", "audit.creator",
						"data.gravitino_principal.me", "name"),
					resource.TestCheckResourceAttrSet("gravitino_tag.this", "audit.create_time"),
				),
			},
		},
	})
}
