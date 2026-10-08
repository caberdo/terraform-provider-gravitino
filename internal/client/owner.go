package client

import (
	"context"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) GetOwner(ctx context.Context, metalake, objectType, objectFullName string) (*models.OwnerResponse, error) {
	path := metalakePath(metalake) + "/owners/" + url.PathEscape(objectType) + "/" + url.PathEscape(objectFullName)
	var result models.OwnerResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) SetOwner(ctx context.Context, metalake, objectType, objectFullName string, req *models.SetOwnerRequest) (*models.SetOwnerResponse, error) {
	path := metalakePath(metalake) + "/owners/" + url.PathEscape(objectType) + "/" + url.PathEscape(objectFullName)
	var result models.SetOwnerResponse
	if err := c.Put(ctx, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
