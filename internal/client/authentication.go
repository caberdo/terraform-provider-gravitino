package client

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

// GetAuthenticatedPrincipal resolves the server-side principal of the current
// authenticated user:
//
//	GET /api/authn/me -> { "code": 0, "principal": "admin" }
//
// (docs/open-api/authn.yaml). The response contains the principal only; it does
// not return roles.
func (c *Client) GetAuthenticatedPrincipal(ctx context.Context) (*models.AuthMeResponse, error) {
	var result models.AuthMeResponse
	err := c.Get(ctx, "/authn/me", &result)
	return &result, err
}
