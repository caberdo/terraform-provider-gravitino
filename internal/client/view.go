package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListViews(ctx context.Context, metalake, catalog, schema string) (*models.IdentifiersResponse, error) {
	var result models.IdentifiersResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/views", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema))
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) ListViewsDetails(ctx context.Context, metalake, catalog, schema string) ([]models.View, error) {
	identifiers, err := c.ListViews(ctx, metalake, catalog, schema)
	if err != nil {
		return nil, err
	}
	views := make([]models.View, 0, len(identifiers.Identifiers))
	for _, id := range identifiers.Identifiers {
		resp, err := c.GetView(ctx, metalake, catalog, schema, id.Name)
		if err != nil {
			// views.yaml (v1.3.0) has no `details` query parameter, so the list
			// data source has to load every view individually. A view that is
			// dropped between the list and the GET must not fail the whole
			// listing, so it is skipped.
			if IsNotFoundError(err) {
				continue
			}
			return nil, err
		}
		views = append(views, resp.View)
	}
	return views, nil
}

func (c *Client) GetView(ctx context.Context, metalake, catalog, schema, name string) (*models.ViewResponse, error) {
	var result models.ViewResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/views/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(name))
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) CreateView(ctx context.Context, metalake, catalog, schema string, req *models.ViewCreateRequest) (*models.ViewResponse, error) {
	var result models.ViewResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/views", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema))
	err := c.Post(ctx, path, req, &result)
	return &result, err
}

func (c *Client) UpdateView(ctx context.Context, metalake, catalog, schema, name string, updates []interface{}) (*models.ViewResponse, error) {
	var result models.ViewResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/views/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(name))
	err := c.Put(ctx, path, &models.ViewUpdateRequest{Updates: updates}, &result)
	return &result, err
}

func (c *Client) DropView(ctx context.Context, metalake, catalog, schema, name string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/views/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(name))
	var result models.DropResponse
	err := c.Delete(ctx, path, &result)
	return &result, err
}
