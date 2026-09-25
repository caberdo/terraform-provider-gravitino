package model_version_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestLiveAccModelVersionResource runs against a real Gravitino (GRAVITINO_URI).
// It covers linking a version (the version number is assigned by the server),
// comment/property/alias/uri updates, the list and get data sources, and import.
func TestLiveAccModelVersionResource(t *testing.T) {
	metalake := acceptance.UniqueName("mvml")
	catalog := acceptance.UniqueName("mvcat")
	schema := acceptance.UniqueName("mvsch")
	model := acceptance.UniqueName("mvmodel")

	config := func(uri, comment, aliasBeta string, withProperty string) string {
		properties := ""
		if withProperty != "" {
			properties = fmt.Sprintf(`
  properties = {
    %[1]q = "value1"
  }
`, withProperty)
		}
		return fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live acceptance metalake"
}

resource "gravitino_catalog" "this" {
  metalake = gravitino_metalake.this.name
  name     = %[2]q
  type     = "model"
}

resource "gravitino_schema" "this" {
  metalake = gravitino_metalake.this.name
  catalog  = gravitino_catalog.this.name
  name     = %[3]q
}

resource "gravitino_model" "this" {
  metalake = gravitino_metalake.this.name
  catalog  = gravitino_catalog.this.name
  schema   = gravitino_schema.this.name
  name     = %[4]q
}

resource "gravitino_model_version" "this" {
  metalake = gravitino_metalake.this.name
  catalog  = gravitino_catalog.this.name
  schema   = gravitino_schema.this.name
  model    = gravitino_model.this.name
  uri      = %[5]q
  aliases  = ["live-prod", %[6]q]
  comment  = %[7]q
%[8]s
}

data "gravitino_model_versions" "all" {
  metalake = gravitino_metalake.this.name
  catalog  = gravitino_catalog.this.name
  schema   = gravitino_schema.this.name
  model    = gravitino_model.this.name
  depends_on = [gravitino_model_version.this]
}

data "gravitino_model_version" "by_alias" {
  metalake = gravitino_metalake.this.name
  catalog  = gravitino_catalog.this.name
  schema   = gravitino_schema.this.name
  model    = gravitino_model.this.name
  alias    = "live-prod"
  # The alias only exists once the version is linked.
  depends_on = [gravitino_model_version.this]
}
`, metalake, catalog, schema, model, uri, aliasBeta, comment, properties)
	}

	importID := func(state *terraform.State) (string, error) {
		res, ok := state.RootModule().Resources["gravitino_model_version.this"]
		if !ok {
			return "", fmt.Errorf("gravitino_model_version.this not found in state")
		}
		version := res.Primary.Attributes["version"]
		if version == "" {
			return "", fmt.Errorf("version is not known yet")
		}
		return fmt.Sprintf("%s.%s.%s.%s.%s", metalake, catalog, schema, model, version), nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("s3://bucket/model/v1", "live acceptance version", "live-beta", "accuracy"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("gravitino_model_version.this", "version"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "uri", "s3://bucket/model/v1"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "comment", "live acceptance version"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "aliases.#", "2"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "properties.accuracy", "value1"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "uris.unknown", "s3://bucket/model/v1"),
					resource.TestCheckResourceAttrSet("gravitino_model_version.this", "id"),
					resource.TestCheckResourceAttr("data.gravitino_model_versions.all", "versions.#", "1"),
					resource.TestCheckResourceAttrPair(
						"data.gravitino_model_version.by_alias", "version",
						"gravitino_model_version.this", "version"),
				),
			},
			{
				Config: config("s3://bucket/model/v2", "live acceptance version (updated)", "live-staging", "framework"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_model_version.this", "uri", "s3://bucket/model/v2"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "comment", "live acceptance version (updated)"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "aliases.#", "2"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "properties.%", "1"),
					resource.TestCheckResourceAttr("gravitino_model_version.this", "properties.framework", "value1"),
					// updateUri also moves the unnamed uri in the uris map.
					resource.TestCheckResourceAttr("gravitino_model_version.this", "uris.unknown", "s3://bucket/model/v2"),
					resource.TestCheckResourceAttr("data.gravitino_model_versions.all", "versions.#", "1"),
				),
			},
			{
				ResourceName:      "gravitino_model_version.this",
				ImportState:       true,
				ImportStateIdFunc: importID,
				ImportStateVerify: true,
			},
		},
	})
}
