package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListStatistics(ctx context.Context, metalake, resourceType, resource string) (*models.StatisticsResponse, error) {
	var result models.StatisticsResponse
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/statistics", url.PathEscape(metalake), url.PathEscape(resourceType), url.PathEscape(resource))
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) ListPartitionStatistics(ctx context.Context, metalake, resourceType, resource string) (*models.PartitionStatisticsResponse, error) {
	var result models.PartitionStatisticsResponse
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/statistics/partitions", url.PathEscape(metalake), url.PathEscape(resourceType), url.PathEscape(resource))
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) UpdateStatistics(ctx context.Context, metalake, objType, objFullName string, body interface{}) (*models.BaseResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/statistics", url.PathEscape(metalake), url.PathEscape(objType), url.PathEscape(objFullName))
	var result models.BaseResponse
	if err := c.Put(ctx, path, body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteStatistics(ctx context.Context, metalake, objType, objFullName string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/statistics", url.PathEscape(metalake), url.PathEscape(objType), url.PathEscape(objFullName))
	var result models.DropResponse
	if err := c.Delete(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdatePartitionStatistics(ctx context.Context, metalake, objType, objFullName string, body interface{}) (*models.BaseResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/statistics/partitions", url.PathEscape(metalake), url.PathEscape(objType), url.PathEscape(objFullName))
	var result models.BaseResponse
	if err := c.Put(ctx, path, body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeletePartitionStatistics(ctx context.Context, metalake, objType, objFullName string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/statistics/partitions", url.PathEscape(metalake), url.PathEscape(objType), url.PathEscape(objFullName))
	var result models.DropResponse
	if err := c.Delete(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
