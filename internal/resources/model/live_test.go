package model_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLiveAccModelResource runs against a real Gravitino (GRAVITINO_URI). It
// covers register, rename/comment/properties updates, the list and get data
// sources, and import.
func TestLiveAccModelResource(t *testing.T) {
	metalake := acceptance.UniqueName("modelml")
	catalog := acceptance.UniqueName("modelcat")
	schema := acceptance.UniqueName("modelsch")
	model := acceptance.UniqueName("model")
	renamed := model + "r"

	config := func(modelName, comment, framework string) string {
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
  comment  = %[5]q
  properties = {
    "framework" = %[6]q
  }
}

data "gravitino_model" "this" {
  metalake = gravitino_metalake.this.name
  catalog  = gravitino_catalog.this.name
  schema   = gravitino_schema.this.name
  name     = gravitino_model.this.name
}

data "gravitino_models" "all" {
  metalake = gravitino_metalake.this.name
  catalog  = gravitino_catalog.this.name
  schema   = gravitino_schema.this.name
  depends_on = [gravitino_model.this]
}
`, metalake, catalog, schema, modelName, comment, framework)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config(model, "live acceptance model", "pytorch"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_model.this", "name", model),
					resource.TestCheckResourceAttr("gravitino_model.this", "id", fmt.Sprintf("%s.%s.%s.%s", metalake, catalog, schema, model)),
					resource.TestCheckResourceAttr("gravitino_model.this", "comment", "live acceptance model"),
					resource.TestCheckResourceAttr("gravitino_model.this", "properties.framework", "pytorch"),
					resource.TestCheckResourceAttrSet("gravitino_model.this", "latest_version"),
					resource.TestCheckResourceAttrSet("gravitino_model.this", "audit.creator"),
					resource.TestCheckResourceAttr("data.gravitino_model.this", "comment", "live acceptance model"),
					resource.TestCheckResourceAttr("data.gravitino_model.this", "properties.framework", "pytorch"),
					resource.TestCheckResourceAttr("data.gravitino_models.all", "models.#", "1"),
					resource.TestCheckResourceAttr("data.gravitino_models.all", "models.0.name", model),
				),
			},
			{
				Config: config(renamed, "live acceptance model (updated)", "tensorflow"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_model.this", "name", renamed),
					resource.TestCheckResourceAttr("gravitino_model.this", "id", fmt.Sprintf("%s.%s.%s.%s", metalake, catalog, schema, renamed)),
					resource.TestCheckResourceAttr("gravitino_model.this", "comment", "live acceptance model (updated)"),
					resource.TestCheckResourceAttr("gravitino_model.this", "properties.framework", "tensorflow"),
					resource.TestCheckResourceAttr("data.gravitino_models.all", "models.0.name", renamed),
				),
			},
			{
				ResourceName:      "gravitino_model.this",
				ImportState:       true,
				ImportStateId:     fmt.Sprintf("%s.%s.%s.%s", metalake, catalog, schema, renamed),
				ImportStateVerify: true,
			},
		},
	})
}
