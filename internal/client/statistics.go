package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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

// UpdateStatistics calls updateStatistics (PUT .../statistics).
func (c *Client) UpdateStatistics(ctx context.Context, metalake, objType, objFullName string, req *models.StatisticsUpdateRequest) (*models.BaseResponse, error) {
	path := objectPath(metalake, objType, objFullName) + "/statistics"
	var result models.BaseResponse
	if err := c.Put(ctx, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteStatistics calls dropStatistics (POST .../statistics). statistics.yaml
// exposes the drop as a POST carrying the statistic names; there is no DELETE
// endpoint for statistics.
func (c *Client) DeleteStatistics(ctx context.Context, metalake, objType, objFullName string, req *models.StatisticsDropRequest) (*models.DropResponse, error) {
	path := objectPath(metalake, objType, objFullName) + "/statistics"
	var result models.DropResponse
	if err := c.Post(ctx, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdatePartitionStatistics calls updatePartitionStatistics
// (PUT .../statistics/partitions).
func (c *Client) UpdatePartitionStatistics(ctx context.Context, metalake, objType, objFullName string, req *models.PartitionStatisticsUpdateRequest) (*models.BaseResponse, error) {
	path := objectPath(metalake, objType, objFullName) + "/statistics/partitions"
	var result models.BaseResponse
	if err := c.Put(ctx, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeletePartitionStatistics calls dropPartitionStatistics
// (POST .../statistics/partitions), which is the only drop endpoint in
// statistics.yaml.
func (c *Client) DeletePartitionStatistics(ctx context.Context, metalake, objType, objFullName string, req *models.PartitionStatisticsDropRequest) (*models.DropResponse, error) {
	path := objectPath(metalake, objType, objFullName) + "/statistics/partitions"
	var result models.DropResponse
	if err := c.Post(ctx, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// NewStatisticsError turns the two refusals statistics.yaml documents for a
// write into diagnostics that name the cause, and delegates everything else to
// NewResourceError.
//
//   - 409 Conflict is UnmodifiableStatisticException: the statistic belongs to
//     the underlying catalog (for example Iceberg), which owns and recomputes
//     it, so Gravitino refuses a user write.
//   - 501 Not Implemented is UnsupportedOperationException: the object's
//     catalog does not implement the statistics operation at all.
func NewStatisticsError(operation, resource string, err error) diag.Diagnostics {
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		return NewResourceError(operation, resource, err)
	}

	switch httpErr.StatusCode {
	case http.StatusConflict:
		return diag.Diagnostics{diag.NewErrorDiagnostic(
			fmt.Sprintf("Statistic is unmodifiable (%s %q)", operation, resource),
			fmt.Sprintf("Gravitino refused the request with [409 Conflict]: %v. "+
				"The statistic is owned by the catalog behind %q (for example Iceberg), which recomputes it; "+
				"only custom statistics that Gravitino manages itself can be written or dropped. "+
				"Remove the statistic from the resource configuration instead of setting it here.",
				httpErr, resource),
		)}
	case http.StatusNotImplemented:
		return diag.Diagnostics{diag.NewErrorDiagnostic(
			fmt.Sprintf("Statistics are not supported (%s %q)", operation, resource),
			fmt.Sprintf("Gravitino refused the request with [501 Not Implemented]: %v. "+
				"The catalog behind %q does not implement statistics. "+
				"Statistics are available on lakehouse catalogs (for example Hive or Iceberg); "+
				"remove this resource or point it at a supported table.",
				httpErr, resource),
		)}
	default:
		return NewResourceError(operation, resource, err)
	}
}
