package client

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListJobs(ctx context.Context, metalake string) (*models.JobListResponse, error) {
	var result models.JobListResponse
	err := c.Get(ctx, jobRunsPath(metalake), &result)
	return &result, err
}

func (c *Client) GetJob(ctx context.Context, metalake, jobID string) (*models.JobResponse, error) {
	var result models.JobResponse
	err := c.Get(ctx, jobRunPath(metalake, jobID), &result)
	return &result, err
}

// RunJob starts a job run for the given template. The server assigns the jobId.
func (c *Client) RunJob(ctx context.Context, metalake string, req *models.JobRunRequest) (*models.JobResponse, error) {
	var result models.JobResponse
	err := c.Post(ctx, jobRunsPath(metalake), req, &result)
	return &result, err
}

// CancelJob cancels a queued or running job. Gravitino offers no way to delete a
// job record; cancelling is the only terminal operation.
func (c *Client) CancelJob(ctx context.Context, metalake, jobID string) (*models.JobResponse, error) {
	var result models.JobResponse
	err := c.Post(ctx, jobRunPath(metalake, jobID), nil, &result)
	return &result, err
}

func (c *Client) ListJobTemplates(ctx context.Context, metalake string) (*models.JobTemplateListResponse, error) {
	var result models.JobTemplateListResponse
	err := c.Get(ctx, jobTemplatesPath(metalake)+"?details=true", &result)
	return &result, err
}

// RegisterJobTemplate registers a template. The API answers with a bare
// BaseResponse, so the caller has to read the template back to learn its audit
// information.
func (c *Client) RegisterJobTemplate(ctx context.Context, metalake string, req *models.JobTemplateRegisterRequest) (*models.BaseResponse, error) {
	var result models.BaseResponse
	err := c.Post(ctx, jobTemplatesPath(metalake), req, &result)
	return &result, err
}

func (c *Client) GetJobTemplate(ctx context.Context, metalake, name string) (*models.JobTemplateResponse, error) {
	var result models.JobTemplateResponse
	err := c.Get(ctx, jobTemplatePath(metalake, name), &result)
	return &result, err
}

func (c *Client) UpdateJobTemplate(ctx context.Context, metalake, name string, updates []interface{}) (*models.JobTemplateResponse, error) {
	var result models.JobTemplateResponse
	path := jobTemplatePath(metalake, name)
	err := c.Put(ctx, path, &models.JobTemplateUpdateRequest{Updates: updates}, &result)
	return &result, err
}

func (c *Client) DeleteJobTemplate(ctx context.Context, metalake, name string) (*models.DropResponse, error) {
	var result models.DropResponse
	path := jobTemplatePath(metalake, name)
	err := c.Delete(ctx, path, &result)
	return &result, err
}
