package health

import (
	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// checkServerVersion rejects a server that predates Gravitino 1.3.0, which
// introduced the health endpoints. An unknown version stays permissive,
// matching client.AtLeast.
func checkServerVersion(c *client.Client, diags *diag.Diagnostics) bool {
	if err := c.CheckMinimumVersion(1, 3, 0, "the Gravitino health data sources"); err != nil {
		diags.AddError("Gravitino 1.3.0 or newer required", err.Error())
		return false
	}
	return true
}
