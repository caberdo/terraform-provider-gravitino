package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

// GetCredentials returns the credentials associated with a metadata object:
//
//	GET /api/metalakes/{metalake}/objects/{metadataObjectType}/{metadataObjectFullName}/credentials
//
// (docs/open-api/credentials.yaml). The response carries its list under the
// `credentials` key; every entry has `credentialType`, `expireTimeInMs` and
// `credentialInfo`.
func (c *Client) GetCredentials(ctx context.Context, metalake, resourceType, resource string) (*models.CredentialResponse, error) {
	var result models.CredentialResponse
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/credentials", url.PathEscape(metalake), url.PathEscape(resourceType), url.PathEscape(resource))
	err := c.Get(ctx, path, &result)
	return &result, err
}
