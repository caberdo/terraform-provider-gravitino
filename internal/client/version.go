package client

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
// check normally costs no request of its own.
func (c *Client) CheckMetadataObjectTypeSupported(ctx context.Context, objectType string) error {
	if !models.ObjectTypeRequiresGravitino131(objectType) {
		return nil
	}

	version, err := c.ResolveServerVersion(ctx)
	if err != nil {
		return fmt.Errorf("cannot verify that metadata object type %q is supported by the server: %w", objectType, err)
	}

	if !models.ServerVersionAtLeast(version, 1, 3, 1) {
		return fmt.Errorf("metadata object type %q requires Gravitino 1.3.1 or newer, but the server reports version %q",
			objectType, version)
	}
	return nil
}

// CheckPolicyObjectTypesSupported returns an error when one of the given
// `supportedObjectTypes` values is one that Gravitino 1.3.1 added to
// PolicyContentBase (VIEW, FUNCTION) and the server reports an older version.
//
// The version detected at provider.Configure is used when it is known, so the
// check normally costs no request of its own.
func (c *Client) CheckPolicyObjectTypesSupported(ctx context.Context, objectTypes []string) error {
	unsupported := make([]string, 0, len(objectTypes))
	for _, objectType := range objectTypes {
		if models.ObjectTypeRequiresGravitino131(objectType) {
			unsupported = append(unsupported, models.CanonicalObjectType(objectType))
		}
	}
	if len(unsupported) == 0 {
		return nil
	}

	// The values come from a Terraform set, which has no order; sort them so the
	// diagnostic is stable.
	sort.Strings(unsupported)
	values := strings.Join(unsupported, ", ")

	version, err := c.ResolveServerVersion(ctx)
	if err != nil {
		return fmt.Errorf("cannot verify that supported_object_types value(s) %s are supported by the server: %w", values, err)
	}

	if !models.ServerVersionAtLeast(version, 1, 3, 1) {
		return fmt.Errorf("supported_object_types value(s) %s require Gravitino >= 1.3.1, but the server reports version %q",
			values, version)
	}
	return nil
}

// CheckIndexesSupported returns an error when an index uses one of the
// indexes.yaml#/IndexSpec additions of Gravitino 1.3.1 (a data_skipping_* index
// type, or custom index properties) and the server reports an older version.
// A 1.3.0 server rejects both with an opaque error, so the provider verifies
// the server version first.
//
// The version detected at provider.Configure is used when it is known, so the
// check normally costs no request of its own.
func (c *Client) CheckIndexesSupported(ctx context.Context, indexes []models.Index) error {
	restricted := make([]string, 0, len(indexes))
	hasProperties := false
	for _, index := range indexes {
		if models.IndexTypeRequiresGravitino131(index.IndexType) {
			restricted = append(restricted, strings.ToLower(strings.TrimSpace(index.IndexType)))
		}
		if len(index.Properties) > 0 {
			hasProperties = true
		}
	}
	if len(restricted) == 0 && !hasProperties {
		return nil
	}

	// The indexes come from a Terraform block list, which has an order, but the
	// types are sorted anyway so the diagnostic is stable when the same type
	// appears more than once.
	sort.Strings(restricted)

	feature := "the index properties"
	switch {
	case len(restricted) > 0 && hasProperties:
		feature = fmt.Sprintf("index type(s) %s and the index properties", strings.Join(restricted, ", "))
	case len(restricted) > 0:
		feature = fmt.Sprintf("index type(s) %s", strings.Join(restricted, ", "))
	}

	version, err := c.ResolveServerVersion(ctx)
	if err != nil {
		return fmt.Errorf("cannot verify that %s are supported by the server: %w", feature, err)
	}

	if !models.ServerVersionAtLeast(version, 1, 3, 1) {
		return fmt.Errorf("%s require Gravitino >= 1.3.1, but the server reports version %q",
			feature, version)
	}
	return nil
}
