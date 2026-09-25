package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) AddUser(ctx context.Context, metalake string, name string) (*models.UserResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/users", url.PathEscape(metalake))
	var result models.UserResponse
	if err := c.Post(ctx, path, &models.UserCreateRequest{Name: name}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetUser(ctx context.Context, metalake, name string) (*models.UserResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/users/%s", url.PathEscape(metalake), url.PathEscape(name))
	var result models.UserResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListUsers(ctx context.Context, metalake string) (*models.NameListResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/users", url.PathEscape(metalake))
	var result models.NameListResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RemoveUser(ctx context.Context, metalake, name string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/users/%s", url.PathEscape(metalake), url.PathEscape(name))
	var result models.DropResponse
	if err := c.Delete(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GrantRolesToUser(ctx context.Context, metalake, user string, roleNames []string) (*models.UserResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/permissions/users/%s/grant", url.PathEscape(metalake), url.PathEscape(user))
	var result models.UserResponse
	if err := c.Put(ctx, path, &models.GrantRevokeRequest{RoleNames: roleNames}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RevokeRolesFromUser(ctx context.Context, metalake, user string, roleNames []string) (*models.UserResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/permissions/users/%s/revoke", url.PathEscape(metalake), url.PathEscape(user))
	var result models.UserResponse
	if err := c.Put(ctx, path, &models.GrantRevokeRequest{RoleNames: roleNames}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
