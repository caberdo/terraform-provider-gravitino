package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

// ListRoles lists the role names that are attached to a metadata object
// (roles.yaml path /metalakes/{metalake}/objects/{metadataObjectType}/{metadataObjectFullName}/roles).
// The endpoint answers with a NameListResponse, not with role details.
func (c *Client) ListRoles(ctx context.Context, metalake, objectType, objectFullName string) (*models.NameListResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/roles", url.PathEscape(metalake), url.PathEscape(objectType), url.PathEscape(objectFullName))
	var result models.NameListResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateRole(ctx context.Context, metalake string, req *models.RoleCreateRequest) (*models.RoleResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/roles", url.PathEscape(metalake))

	// The server rejects a create request whose `securableObjects` is absent
	// (RoleCreateRequest.validate()), so always send the array.
	body := *req
	if body.SecurableObjects == nil {
		body.SecurableObjects = []models.SecurableObject{}
	}

	var result models.RoleResponse
	if err := c.Post(ctx, path, &body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetRole(ctx context.Context, metalake, name string) (*models.RoleResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/roles/%s", url.PathEscape(metalake), url.PathEscape(name))
	var result models.RoleResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteRole(ctx context.Context, metalake, name string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/roles/%s", url.PathEscape(metalake), url.PathEscape(name))
	var result models.DropResponse
	if err := c.Delete(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListAllRoles(ctx context.Context, metalake string) (*models.NameListResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/roles", url.PathEscape(metalake))
	var result models.NameListResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GrantPrivilegeToRole(ctx context.Context, metalake, role, objectType, objectFullName string, privileges []models.Privilege) (*models.RoleResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/permissions/roles/%s/%s/%s/grant", url.PathEscape(metalake), url.PathEscape(role), url.PathEscape(objectType), url.PathEscape(objectFullName))
	var result models.RoleResponse
	if err := c.Put(ctx, path, &models.PrivilegesRequest{Privileges: privileges}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RevokePrivilegeFromRole(ctx context.Context, metalake, role, objectType, objectFullName string, privileges []models.Privilege) (*models.RoleResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/permissions/roles/%s/%s/%s/revoke", url.PathEscape(metalake), url.PathEscape(role), url.PathEscape(objectType), url.PathEscape(objectFullName))
	var result models.RoleResponse
	if err := c.Put(ctx, path, &models.PrivilegesRequest{Privileges: privileges}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) OverrideRolePrivileges(ctx context.Context, metalake, role string, overrides []models.SecurableObject) (*models.RoleResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/permissions/roles/%s", url.PathEscape(metalake), url.PathEscape(role))

	// `overrides` is part of the (required) request schema; send an empty array
	// instead of omitting the field.
	if overrides == nil {
		overrides = []models.SecurableObject{}
	}

	var result models.RoleResponse
	if err := c.Put(ctx, path, &models.PrivilegeOverrideRequest{Overrides: overrides}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
