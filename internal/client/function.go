package client

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

// ListFunctionsDetails returns the full function objects of a schema in a single
// request (GET .../functions?details=true, see the `details` query parameter of
// functions.yaml). The API returns either identifiers or the detailed objects,
// depending on `details`.
func (c *Client) ListFunctionsDetails(ctx context.Context, metalake, catalog, schema string) ([]models.Function, error) {
	var result models.FunctionListResponse
	path := catalogCollectionPath(metalake, catalog, schema, "functions") + "?details=true"
	err := c.Get(ctx, path, &result)
	if err != nil {
		return nil, err
	}
	return result.Functions, nil
}

// GetFunction returns a single function
// (GET .../functions/{function}).
func (c *Client) GetFunction(ctx context.Context, metalake, catalog, schema, name string) (*models.FunctionResponse, error) {
	var result models.FunctionResponse
	path := catalogEntityPath(metalake, catalog, schema, "functions", name)
	err := c.Get(ctx, path, &result)
	return &result, err
}

// RegisterFunction registers a function
// (POST .../functions, operationId registerFunction).
func (c *Client) RegisterFunction(ctx context.Context, metalake, catalog, schema string, req *models.FunctionRegisterRequest) (*models.FunctionResponse, error) {
	var result models.FunctionResponse
	path := catalogCollectionPath(metalake, catalog, schema, "functions")
	err := c.Post(ctx, path, req, &result)
	return &result, err
}

// UpdateFunction alters a function
// (PUT .../functions/{function}, operationId alterFunction). updates holds
// FunctionUpdateRequest values such as models.NewUpdateFunctionCommentRequest.
func (c *Client) UpdateFunction(ctx context.Context, metalake, catalog, schema, name string, updates []any) (*models.FunctionResponse, error) {
	var result models.FunctionResponse
	path := catalogEntityPath(metalake, catalog, schema, "functions", name)
	err := c.Put(ctx, path, &models.FunctionUpdatesRequest{Updates: updates}, &result)
	return &result, err
}

// DropFunction drops a function
// (DELETE .../functions/{function}, operationId dropFunction). The API documents
// no query parameters for this operation.
func (c *Client) DropFunction(ctx context.Context, metalake, catalog, schema, name string) (*models.DropResponse, error) {
	path := catalogEntityPath(metalake, catalog, schema, "functions", name)
	var result models.DropResponse
	err := c.Delete(ctx, path, &result)
	return &result, err
}
