package client

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListMetalakes(ctx context.Context) (*models.MetalakeListResponse, error) {
	var result models.MetalakeListResponse
	err := c.Get(ctx, "/metalakes", &result)
	return &result, err
}

func (c *Client) GetMetalake(ctx context.Context, name string) (*models.MetalakeResponse, error) {
	var result models.MetalakeResponse
	err := c.Get(ctx, metalakePath(name), &result)
	return &result, err
}

func (c *Client) CreateMetalake(ctx context.Context, req *models.MetalakeCreateRequest) (*models.MetalakeResponse, error) {
	var result models.MetalakeResponse
	err := c.Post(ctx, "/metalakes", req, &result)
	return &result, err
}

func (c *Client) UpdateMetalake(ctx context.Context, name string, updates []interface{}) (*models.MetalakeResponse, error) {
	var result models.MetalakeResponse
	err := c.Put(ctx, metalakePath(name), &models.MetalakeUpdateRequest{Updates: updates}, &result)
	return &result, err
}

func (c *Client) DropMetalake(ctx context.Context, name string, force bool) (*models.DropResponse, error) {
	path := metalakePath(name) + fmt.Sprintf("?force=%t", force)
	var result models.DropResponse
	err := c.Delete(ctx, path, &result)
	return &result, err
}

// SetMetalakeInUse marks a metalake as in-use (or not), via
// PATCH /metalakes/{metalake} with a `MetalakeSetRequest` body.
func (c *Client) SetMetalakeInUse(ctx context.Context, metalake string, inUse bool) (*models.BaseResponse, error) {
	path := metalakePath(metalake)
	var result models.BaseResponse
	if err := c.Patch(ctx, path, &models.MetalakeSetRequest{InUse: inUse}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
