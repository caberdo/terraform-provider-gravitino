package client

import (
	"context"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) AddUser(ctx context.Context, metalake string, name string) (*models.UserResponse, error) {
	path := collectionPath(metalake, "users")
	var result models.UserResponse
	if err := c.Post(ctx, path, &models.UserCreateRequest{Name: name}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetUser(ctx context.Context, metalake, name string) (*models.UserResponse, error) {
	path := entityPath(metalake, "users", name)
	var result models.UserResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListUsers(ctx context.Context, metalake string) (*models.NameListResponse, error) {
	path := collectionPath(metalake, "users")
	var result models.NameListResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RemoveUser(ctx context.Context, metalake, name string) (*models.DropResponse, error) {
	path := entityPath(metalake, "users", name)
	var result models.DropResponse
	if err := c.Delete(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GrantRolesToUser(ctx context.Context, metalake, user string, roleNames []string) (*models.UserResponse, error) {
	path := metalakePath(metalake) + "/permissions/users/" + url.PathEscape(user) + "/grant"
	var result models.UserResponse
	if err := c.Put(ctx, path, &models.GrantRevokeRequest{RoleNames: roleNames}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RevokeRolesFromUser(ctx context.Context, metalake, user string, roleNames []string) (*models.UserResponse, error) {
	path := metalakePath(metalake) + "/permissions/users/" + url.PathEscape(user) + "/revoke"
	var result models.UserResponse
	if err := c.Put(ctx, path, &models.GrantRevokeRequest{RoleNames: roleNames}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
