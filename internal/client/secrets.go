package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) GetSecrets(ctx context.Context, metalake, objType, objFullName string) (*models.SecretsResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/secrets", url.PathEscape(metalake), url.PathEscape(objType), url.PathEscape(objFullName))
	var result models.SecretsResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
