package client

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

// GetAuthenticatedPrincipal resolves the server-side principal of the current
// authenticated user:
//
//	GET /api/authn/me -> { "code": 0, "principal": "admin", "serviceAdmin": true }
//
// (docs/open-api/authn.yaml; Gravitino 1.3.1 renamed the operation id to
// getAuthenticatedUser and added serviceAdmin to the response). The response
// contains the principal and service-admin status only; it does not return
// roles.
func (c *Client) GetAuthenticatedPrincipal(ctx context.Context) (*models.AuthMeResponse, error) {
	var result models.AuthMeResponse
	err := c.Get(ctx, "/authn/me", &result)
	return &result, err
}
