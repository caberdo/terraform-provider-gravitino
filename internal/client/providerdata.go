package client

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// FromProviderData resolves the *Client the framework hands to a resource or
// data source Configure method. It returns no diagnostics when providerData is
// nil, which is how the framework signals that the provider has not been
// configured yet, and a single error otherwise.
func FromProviderData(providerData any) (*Client, diag.Diagnostics) {
	if providerData == nil {
		return nil, nil
	}

	c, ok := providerData.(*Client)
	if !ok {
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T. Please report this issue to the provider developers.", providerData),
		)}
	}

	return c, nil
}
