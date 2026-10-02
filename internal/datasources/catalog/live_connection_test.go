package catalog_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"
	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLiveAccCatalogConnectionTestProposedConfig exercises the proposed-config
// variant against a real server: a fileset catalog whose catalog-level
// `location` can be probed succeeds, while a Hive catalog pointing at a dead
// metastore reports the server's ConnectionFailedException as success = false.
// Both endpoints exist since Gravitino 1.3.0.
func TestLiveAccCatalogConnectionTestProposedConfig(t *testing.T) {
	mlName := acceptance.UniqueName("ctml")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live connection test metalake"
}

data "gravitino_catalog_connection_test" "fileset" {
  metalake         = gravitino_metalake.this.name
  name             = "probe_fileset_catalog"
  type             = "fileset"
  catalog_provider = "fileset"
  properties = {
    location = "file:///tmp/gravitino-live-connection-test"
  }
}

data "gravitino_catalog_connection_test" "hive" {
  metalake         = gravitino_metalake.this.name
  name             = "probe_hive_catalog"
  type             = "relational"
  catalog_provider = "hive"
  properties = {
    "metastore.uris" = "thrift://127.0.0.1:9083"
  }
}
`, mlName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_catalog_connection_test.fileset", "success", "true"),
					resource.TestCheckResourceAttr("data.gravitino_catalog_connection_test.fileset", "message", ""),
					resource.TestCheckResourceAttr("data.gravitino_catalog_connection_test.hive", "success", "false"),
					resource.TestCheckResourceAttrSet("data.gravitino_catalog_connection_test.hive", "message"),
				),
			},
		},
	})
}

// TestLiveAccCatalogConnectionTestExisting exercises the existing-catalog
// variant: it creates a fileset catalog (with a probed `location`) through the
// provider and tests its stored configuration. The endpoint was added in
// Gravitino 1.3.1, so the test skips against the 1.3.0 server the repository
// pins in docker-compose.yml.
func TestLiveAccCatalogConnectionTestExisting(t *testing.T) {
	requireLiveServerAtLeast(t, 1, 3, 1)

	mlName := acceptance.UniqueName("cteml")
	catName := acceptance.UniqueName("probe_fileset")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live existing connection test metalake"
}

resource "gravitino_catalog" "this" {
  metalake         = gravitino_metalake.this.name
  name             = %[2]q
  type             = "fileset"
  catalog_provider = "fileset"
  properties = {
    location = "file:///tmp/gravitino-live-existing-connection"
  }
}

data "gravitino_catalog_connection_test" "existing" {
  metalake = gravitino_metalake.this.name
  catalog  = gravitino_catalog.this.name
}
`, mlName, catName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_catalog_connection_test.existing", "success", "true"),
					resource.TestCheckResourceAttr("data.gravitino_catalog_connection_test.existing", "message", ""),
				),
			},
		},
	})
}

// requireLiveServerAtLeast skips the test when the live server is older than
// major.minor.patch. It reads the version itself because acceptance.LivePreCheck
// only asserts GRAVITINO_EXPECT_VERSION, which is not set for a manually kept
// server.
func requireLiveServerAtLeast(t *testing.T, major, minor, patch int) {
	t.Helper()

	uri := strings.TrimRight(os.Getenv("GRAVITINO_URI"), "/")
	if uri == "" {
		// acceptance.LivePreCheck(t) skips the test as well.
		return
	}

	c, err := client.New(uri, nil)
	if err != nil {
		t.Fatalf("invalid GRAVITINO_URI %q: %v", uri, err)
	}

	version, err := c.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("server at %s did not answer /api/version: %v", uri, err)
	}

	reported := version.Version.Version
	if models.ServerVersionAtLeast(reported, major, minor, patch) {
		return
	}

	t.Skipf("server at %s reports version %s, which predates %d.%d.%d", uri, reported, major, minor, patch)
}
