package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) AddIdpUser(ctx context.Context, req *models.IdpAddUserRequest) (*models.IdpUserResponse, error) {
	var result models.IdpUserResponse
	if err := c.Post(ctx, "/idp/users", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetIdpUser(ctx context.Context, user string) (*models.IdpUserResponse, error) {
	path := fmt.Sprintf("/idp/users/%s", url.PathEscape(user))
	var result models.IdpUserResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ChangeIdpUserPassword implements PUT /idp/users/{user}, whose body is a
// ChangePasswordRequest: the endpoint cannot change anything else about a user.
func (c *Client) ChangeIdpUserPassword(ctx context.Context, user string, req *models.IdpChangePasswordRequest) (*models.IdpUserResponse, error) {
	path := fmt.Sprintf("/idp/users/%s", url.PathEscape(user))
	var result models.IdpUserResponse
	if err := c.Put(ctx, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RemoveIdpUser(ctx context.Context, user string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/idp/users/%s", url.PathEscape(user))
	var result models.DropResponse
	if err := c.Delete(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) AddIdpGroup(ctx context.Context, req *models.IdpAddGroupRequest) (*models.IdpGroupResponse, error) {
	var result models.IdpGroupResponse
	if err := c.Post(ctx, "/idp/groups", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetIdpGroup(ctx context.Context, group string) (*models.IdpGroupResponse, error) {
	path := fmt.Sprintf("/idp/groups/%s", url.PathEscape(group))
	var result models.IdpGroupResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ChangeIdpGroupMembership implements PUT /idp/groups/{group}/users. It is the
// only way to add or remove members of a built-in IDP group.
func (c *Client) ChangeIdpGroupMembership(ctx context.Context, group string, req *models.IdpGroupMembershipChangeRequest) (*models.IdpGroupResponse, error) {
	path := fmt.Sprintf("/idp/groups/%s/users", url.PathEscape(group))
	var result models.IdpGroupResponse
	if err := c.Put(ctx, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RemoveIdpGroup(ctx context.Context, group string, force bool) (*models.DropResponse, error) {
	path := fmt.Sprintf("/idp/groups/%s?force=%t", url.PathEscape(group), force)
	var result models.DropResponse
	if err := c.Delete(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
