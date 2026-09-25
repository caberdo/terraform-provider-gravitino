package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func tablesPath(metalake, catalog, schema string) string {
	return fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/tables",
		url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema))
}

func tablePath(metalake, catalog, schema, table string) string {
	return tablesPath(metalake, catalog, schema) + "/" + url.PathEscape(table)
}

// ListTables returns the identifiers of the tables in a schema.
func (c *Client) ListTables(ctx context.Context, metalake, catalog, schema string) (*models.IdentifiersResponse, error) {
	var result models.IdentifiersResponse
	err := c.Get(ctx, tablesPath(metalake, catalog, schema), &result)
	return &result, err
}

// GetTable returns a single table of a schema.
func (c *Client) GetTable(ctx context.Context, metalake, catalog, schema, table string) (*models.TableResponse, error) {
	var result models.TableResponse
	err := c.Get(ctx, tablePath(metalake, catalog, schema, table), &result)
	return &result, err
}

// CreateTable creates a table in a schema.
func (c *Client) CreateTable(ctx context.Context, metalake, catalog, schema string, req *models.TableCreateRequest) (*models.TableResponse, error) {
	var result models.TableResponse
	err := c.Post(ctx, tablesPath(metalake, catalog, schema), req, &result)
	return &result, err
}

// UpdateTable applies table updates to an existing table. updates is serialised
// as the updates array of tables.yaml#/TableUpdatesRequest.
func (c *Client) UpdateTable(ctx context.Context, metalake, catalog, schema, table string, updates []models.TableUpdateRequest) (*models.TableResponse, error) {
	var result models.TableResponse
	err := c.Put(ctx, tablePath(metalake, catalog, schema, table), &models.TableUpdatesRequest{Updates: updates}, &result)
	return &result, err
}

// DropTable drops a table. partitions.yaml and tables.yaml define no force
// parameter; purge defaults to false, which keeps the data of the table.
func (c *Client) DropTable(ctx context.Context, metalake, catalog, schema, table string) (*models.DropResponse, error) {
	var result models.DropResponse
	err := c.Delete(ctx, tablePath(metalake, catalog, schema, table), &result)
	return &result, err
}
