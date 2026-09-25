package job_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func jobTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// TestAccJobResource_Create runs a create through Terraform Core against a mock
// that answers the spec's JobResponse. It fails hard ("Provider produced
// inconsistent result after apply" / "Provider returned invalid result object
// after apply") if any planned attribute is not reproduced exactly in state.
//
// The mock is stateful about the job run: POST /jobs/runs stores it and
// GET /jobs/runs/{jobId} returns it, so the post-apply refresh and plan see the
// same job.
func TestAccJobResource_Create(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/jobs/runs":
			_, _ = w.Write([]byte(jobResponseExample))
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/ml/jobs/runs/job-12345":
			// Refresh: a terminal job is returned unchanged and Delete must not
			// try to cancel it.
			_, _ = w.Write([]byte(jobResponseExample))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: jobTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_job" "this" {
  metalake     = "ml"
  job_template = "test_run_get"
  job_conf     = { arg1 = "value1", arg2 = "value2" }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_job.this", "id", "ml.job-12345"),
					resource.TestCheckResourceAttr("gravitino_job.this", "job_id", "job-12345"),
					resource.TestCheckResourceAttr("gravitino_job.this", "status", "succeeded"),
					resource.TestCheckResourceAttr("gravitino_job.this", "job_template", "test_run_get"),
					resource.TestCheckResourceAttr("gravitino_job.this", "metalake", "ml"),
					resource.TestCheckResourceAttr("gravitino_job.this", "job_conf.arg1", "value1"),
					resource.TestCheckResourceAttr("gravitino_job.this", "job_conf.arg2", "value2"),
					resource.TestCheckResourceAttr("gravitino_job.this", "audit.creator", "anonymous"),
					resource.TestCheckResourceAttr("gravitino_job.this", "audit.create_time", "2025-08-12T02:14:28Z"),
				),
			},
		},
	})
}
