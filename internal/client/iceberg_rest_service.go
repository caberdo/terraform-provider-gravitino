package client

import (
	"context"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

// GetIcebergRestServiceURI discovers the Iceberg REST service endpoint
// advertised by the server (GET /api/system/iceberg-rest, Gravitino >= 1.3.1).
//
// An empty metalake omits the query parameter, which asks the server to report
// the endpoint regardless of which metalake it serves. The returned URI is nil
// when the server advertises no endpoint for the requested metalake.
func (c *Client) GetIcebergRestServiceURI(ctx context.Context, metalake string) (*models.IcebergRESTServiceResponse, error) {
	path := "/system/iceberg-rest"
	if metalake != "" {
		path += "?metalake=" + url.QueryEscape(metalake)
	}

	var result models.IcebergRESTServiceResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
