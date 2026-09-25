package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListCatalogs(ctx context.Context, metalake string) (*models.IdentifiersResponse, error) {
	var result models.IdentifiersResponse
	err := c.Get(ctx, "/metalakes/"+url.PathEscape(metalake)+"/catalogs", &result)
	return &result, err
}

func (c *Client) ListCatalogsDetails(ctx context.Context, metalake string) (*models.CatalogInfoListResponse, error) {
	var result models.CatalogInfoListResponse
	err := c.Get(ctx, "/metalakes/"+url.PathEscape(metalake)+"/catalogs?details=true", &result)
	return &result, err
}

func (c *Client) GetCatalog(ctx context.Context, metalake, name string) (*models.CatalogResponse, error) {
	var result models.CatalogResponse
	err := c.Get(ctx, "/metalakes/"+url.PathEscape(metalake)+"/catalogs/"+url.PathEscape(name), &result)
	return &result, err
}

func (c *Client) CreateCatalog(ctx context.Context, metalake string, req *models.CatalogCreateRequest) (*models.CatalogResponse, error) {
	var result models.CatalogResponse
	err := c.Post(ctx, "/metalakes/"+url.PathEscape(metalake)+"/catalogs", req, &result)
	return &result, err
}

func (c *Client) UpdateCatalog(ctx context.Context, metalake, name string, updates []interface{}) (*models.CatalogResponse, error) {
	var result models.CatalogResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s", url.PathEscape(metalake), url.PathEscape(name))
	err := c.Put(ctx, path, &models.CatalogUpdateRequest{Updates: updates}, &result)
	return &result, err
}

// SetCatalogInUse marks a catalog as in-use (or not), via
// PATCH /metalakes/{metalake}/catalogs/{catalog} with a `CatalogSetRequest` body.
func (c *Client) SetCatalogInUse(ctx context.Context, metalake, name string, inUse bool) (*models.BaseResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s", url.PathEscape(metalake), url.PathEscape(name))
	var result models.BaseResponse
	if err := c.Patch(ctx, path, &models.CatalogSetRequest{InUse: inUse}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DropCatalog(ctx context.Context, metalake, name string, force bool) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s?force=%t", url.PathEscape(metalake), url.PathEscape(name), force)
	var result models.DropResponse
	err := c.Delete(ctx, path, &result)
	return &result, err
}

func (c *Client) TestCatalogConnection(ctx context.Context, metalake, catalogName string, testReq interface{}) (*models.BaseResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/testConnection", url.PathEscape(metalake), url.PathEscape(catalogName))
	var result models.BaseResponse
	if err := c.Post(ctx, path, testReq, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) TestCatalogConfig(ctx context.Context, metalake string, testReq interface{}) (*models.BaseResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/catalogs/testConnection", url.PathEscape(metalake))
	var result models.BaseResponse
	if err := c.Post(ctx, path, testReq, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
