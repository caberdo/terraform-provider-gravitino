package iceberg_rest_service_test

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
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestLiveAccIcebergRESTServiceDataSource proves the data source against a real
// server. Gravitino >= 1.3.1 serves GET /api/system/iceberg-rest, so the read
// must succeed with either an endpoint or null; older servers do not have the
// endpoint, so the read must fail with the version requirement.
func TestLiveAccIcebergRESTServiceDataSource(t *testing.T) {
	uri := strings.TrimRight(os.Getenv("GRAVITINO_URI"), "/")
	if uri == "" {
		t.Skip("GRAVITINO_URI is not set; skipping live acceptance tests")
	}

	c, err := client.New(uri, nil)
	if err != nil {
		t.Fatalf("invalid GRAVITINO_URI %q: %v", uri, err)
	}
	version, err := c.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("server at %s did not report a usable version: %v", uri, err)
	}

	step := resource.TestStep{
		Config: `
data "gravitino_iceberg_rest_service" "svc" {}
`,
		Check: checkIcebergURIDomain,
	}
	if !models.ServerVersionAtLeast(version.Version.Version, 1, 3, 1) {
		t.Logf("server reports %s (< 1.3.1): expecting the version-requirement diagnostic", version.Version.Version)
		step.Check = nil
		step.ExpectError = regexp.MustCompile(`(?s)requires Gravitino >= 1\.3\.1`)
	} else {
		t.Logf("server reports %s (>= 1.3.1): expecting a successful read", version.Version.Version)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps:                    []resource.TestStep{step},
	})
}

// checkIcebergURIDomain asserts the read produced a usable state: Terraform
// itself fails the read if `uri` stays unknown, so this only has to pin the
// domain of the value (empty/null, or an absolute http(s) endpoint).
func checkIcebergURIDomain(s *terraform.State) error {
	rs, ok := s.RootModule().Resources["data.gravitino_iceberg_rest_service.svc"]
	if !ok {
		return fmt.Errorf("data.gravitino_iceberg_rest_service.svc is missing from state")
	}
	uri := rs.Primary.Attributes["uri"]
	if uri != "" && !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		return fmt.Errorf("uri = %q, want null or an absolute http(s) endpoint", uri)
	}
	return nil
}
