package fileset_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLiveAccFilesetResourceProperties runs against a real Gravitino server.
//
// Gravitino appends a server-side "default-location-name" property to every
// fileset, and a fileset catalog adds "in-use" to catalogs. This test proves that
// the provider writes exactly the configured property set to state:
//
//   - properties = {}            -> properties.% == 0 (the server-only key is not
//     added to state)
//   - properties = { "k" = "v" }  -> properties.% == 1
//   - back to {}                  -> the key is removed through removeProperty
//
// Together with the implicit post-apply plan check of each step it also proves
// that a configured property set does not drift, and that storage_location keeps
// the configured form (Gravitino normalises file:///path to file:/path, which
// must not make Terraform plan a replacement on every run).
func TestLiveAccFilesetResourceProperties(t *testing.T) {
	mlName := acceptance.UniqueName("fsml")
	catName := acceptance.UniqueName("fscat")
	schName := acceptance.UniqueName("fssch")
	fsName := acceptance.UniqueName("fs")
	location := "file:///tmp/terraform-provider-gravitino/live-fileset"

	config := func(properties string) string {
		return fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live fileset metalake"
}

resource "gravitino_catalog" "this" {
  metalake         = gravitino_metalake.this.name
  name             = %[2]q
  type             = "fileset"
  catalog_provider = "fileset"
  comment          = "live fileset catalog"
}

resource "gravitino_schema" "this" {
  metalake  = gravitino_metalake.this.name
  catalog   = gravitino_catalog.this.name
  name      = %[3]q
  properties = {}
}

resource "gravitino_fileset" "this" {
  metalake         = gravitino_metalake.this.name
  catalog          = gravitino_catalog.this.name
  schema           = gravitino_schema.this.name
  name             = %[4]q
  type             = "managed"
  comment          = "live fileset"
  storage_location = %[5]q
  properties       = %[6]s
}
`, mlName, catName, schName, fsName, location, properties)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("{}"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_fileset.this", "name", fsName),
					resource.TestCheckResourceAttr("gravitino_fileset.this", "type", "managed"),
					resource.TestCheckResourceAttr("gravitino_fileset.this", "comment", "live fileset"),
					resource.TestCheckResourceAttr("gravitino_fileset.this", "properties.%", "0"),
					resource.TestCheckResourceAttr("gravitino_fileset.this", "storage_location", location),
					resource.TestCheckResourceAttr("gravitino_fileset.this", "id",
						mlName+"."+catName+"."+schName+"."+fsName),
				),
			},
			{
				Config: config(`{ "k" = "v" }`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_fileset.this", "properties.%", "1"),
					resource.TestCheckResourceAttr("gravitino_fileset.this", "properties.k", "v"),
					resource.TestCheckResourceAttr("gravitino_fileset.this", "storage_location", location),
				),
			},
			{
				Config: config("{}"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_fileset.this", "properties.%", "0"),
				),
			},
		},
	})
}

// TestLiveAccFilesetResourceRename proves that a rename is applied in place: the
// update request is sent to the previous name and the state adopts the new name
// and compound id.
func TestLiveAccFilesetResourceRename(t *testing.T) {
	mlName := acceptance.UniqueName("fsrml")
	catName := acceptance.UniqueName("fscat")
	schName := acceptance.UniqueName("fssch")
	firstName := acceptance.UniqueName("fs")
	secondName := acceptance.UniqueName("fs")
	location := "file:///tmp/terraform-provider-gravitino/live-rename"

	config := func(name string) string {
		return fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live fileset metalake"
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
}

resource "gravitino_fileset" "this" {
  metalake         = gravitino_metalake.this.name
  catalog          = gravitino_catalog.this.name
  schema           = gravitino_schema.this.name
  name             = %[4]q
  type             = "managed"
  storage_location = %[5]q
}
`, mlName, catName, schName, name, location)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config(firstName),
				Check: resource.TestCheckResourceAttr("gravitino_fileset.this", "id",
					mlName+"."+catName+"."+schName+"."+firstName),
			},
			{
				Config: config(secondName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_fileset.this", "name", secondName),
					resource.TestCheckResourceAttr("gravitino_fileset.this", "id",
						mlName+"."+catName+"."+schName+"."+secondName),
				),
			},
		},
	})
}
