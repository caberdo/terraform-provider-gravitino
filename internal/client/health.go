package client

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) GetHealth(ctx context.Context) (*models.HealthResponse, error) {
	var result models.HealthResponse
	err := c.Get(ctx, "/health", &result)
	return &result, err
}

func (c *Client) GetLiveness(ctx context.Context) (*models.HealthResponse, error) {
	var result models.HealthResponse
	err := c.Get(ctx, "/health/live", &result)
	return &result, err
}

func (c *Client) GetReadiness(ctx context.Context) (*models.HealthResponse, error) {
	var result models.HealthResponse
	err := c.Get(ctx, "/health/ready", &result)
	return &result, err
}
