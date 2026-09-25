package client

import (
	"context"
	"net/url"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

func (c *Client) ListJobs(ctx context.Context, metalake string) (*models.JobListResponse, error) {
	var result models.JobListResponse
	err := c.Get(ctx, "/metalakes/"+url.PathEscape(metalake)+"/jobs/runs", &result)
	return &result, err
}

func (c *Client) GetJob(ctx context.Context, metalake, jobID string) (*models.JobResponse, error) {
	var result models.JobResponse
	err := c.Get(ctx, "/metalakes/"+url.PathEscape(metalake)+"/jobs/runs/"+url.PathEscape(jobID), &result)
	return &result, err
}

// RunJob starts a job run for the given template. The server assigns the jobId.
func (c *Client) RunJob(ctx context.Context, metalake string, req *models.JobRunRequest) (*models.JobResponse, error) {
	var result models.JobResponse
	err := c.Post(ctx, "/metalakes/"+url.PathEscape(metalake)+"/jobs/runs", req, &result)
	return &result, err
}

// CancelJob cancels a queued or running job. Gravitino offers no way to delete a
// job record; cancelling is the only terminal operation.
func (c *Client) CancelJob(ctx context.Context, metalake, jobID string) (*models.JobResponse, error) {
	var result models.JobResponse
	err := c.Post(ctx, "/metalakes/"+url.PathEscape(metalake)+"/jobs/runs/"+url.PathEscape(jobID), nil, &result)
	return &result, err
}

func (c *Client) ListJobTemplates(ctx context.Context, metalake string) (*models.JobTemplateListResponse, error) {
	var result models.JobTemplateListResponse
	err := c.Get(ctx, "/metalakes/"+url.PathEscape(metalake)+"/jobs/templates?details=true", &result)
	return &result, err
}

// RegisterJobTemplate registers a template. The API answers with a bare
// BaseResponse, so the caller has to read the template back to learn its audit
// information.
func (c *Client) RegisterJobTemplate(ctx context.Context, metalake string, req *models.JobTemplateRegisterRequest) (*models.BaseResponse, error) {
	var result models.BaseResponse
	err := c.Post(ctx, "/metalakes/"+url.PathEscape(metalake)+"/jobs/templates", req, &result)
	return &result, err
}

func (c *Client) GetJobTemplate(ctx context.Context, metalake, name string) (*models.JobTemplateResponse, error) {
	var result models.JobTemplateResponse
	err := c.Get(ctx, "/metalakes/"+url.PathEscape(metalake)+"/jobs/templates/"+url.PathEscape(name), &result)
	return &result, err
}

func (c *Client) UpdateJobTemplate(ctx context.Context, metalake, name string, updates []interface{}) (*models.JobTemplateResponse, error) {
	var result models.JobTemplateResponse
	path := "/metalakes/" + url.PathEscape(metalake) + "/jobs/templates/" + url.PathEscape(name)
	err := c.Put(ctx, path, &models.JobTemplateUpdateRequest{Updates: updates}, &result)
	return &result, err
}

func (c *Client) DeleteJobTemplate(ctx context.Context, metalake, name string) (*models.DropResponse, error) {
	var result models.DropResponse
	path := "/metalakes/" + url.PathEscape(metalake) + "/jobs/templates/" + url.PathEscape(name)
	err := c.Delete(ctx, path, &result)
	return &result, err
}
