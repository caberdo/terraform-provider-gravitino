package health_test

import (
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAccHealthDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_health" "h" {}

data "gravitino_liveness" "l" {}

data "gravitino_readiness" "r" {}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_health.h", "status", "up"),
					resource.TestCheckResourceAttr("data.gravitino_liveness.l", "status", "up"),
					resource.TestCheckResourceAttr("data.gravitino_readiness.r", "status", "up"),
				),
			},
		},
	})
}
