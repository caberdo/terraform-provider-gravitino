package metalake_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAccMetalakeDataSources(t *testing.T) {
	mlName := acceptance.UniqueName("dsml")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live datasource metalake"
}

data "gravitino_metalake" "by_name" {
  name = gravitino_metalake.this.name
}

data "gravitino_metalakes" "all" {}
`, mlName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_metalake.by_name", "name", mlName),
					resource.TestCheckResourceAttr("data.gravitino_metalake.by_name", "comment", "live datasource metalake"),
				),
			},
		},
	})
}
