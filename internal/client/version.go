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
// The version detected at provider.Configure is used when it is known, so the
// check normally costs nothing; GET /api/version is only queried when the
// provider could not detect the version and the caller uses one of those two
// types.
func (c *Client) CheckMetadataObjectTypeSupported(ctx context.Context, objectType string) error {
	if !models.ObjectTypeRequiresGravitino131(objectType) {
		return nil
	}

	version := c.ServerVersion()
	if !models.ValidServerVersion(version) {
		resp, err := c.GetVersion(ctx)
		if err != nil {
			return fmt.Errorf("cannot verify that metadata object type %q is supported by the server: %w", objectType, err)
		}
		version = resp.Version.Version
		if models.ValidServerVersion(version) {
			c.setServerVersion(version)
		}
	}

	if !models.ServerVersionAtLeast(version, 1, 3, 1) {
		return fmt.Errorf("metadata object type %q requires Gravitino 1.3.1 or newer, but the server reports version %q",
			objectType, version)
	}
	return nil
}
