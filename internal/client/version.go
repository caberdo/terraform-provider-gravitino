package client

import "github.com/gravitino/terraform-provider-gravitino/internal/models"

func (c *Client) GetVersion() (*models.VersionResponse, error) {
	var result models.VersionResponse
	err := c.Get("/version", &result)
	return &result, err
}
