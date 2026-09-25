package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListPoliciesForObject(ctx context.Context, metalake, objType, objFullName string) (*models.NameListResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/policies", url.PathEscape(metalake), url.PathEscape(objType), url.PathEscape(objFullName))
	var result models.NameListResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) AssociatePolicies(ctx context.Context, metalake, objType, objFullName string, req *models.PolicyAssociationRequest) (*models.NameListResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/policies", url.PathEscape(metalake), url.PathEscape(objType), url.PathEscape(objFullName))
	var result models.NameListResponse
	if err := c.Post(ctx, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListPolicies(ctx context.Context, metalake string) (*models.PolicyListResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/policies", url.PathEscape(metalake))
	var result models.PolicyListResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreatePolicy(ctx context.Context, metalake string, req *models.PolicyCreateRequest) (*models.PolicyResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/policies", url.PathEscape(metalake))
	var result models.PolicyResponse
	if err := c.Post(ctx, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetPolicy(ctx context.Context, metalake, name string) (*models.PolicyResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/policies/%s", url.PathEscape(metalake), url.PathEscape(name))
	var result models.PolicyResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdatePolicy sends a `PolicyUpdatesRequest` to
// PUT /metalakes/{metalake}/policies/{policy}. Only the update request types
// defined by the v1.3.0 spec may be passed in updates.
func (c *Client) UpdatePolicy(ctx context.Context, metalake, name string, updates []interface{}) (*models.PolicyResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/policies/%s", url.PathEscape(metalake), url.PathEscape(name))
	var result models.PolicyResponse
	if err := c.Put(ctx, path, &models.PolicyUpdatesRequest{Updates: updates}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetPolicyEnabled enables or disables a policy via
// PATCH /metalakes/{metalake}/policies/{policy} with a `PolicySetRequest` body.
// The response is a `BaseResponse`, not a `PolicyResponse`.
func (c *Client) SetPolicyEnabled(ctx context.Context, metalake, name string, enable bool) (*models.BaseResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/policies/%s", url.PathEscape(metalake), url.PathEscape(name))
	var result models.BaseResponse
	if err := c.Patch(ctx, path, &models.PolicySetRequest{Enable: enable}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeletePolicy(ctx context.Context, metalake, name string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/policies/%s", url.PathEscape(metalake), url.PathEscape(name))
	var result models.DropResponse
	if err := c.Delete(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListObjectsForPolicy(ctx context.Context, metalake, name string) (*models.IdentifiersResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/policies/%s/objects", url.PathEscape(metalake), url.PathEscape(name))
	var result models.IdentifiersResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
