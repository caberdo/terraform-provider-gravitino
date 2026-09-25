package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListFilesets(ctx context.Context, metalake, catalog, schema string) (*models.IdentifiersResponse, error) {
	var result models.IdentifiersResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/filesets", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema))
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) GetFileset(ctx context.Context, metalake, catalog, schema, name string) (*models.FilesetResponse, error) {
	var result models.FilesetResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/filesets/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(name))
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) CreateFileset(ctx context.Context, metalake, catalog, schema string, req *models.FilesetCreateRequest) (*models.FilesetResponse, error) {
	var result models.FilesetResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/filesets", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema))
	err := c.Post(ctx, path, req, &result)
	return &result, err
}

func (c *Client) UpdateFileset(ctx context.Context, metalake, catalog, schema, name string, updates []interface{}) (*models.FilesetResponse, error) {
	var result models.FilesetResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/filesets/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(name))
	err := c.Put(ctx, path, &models.FilesetUpdateRequest{Updates: updates}, &result)
	return &result, err
}

func (c *Client) DropFileset(ctx context.Context, metalake, catalog, schema, name string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/filesets/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(name))
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
	identifiers, err := c.ListFilesets(ctx, metalake, catalog, schema)
	if err != nil {
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

func (c *Client) ListFilesetFiles(ctx context.Context, metalake, catalog, schema, fileset string) (*models.FilesetFileListResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/filesets/%s/files", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(fileset))
	var result models.FilesetFileListResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
