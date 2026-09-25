package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListTopics(ctx context.Context, metalake, catalog, schema string) (*models.IdentifiersResponse, error) {
	var result models.IdentifiersResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/topics", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema))
	err := c.Get(ctx, path, &result)
	return &result, err
}

// ListTopicsDetails lists the topic identifiers and then loads each topic.
// The v1.3.0 spec declares no `details` query parameter for this endpoint, so a
// per-topic GET is the only way to obtain full topic objects. Topics that are
// deleted between the list call and their GET (404) are skipped.
func (c *Client) ListTopicsDetails(ctx context.Context, metalake, catalog, schema string) ([]models.Topic, error) {
	identifiers, err := c.ListTopics(ctx, metalake, catalog, schema)
	if err != nil {
		return nil, err
	}
	topics := make([]models.Topic, 0, len(identifiers.Identifiers))
	for _, id := range identifiers.Identifiers {
		resp, err := c.GetTopic(ctx, metalake, catalog, schema, id.Name)
		if err != nil {
			if IsNotFoundError(err) {
				continue
			}
			return nil, err
		}
		topics = append(topics, resp.Topic)
	}
	return topics, nil
}

func (c *Client) GetTopic(ctx context.Context, metalake, catalog, schema, name string) (*models.TopicResponse, error) {
	var result models.TopicResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/topics/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(name))
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) CreateTopic(ctx context.Context, metalake, catalog, schema string, req *models.TopicCreateRequest) (*models.TopicResponse, error) {
	var result models.TopicResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/topics", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema))
	err := c.Post(ctx, path, req, &result)
	return &result, err
}

func (c *Client) UpdateTopic(ctx context.Context, metalake, catalog, schema, name string, updates []interface{}) (*models.TopicResponse, error) {
	var result models.TopicResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/topics/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(name))
	err := c.Put(ctx, path, &models.TopicUpdateRequest{Updates: updates}, &result)
	return &result, err
}

func (c *Client) DropTopic(ctx context.Context, metalake, catalog, schema, name string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/topics/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(name))
	var result models.DropResponse
	err := c.Delete(ctx, path, &result)
	return &result, err
}
