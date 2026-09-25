package client

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) GetVersion(ctx context.Context) (*models.VersionResponse, error) {
	var result models.VersionResponse
	err := c.Get(ctx, "/version", &result)
	return &result, err
}
