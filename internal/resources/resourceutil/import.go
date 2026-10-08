// Package resourceutil holds helpers shared by the Gravitino Terraform
// resources.
package resourceutil

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// SplitImportID splits req.ID on "." and requires exactly n segments, none
// empty. The final segment may itself contain dots, so an object full name such
// as "catalog.schema" is accepted. expectedFormat names the accepted shape, for
// example "metalake.catalog.schema". On failure it appends an "Invalid import
// ID" diagnostic and returns nil, false.
func SplitImportID(req resource.ImportStateRequest, resp *resource.ImportStateResponse, n int, expectedFormat string) ([]string, bool) {
	return splitImportID(req, resp, n, expectedFormat, false)
}

// SplitImportIDExact is like SplitImportID but rejects any extra "." in the
// final segment: the whole ID must consist of exactly n segments.
func SplitImportIDExact(req resource.ImportStateRequest, resp *resource.ImportStateResponse, n int, expectedFormat string) ([]string, bool) {
	return splitImportID(req, resp, n, expectedFormat, true)
}

func splitImportID(req resource.ImportStateRequest, resp *resource.ImportStateResponse, n int, expectedFormat string, exact bool) ([]string, bool) {
	var parts []string
	if exact {
		parts = strings.Split(req.ID, ".")
	} else {
		parts = strings.SplitN(req.ID, ".", n)
	}

	if len(parts) != n {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected format %s, got %q.", expectedFormat, req.ID))
		return nil, false
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected format %s, got %q.", expectedFormat, req.ID))
			return nil, false
		}
	}
	return parts, true
}
