package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) AddGroup(ctx context.Context, metalake string, name string) (*models.GroupResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/groups", url.PathEscape(metalake))
	var result models.GroupResponse
	if err := c.Post(ctx, path, &models.GroupCreateRequest{Name: name}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetGroup(ctx context.Context, metalake, name string) (*models.GroupResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/groups/%s", url.PathEscape(metalake), url.PathEscape(name))
	var result models.GroupResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListGroups(ctx context.Context, metalake string) (*models.NameListResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/groups", url.PathEscape(metalake))
	var result models.NameListResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RemoveGroup(ctx context.Context, metalake, name string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/groups/%s", url.PathEscape(metalake), url.PathEscape(name))
	var result models.DropResponse
	if err := c.Delete(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GrantRolesToGroup(ctx context.Context, metalake, group string, roleNames []string) (*models.GroupResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/permissions/groups/%s/grant", url.PathEscape(metalake), url.PathEscape(group))
	var result models.GroupResponse
	if err := c.Put(ctx, path, &models.GrantRevokeRequest{RoleNames: roleNames}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RevokeRolesFromGroup(ctx context.Context, metalake, group string, roleNames []string) (*models.GroupResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/permissions/groups/%s/revoke", url.PathEscape(metalake), url.PathEscape(group))
	var result models.GroupResponse
	if err := c.Put(ctx, path, &models.GrantRevokeRequest{RoleNames: roleNames}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
