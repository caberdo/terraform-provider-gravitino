package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListTags(ctx context.Context, metalake string) (*models.TagNameListResponse, error) {
	var result models.TagNameListResponse
	err := c.Get(ctx, "/metalakes/"+url.PathEscape(metalake)+"/tags", &result)
	return &result, err
}

func (c *Client) ListTagsDetailed(ctx context.Context, metalake string) (*models.TagListResponse, error) {
	var result models.TagListResponse
	err := c.Get(ctx, "/metalakes/"+url.PathEscape(metalake)+"/tags?details=true", &result)
	return &result, err
}

func (c *Client) GetTag(ctx context.Context, metalake, name string) (*models.TagResponse, error) {
	var result models.TagResponse
	err := c.Get(ctx, "/metalakes/"+url.PathEscape(metalake)+"/tags/"+url.PathEscape(name), &result)
	return &result, err
}

func (c *Client) CreateTag(ctx context.Context, metalake string, req *models.TagCreateRequest) (*models.TagResponse, error) {
	var result models.TagResponse
	err := c.Post(ctx, "/metalakes/"+url.PathEscape(metalake)+"/tags", req, &result)
	return &result, err
}

func (c *Client) UpdateTag(ctx context.Context, metalake, name string, updates []interface{}) (*models.TagResponse, error) {
	var result models.TagResponse
	path := fmt.Sprintf("/metalakes/%s/tags/%s", url.PathEscape(metalake), url.PathEscape(name))
	err := c.Put(ctx, path, &models.TagUpdateRequest{Updates: updates}, &result)
	return &result, err
}

func (c *Client) DeleteTag(ctx context.Context, metalake, name string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/tags/%s", url.PathEscape(metalake), url.PathEscape(name))
	var result models.DropResponse
	err := c.Delete(ctx, path, &result)
	return &result, err
}

// ListTagsForObject returns the tags associated with a metadata object
// (GET /metalakes/{metalake}/objects/{metadataObjectType}/{metadataObjectFullName}/tags?details=true).
func (c *Client) ListTagsForObject(ctx context.Context, metalake, metadataObjectType, metadataObjectFullName string) (*models.TagListResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/tags?details=true", url.PathEscape(metalake), url.PathEscape(metadataObjectType), url.PathEscape(metadataObjectFullName))
	var result models.TagListResponse
	err := c.Get(ctx, path, &result)
	return &result, err
}

// AssociateTags associates and disassociates tags with a metadata object
// (POST /metalakes/{metalake}/objects/{metadataObjectType}/{metadataObjectFullName}/tags).
// The response contains the tag names associated with the object.
func (c *Client) AssociateTags(ctx context.Context, metalake, metadataObjectType, metadataObjectFullName string, req *models.TagsAssociateRequest) (*models.TagNameListResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/tags", url.PathEscape(metalake), url.PathEscape(metadataObjectType), url.PathEscape(metadataObjectFullName))
	var result models.TagNameListResponse
	err := c.Post(ctx, path, req, &result)
	return &result, err
}

// GetTagForObject returns a single tag associated with a metadata object
// (GET /metalakes/{metalake}/objects/{metadataObjectType}/{metadataObjectFullName}/tags/{tag}).
func (c *Client) GetTagForObject(ctx context.Context, metalake, metadataObjectType, metadataObjectFullName, tag string) (*models.TagResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/objects/%s/%s/tags/%s", url.PathEscape(metalake), url.PathEscape(metadataObjectType), url.PathEscape(metadataObjectFullName), url.PathEscape(tag))
	var result models.TagResponse
	err := c.Get(ctx, path, &result)
	return &result, err
}

// ListObjectsForTag returns the metadata objects associated with a tag
// (GET /metalakes/{metalake}/tags/{tag}/objects).
func (c *Client) ListObjectsForTag(ctx context.Context, metalake, tag string) (*models.MetadataObjectListResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/tags/%s/objects", url.PathEscape(metalake), url.PathEscape(tag))
	var result models.MetadataObjectListResponse
	err := c.Get(ctx, path, &result)
	return &result, err
}
