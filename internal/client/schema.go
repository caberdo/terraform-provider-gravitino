package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListSchemas(ctx context.Context, metalake, catalog string) (*models.IdentifiersResponse, error) {
	var result models.IdentifiersResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas", url.PathEscape(metalake), url.PathEscape(catalog))
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) GetSchema(ctx context.Context, metalake, catalog, schema string) (*models.SchemaResponse, error) {
	var result models.SchemaResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema))
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) CreateSchema(ctx context.Context, metalake, catalog string, req *models.SchemaCreateRequest) (*models.SchemaResponse, error) {
	var result models.SchemaResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas", url.PathEscape(metalake), url.PathEscape(catalog))
	err := c.Post(ctx, path, req, &result)
	return &result, err
}

func (c *Client) UpdateSchema(ctx context.Context, metalake, catalog, schema string, updates []interface{}) (*models.SchemaResponse, error) {
	var result models.SchemaResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema))
	err := c.Put(ctx, path, &models.SchemaUpdateRequest{Updates: updates}, &result)
	return &result, err
}

// DropSchema drops a schema. The `dropSchema` operation of the v1.3.0 spec
// declares no `force` query parameter.
func (c *Client) DropSchema(ctx context.Context, metalake, catalog, schema string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema))
	var result models.DropResponse
	err := c.Delete(ctx, path, &result)
	return &result, err
}

// ListSchemasDetails lists all schemas of a catalog with their full details.
//
// The v1.3.0 spec declares no `details` query parameter on the list endpoint,
// so this performs one GET per listed identifier. Identifiers whose object has
// been deleted in the meantime (404) are skipped rather than failing the whole
// listing.
func (c *Client) ListSchemasDetails(ctx context.Context, metalake, catalog string) ([]models.Schema, error) {
	identifiers, err := c.ListSchemas(ctx, metalake, catalog)
	if err != nil {
		return nil, err
	}
	schemas := make([]models.Schema, 0, len(identifiers.Identifiers))
	for _, id := range identifiers.Identifiers {
		resp, err := c.GetSchema(ctx, metalake, catalog, id.Name)
		if err != nil {
			if IsNotFoundError(err) {
				continue
			}
			return nil, err
		}
		schemas = append(schemas, resp.Schema)
	}
	return schemas, nil
}
