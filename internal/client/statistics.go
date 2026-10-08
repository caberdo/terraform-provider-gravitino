package client

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListStatistics(ctx context.Context, metalake, resourceType, resource string) (*models.StatisticsResponse, error) {
	var result models.StatisticsResponse
	path := objectPath(metalake, resourceType, resource) + "/statistics"
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) ListPartitionStatistics(ctx context.Context, metalake, resourceType, resource string) (*models.PartitionStatisticsResponse, error) {
	var result models.PartitionStatisticsResponse
	path := objectPath(metalake, resourceType, resource) + "/statistics/partitions"
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) UpdateStatistics(ctx context.Context, metalake, objType, objFullName string, body interface{}) (*models.BaseResponse, error) {
	path := objectPath(metalake, objType, objFullName) + "/statistics"
	var result models.BaseResponse
	if err := c.Put(ctx, path, body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteStatistics(ctx context.Context, metalake, objType, objFullName string) (*models.DropResponse, error) {
	path := objectPath(metalake, objType, objFullName) + "/statistics"
	var result models.DropResponse
	if err := c.Delete(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdatePartitionStatistics(ctx context.Context, metalake, objType, objFullName string, body interface{}) (*models.BaseResponse, error) {
	path := objectPath(metalake, objType, objFullName) + "/statistics/partitions"
	var result models.BaseResponse
	if err := c.Put(ctx, path, body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeletePartitionStatistics(ctx context.Context, metalake, objType, objFullName string) (*models.DropResponse, error) {
	path := objectPath(metalake, objType, objFullName) + "/statistics/partitions"
	var result models.DropResponse
	if err := c.Delete(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
