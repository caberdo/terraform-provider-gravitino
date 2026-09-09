package authentication_test

import (
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAccPrincipalDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_principal" "me" {}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_principal.me", "name", "anonymous"),
				),
			},
		},
	})
}
