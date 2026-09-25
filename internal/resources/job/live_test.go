package job_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"
	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// setupLiveJobEnvironment creates a metalake and a job template through the API and
// returns both names. They are deliberately *not* managed by Terraform:
//
// Gravitino refuses to delete a template that still has job runs associated with it
// (HTTP 409 InUseException) and keeps a cancelled-but-not-yet-collected run in
// "cancelling" forever when no job executor is running, so managing the template in the
// same configuration would make the destroy of the job resource depend on the job
// executor finishing the cancellation. The template lifecycle itself is covered by the
// job_template live tests.
func setupLiveJobEnvironment(t *testing.T) (metalake, template string) {
	t.Helper()

	// This runs before the TestCase PreCheck, so it needs the same guard: without a live
	// server there is nothing to set up and the test must skip rather than fail.
	uri := strings.TrimRight(os.Getenv("GRAVITINO_URI"), "/")
	if uri == "" {
		t.Skip("GRAVITINO_URI is not set; skipping live acceptance tests")
	}
	c, err := client.New(uri, nil)
	if err != nil {
		t.Fatalf("client.New(%q) error = %v", uri, err)
	}

	ctx := context.Background()
	metalake = acceptance.UniqueName("jobml")
	if _, err := c.CreateMetalake(ctx, &models.MetalakeCreateRequest{
		Name:    metalake,
		Comment: "live job metalake",
	}); err != nil {
		t.Fatalf("CreateMetalake() error = %v", err)
	}

	template = acceptance.UniqueName("tpl")
	if _, err := c.RegisterJobTemplate(ctx, metalake, &models.JobTemplateRegisterRequest{
		JobTemplate: models.JobTemplate{
			Name:       template,
			JobType:    "shell",
			Comment:    "live job template",
			Executable: "/bin/echo",
			Arguments:  []string{"hello"},
		},
	}); err != nil {
		t.Fatalf("RegisterJobTemplate() error = %v", err)
	}

	t.Cleanup(func() {
		// Best effort: jobs and templates may still be alive.
		if _, err := c.DropMetalake(context.Background(), metalake, true); err != nil {
			t.Logf("cleanup: could not drop metalake %q: %v", metalake, err)
		}
	})

	return metalake, template
}

// TestLiveAccJobResource runs a job template against a real Gravitino server. The job
// run endpoint rejects unknown fields and derives the job id server-side, so this test
// fails if the resource sends a request body the API does not accept.
func TestLiveAccJobResource(t *testing.T) {
	mlName, template := setupLiveJobEnvironment(t)

	cfg := fmt.Sprintf(`
resource "gravitino_job" "this" {
  metalake     = %[1]q
  job_template = %[2]q
  job_conf     = { "conf_key" = "conf_value" }
}
`, mlName, template)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_job.this", "job_template", template),
					resource.TestCheckResourceAttrSet("gravitino_job.this", "job_id"),
					resource.TestMatchResourceAttr("gravitino_job.this", "status",
						regexp.MustCompile(`^(queued|started|failed|succeeded|cancelling|canceled)$`)),
					// The composite id is metalake.<server-generated job id>.
					resource.TestMatchResourceAttr("gravitino_job.this", "id",
						regexp.MustCompile(`^`+regexp.QuoteMeta(mlName)+`\.job-`)),
					resource.TestCheckResourceAttrSet("gravitino_job.this", "audit.creator"),
				),
			},
			{
				// A refresh must re-read the job and keep the server-side status.
				RefreshState: true,
				Check: resource.TestMatchResourceAttr("gravitino_job.this", "status",
					regexp.MustCompile(`^(queued|started|failed|succeeded|cancelling|canceled)$`)),
			},
		},
		// No CheckDestroy: destroying a job cancels the run, but Gravitino keeps the
		// job record, so there is nothing to verify beyond the cancel call.
	})
}

// TestLiveAccJobResource_UnknownTemplate asserts a clear API error when the template
// does not exist (the server rejects the run with 404 NoSuchJobTemplateException).
func TestLiveAccJobResource_UnknownTemplate(t *testing.T) {
	mlName := acceptance.UniqueName("jobmiss")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live job metalake"
}

resource "gravitino_job" "this" {
  metalake     = gravitino_metalake.this.name
  job_template = "does_not_exist"
}
`, mlName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      cfg,
				ExpectError: regexp.MustCompile(`Failed running job`),
			},
		},
	})
}
