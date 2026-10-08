package client

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) GetSecrets(ctx context.Context, metalake, objType, objFullName string) (*models.SecretsResponse, error) {
	path := objectPath(metalake, objType, objFullName) + "/secrets"
	var result models.SecretsResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
