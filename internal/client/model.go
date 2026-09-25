package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListModels(ctx context.Context, metalake, catalog, schema string) (*models.IdentifiersResponse, error) {
	var result models.IdentifiersResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/models", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema))
	err := c.Get(ctx, path, &result)
	return &result, err
}

// ListModelsDetails returns the full model objects of a schema. The list
// endpoint only returns identifiers, so every model is fetched individually.
func (c *Client) ListModelsDetails(ctx context.Context, metalake, catalog, schema string) ([]models.Model, error) {
	identifiers, err := c.ListModels(ctx, metalake, catalog, schema)
	if err != nil {
		return nil, err
	}
	mods := make([]models.Model, 0, len(identifiers.Identifiers))
	for _, id := range identifiers.Identifiers {
		resp, err := c.GetModel(ctx, metalake, catalog, schema, id.Name)
		if err != nil {
			return nil, err
		}
		mods = append(mods, resp.Model)
	}
	return mods, nil
}

func (c *Client) GetModel(ctx context.Context, metalake, catalog, schema, model string) (*models.ModelResponse, error) {
	var result models.ModelResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/models/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(model))
	err := c.Get(ctx, path, &result)
	return &result, err
}

func (c *Client) CreateModel(ctx context.Context, metalake, catalog, schema string, req *models.ModelRegisterRequest) (*models.ModelResponse, error) {
	var result models.ModelResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/models", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema))
	err := c.Post(ctx, path, req, &result)
	return &result, err
}

func (c *Client) UpdateModel(ctx context.Context, metalake, catalog, schema, model string, updates []interface{}) (*models.ModelResponse, error) {
	var result models.ModelResponse
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/models/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(model))
	err := c.Put(ctx, path, &models.ModelUpdatesRequest{Updates: updates}, &result)
	return &result, err
}

func (c *Client) DropModel(ctx context.Context, metalake, catalog, schema, model string) (*models.DropResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/models/%s", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(model))
	var result models.DropResponse
	err := c.Delete(ctx, path, &result)
	return &result, err
}

// ListModelVersions lists the model versions of a model. When details is true
// Gravitino answers with the full model version objects (`infos`), otherwise it
// only returns the version numbers (`versions`).
func (c *Client) ListModelVersions(ctx context.Context, metalake, catalog, schema, model string, details bool) (*models.ModelVersionListResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/models/%s/versions", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(model))
	if details {
		path += "?" + url.Values{"details": []string{"true"}}.Encode()
	}
	var result models.ModelVersionListResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// LinkModelVersion links a new model version to a model. Gravitino assigns the
// version number and answers with a plain BaseResponse, so the assigned number
// has to be read back with ListModelVersions.
func (c *Client) LinkModelVersion(ctx context.Context, metalake, catalog, schema, model string, req *models.ModelVersionLinkRequest) (*models.BaseResponse, error) {
	path := fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/models/%s/versions", url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(model))
	var result models.BaseResponse
	if err := c.Post(ctx, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func modelVersionPath(metalake, catalog, schema, model string, version int32) string {
	return fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/models/%s/versions/%s",
		url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(model), strconv.FormatInt(int64(version), 10))
}

func modelVersionAliasPath(metalake, catalog, schema, model, alias string) string {
	return fmt.Sprintf("/metalakes/%s/catalogs/%s/schemas/%s/models/%s/aliases/%s",
		url.PathEscape(metalake), url.PathEscape(catalog), url.PathEscape(schema), url.PathEscape(model), url.PathEscape(alias))
}

func (c *Client) GetModelVersion(ctx context.Context, metalake, catalog, schema, model string, version int32) (*models.ModelVersionResponse, error) {
	var result models.ModelVersionResponse
	if err := c.Get(ctx, modelVersionPath(metalake, catalog, schema, model, version), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdateModelVersion(ctx context.Context, metalake, catalog, schema, model string, version int32, updates []interface{}) (*models.ModelVersionResponse, error) {
	var result models.ModelVersionResponse
	if err := c.Put(ctx, modelVersionPath(metalake, catalog, schema, model, version), &models.ModelVersionUpdatesRequest{Updates: updates}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteModelVersion(ctx context.Context, metalake, catalog, schema, model string, version int32) (*models.DropResponse, error) {
	var result models.DropResponse
	if err := c.Delete(ctx, modelVersionPath(metalake, catalog, schema, model, version), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetModelVersionByAlias(ctx context.Context, metalake, catalog, schema, model, alias string) (*models.ModelVersionResponse, error) {
	var result models.ModelVersionResponse
	if err := c.Get(ctx, modelVersionAliasPath(metalake, catalog, schema, model, alias), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdateModelVersionByAlias(ctx context.Context, metalake, catalog, schema, model, alias string, updates []interface{}) (*models.ModelVersionResponse, error) {
	var result models.ModelVersionResponse
	if err := c.Put(ctx, modelVersionAliasPath(metalake, catalog, schema, model, alias), &models.ModelVersionUpdatesRequest{Updates: updates}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteModelVersionByAlias(ctx context.Context, metalake, catalog, schema, model, alias string) (*models.DropResponse, error) {
	var result models.DropResponse
	if err := c.Delete(ctx, modelVersionAliasPath(metalake, catalog, schema, model, alias), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetModelVersionURI returns the URI of a model version. An empty uriName
// returns the unnamed URI of the model version.
func (c *Client) GetModelVersionURI(ctx context.Context, metalake, catalog, schema, model string, version int32, uriName string) (*models.ModelVersionURIResponse, error) {
	return c.getModelVersionURI(ctx, modelVersionPath(metalake, catalog, schema, model, version)+"/uri", uriName)
}

// GetModelVersionURIByAlias returns the URI of the model version a model alias
// points to. An empty uriName returns the unnamed URI of the model version.
func (c *Client) GetModelVersionURIByAlias(ctx context.Context, metalake, catalog, schema, model, alias, uriName string) (*models.ModelVersionURIResponse, error) {
	return c.getModelVersionURI(ctx, modelVersionAliasPath(metalake, catalog, schema, model, alias)+"/uri", uriName)
}

func (c *Client) getModelVersionURI(ctx context.Context, path, uriName string) (*models.ModelVersionURIResponse, error) {
	if uriName != "" {
		path += "?" + url.Values{"uriName": []string{uriName}}.Encode()
	}
	var result models.ModelVersionURIResponse
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
