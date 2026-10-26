package client

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) GetFileset(ctx context.Context, metalake, catalog, schema, name string) (*models.FilesetResponse, error) {
	var result models.FilesetResponse
	path := catalogEntityPath(metalake, catalog, schema, "filesets", name)
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) CreateFileset(ctx context.Context, metalake, catalog, schema string, req *models.FilesetCreateRequest) (*models.FilesetResponse, error) {
	var result models.FilesetResponse
	path := catalogCollectionPath(metalake, catalog, schema, "filesets")
	err := c.Post(ctx, path, req, &result)
	return &result, err
}

func (c *Client) UpdateFileset(ctx context.Context, metalake, catalog, schema, name string, updates []interface{}) (*models.FilesetResponse, error) {
	var result models.FilesetResponse
	path := catalogEntityPath(metalake, catalog, schema, "filesets", name)
	err := c.Put(ctx, path, &models.FilesetUpdateRequest{Updates: updates}, &result)
	return &result, err
}

func (c *Client) DropFileset(ctx context.Context, metalake, catalog, schema, name string) (*models.DropResponse, error) {
	path := catalogEntityPath(metalake, catalog, schema, "filesets", name)
	var result models.DropResponse
	err := c.Delete(ctx, path, &result)
	return &result, err
}

// ListFilesetsDetails lists all filesets of a schema with their full details.
//
// The v1.3.0 spec declares no `details` query parameter on the list endpoint,
// so this performs one GET per listed identifier. Identifiers whose object has
// been deleted in the meantime (404) are skipped rather than failing the whole
// listing.
func (c *Client) ListFilesetsDetails(ctx context.Context, metalake, catalog, schema string) ([]models.Fileset, error) {
	var identifiers models.IdentifiersResponse
	if err := c.Get(ctx, catalogCollectionPath(metalake, catalog, schema, "filesets"), &identifiers); err != nil {
		return nil, err
	}
	filesets := make([]models.Fileset, 0, len(identifiers.Identifiers))
	for _, id := range identifiers.Identifiers {
		resp, err := c.GetFileset(ctx, metalake, catalog, schema, id.Name)
		if err != nil {
			if IsNotFoundError(err) {
				continue
			}
			return nil, err
		}
		filesets = append(filesets, resp.Fileset)
	}
	return filesets, nil
}
