package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func partitionsPath(metalake, catalog, schema, table string) string {
	return fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/tables/%s/partitions",
		url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(table))
}

// ListPartitions returns the partitions of a table, including their type,
// field names, values and properties, in a single request. The details query
// parameter is defined by partitions.yaml for the list partitions operation.
func (c *Client) ListPartitions(ctx context.Context, metalake, catalog, schema, table string) ([]models.Partition, error) {
	var result models.PartitionListResponse
	err := c.Get(ctx, partitionsPath(metalake, catalog, schema, table)+"?details=true", &result)
	return result.Partitions, err
}

// GetPartition returns a single partition of a table.
func (c *Client) GetPartition(ctx context.Context, metalake, catalog, schema, table, name string) (*models.PartitionResponse, error) {
	var result models.PartitionResponse
	path := partitionsPath(metalake, catalog, schema, table) + "/" + url.PathEscape(name)
	err := c.Get(ctx, path, &result)
	return &result, err
}

// AddPartitions adds partitions to a table. The response contains the added
// partitions, including the names the catalog derived for them.
func (c *Client) AddPartitions(ctx context.Context, metalake, catalog, schema, table string, partitions []models.Partition) (*models.PartitionListResponse, error) {
	var result models.PartitionListResponse
	body := &models.AddPartitionsRequest{Partitions: partitions}
	err := c.Post(ctx, partitionsPath(metalake, catalog, schema, table), body, &result)
	return &result, err
}

// DropPartition drops a single partition of a table.
func (c *Client) DropPartition(ctx context.Context, metalake, catalog, schema, table, name string) (*models.DropResponse, error) {
	var result models.DropResponse
	path := partitionsPath(metalake, catalog, schema, table) + "/" + url.PathEscape(name)
	err := c.Delete(ctx, path, &result)
	return &result, err
}
