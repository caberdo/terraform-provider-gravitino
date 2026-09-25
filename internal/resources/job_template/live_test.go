package job_template_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLiveAccJobTemplateResource exercises the job template resource against a real
// Gravitino server (1.3.0 in CI). The register endpoint rejects unknown fields, so
// this test fails if the request payload drifts from the API contract.
func TestLiveAccJobTemplateResource(t *testing.T) {
	mlName := acceptance.UniqueName("jtml")
	tplName := acceptance.UniqueName("tpl")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live job template metalake"
}

resource "gravitino_job_template" "this" {
  metalake      = gravitino_metalake.this.name
  name          = %[2]q
  job_type      = "shell"
  comment       = "live shell template"
  executable    = "/bin/echo"
  arguments     = ["one", "two"]
  environments  = { "ENV_A" = "1" }
  custom_fields = { "field" = "value" }
  scripts       = ["/bin/true"]
}
`, mlName, tplName)

	updatedCfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live job template metalake"
}

resource "gravitino_job_template" "this" {
  metalake      = gravitino_metalake.this.name
  name          = %[2]q
  job_type      = "shell"
  comment       = "live shell template updated"
  executable    = "/bin/echo"
  arguments     = ["three"]
  environments  = { "ENV_A" = "1" }
  custom_fields = { "field" = "value" }
  scripts       = ["/bin/true"]
}
`, mlName, tplName)

	renamedCfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live job template metalake"
}

resource "gravitino_job_template" "this" {
  metalake      = gravitino_metalake.this.name
  name          = "%[2]s_renamed"
  job_type      = "shell"
  comment       = "live shell template updated"
  executable    = "/bin/echo"
  arguments     = ["three"]
  environments  = { "ENV_A" = "1" }
  custom_fields = { "field" = "value" }
  scripts       = ["/bin/true"]
}
`, mlName, tplName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_job_template.this", "name", tplName),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "job_type", "shell"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "executable", "/bin/echo"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "arguments.#", "2"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "environments.ENV_A", "1"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "custom_fields.field", "value"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "id", mlName+"."+tplName),
					resource.TestCheckResourceAttrSet("gravitino_job_template.this", "audit.creator"),
				),
			},
			{
				// In-place update: comment + arguments via the updateTemplate variant.
				Config: updatedCfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_job_template.this", "comment", "live shell template updated"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "arguments.#", "1"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "arguments.0", "three"),
				),
			},
			{
				// Rename: the PUT must target the old name and the state must end up
				// with the new name and id.
				Config: renamedCfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_job_template.this", "name", tplName+"_renamed"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "id", mlName+"."+tplName+"_renamed"),
				),
			},
		},
	})
}

// TestLiveAccJobTemplateResource_Spark covers the spark variant, which is a distinct
// object type on the wire and therefore a distinct payload.
func TestLiveAccJobTemplateResource_Spark(t *testing.T) {
	mlName := acceptance.UniqueName("jtspark")
	tplName := acceptance.UniqueName("spark")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live spark template metalake"
}

resource "gravitino_job_template" "this" {
  metalake     = gravitino_metalake.this.name
  name         = %[2]q
  job_type     = "spark"
  executable   = "/opt/spark/bin/spark-submit"
  class_name   = "org.example.Main"
  jars         = ["s3://bucket/app.jar"]
  configs      = { "spark.executor.memory" = "1g" }
  arguments    = ["--input", "s3://bucket/in"]
}
`, mlName, tplName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_job_template.this", "job_type", "spark"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "class_name", "org.example.Main"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "jars.0", "s3://bucket/app.jar"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "configs.spark.executor.memory", "1g"),
				),
			},
		},
	})
}

// TestLiveAccJobTemplateResource_InvalidCombination asserts the shell/spark attribute
// split is rejected before any API call is made.
func TestLiveAccJobTemplateResource_InvalidCombination(t *testing.T) {
	mlName := acceptance.UniqueName("jtinvalid")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live invalid template metalake"
}

resource "gravitino_job_template" "this" {
  metalake   = gravitino_metalake.this.name
  name       = "invalid_tpl"
  job_type   = "shell"
  executable = "/bin/echo"
  jars       = ["s3://bucket/app.jar"]
}
`, mlName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      cfg,
				ExpectError: regexp.MustCompile(`only valid when job_type = "spark"`),
			},
		},
	})
}
