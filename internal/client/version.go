package client

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) GetVersion(ctx context.Context) (*models.VersionResponse, error) {
	var result models.VersionResponse
	err := c.Get(ctx, "/version", &result)
	return &result, err
}

// CheckMetadataObjectTypeSupported returns an error when objectType is one of
// the metadata object types that Gravitino 1.3.1 added to the shared
// `metadataObjectType` path parameter (VIEW, FUNCTION, used by the statistics
// and credentials endpoints) and the server reports an older version.
//
// GET /api/version is queried only for those two types, so the check costs
// nothing for the types that exist since 1.3.0.
func (c *Client) CheckMetadataObjectTypeSupported(ctx context.Context, objectType string) error {
	if !models.ObjectTypeRequiresGravitino131(objectType) {
		return nil
	}

	version, err := c.GetVersion(ctx)
	if err != nil {
		return fmt.Errorf("cannot verify that metadata object type %q is supported by the server: %w", objectType, err)
	}

	if !models.ServerVersionAtLeast(version.Version.Version, 1, 3, 1) {
		return fmt.Errorf("metadata object type %q requires Gravitino 1.3.1 or newer, but the server reports version %q",
			objectType, version.Version.Version)
	}
	return nil
}
