package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) GetOwner(ctx context.Context, metalake, objectType, objectFullName string) (*models.OwnerResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/owners/%s/%s", url.PathEscape(metalake), url.PathEscape(objectType), url.PathEscape(objectFullName))
	var result models.OwnerResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) SetOwner(ctx context.Context, metalake, objectType, objectFullName string, req *models.SetOwnerRequest) (*models.SetOwnerResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/owners/%s/%s", url.PathEscape(metalake), url.PathEscape(objectType), url.PathEscape(objectFullName))
	var result models.SetOwnerResponse
	if err := c.Put(ctx, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
