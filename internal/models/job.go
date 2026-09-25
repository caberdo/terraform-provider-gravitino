package models

import "time"

// Job mirrors the Job object of the Gravitino v1.3.0 jobs API. A job is created
// by *running* a job template, so it is identified by a server-generated jobId
// and named after the template it runs; there is no user-supplied job name.
type Job struct {
	JobID           string     `json:"jobId,omitempty"`
	JobTemplateName string     `json:"jobTemplateName,omitempty"`
	Status          string     `json:"status,omitempty"`
	QueuedAt        *time.Time `json:"queuedAt,omitempty"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
	Audit           *Audit     `json:"audit,omitempty"`
}

type JobResponse struct {
	Code int `json:"code"`
	Job  Job `json:"job"`
}

type JobListResponse struct {
	Code int   `json:"code"`
	Jobs []Job `json:"jobs"`
}

// JobRunRequest is the body of POST /metalakes/{metalake}/jobs/runs.
type JobRunRequest struct {
	JobTemplateName string            `json:"jobTemplateName"`
	JobConf         map[string]string `json:"jobConf,omitempty"`
}

// JobTemplate mirrors the shell and spark job template objects. The API models
// them as two variants discriminated by jobType; Scripts only applies to shell
// templates, ClassName/Jars/Files/Archives/Configs only to spark templates.
type JobTemplate struct {
	Name         string            `json:"name"`
	JobType      string            `json:"jobType"`
	Comment      string            `json:"comment,omitempty"`
	Executable   string            `json:"executable,omitempty"`
	Arguments    []string          `json:"arguments,omitempty"`
	Environments map[string]string `json:"environments,omitempty"`
	CustomFields map[string]string `json:"customFields,omitempty"`
	Scripts      []string          `json:"scripts,omitempty"`
	ClassName    string            `json:"className,omitempty"`
	Jars         []string          `json:"jars,omitempty"`
	Files        []string          `json:"files,omitempty"`
	Archives     []string          `json:"archives,omitempty"`
	Configs      map[string]string `json:"configs,omitempty"`
	Audit        *Audit            `json:"audit,omitempty"`
}

type JobTemplateRegisterRequest struct {
	JobTemplate JobTemplate `json:"jobTemplate"`
}

type JobTemplateResponse struct {
	Code        int32       `json:"code"`
	JobTemplate JobTemplate `json:"jobTemplate"`
}

type JobTemplateListResponse struct {
	Code         int32         `json:"code"`
	JobTemplates []JobTemplate `json:"jobTemplates"`
}

// JobTemplateUpdateRequest carries the updates array of
// PUT /metalakes/{metalake}/jobs/templates/{jobTemplate}.
type JobTemplateUpdateRequest struct {
	Updates []interface{} `json:"updates"`
}

// NewJobTemplateRenameRequest renames a job template.
func NewJobTemplateRenameRequest(newName string) interface{} {
	return struct {
		Type    string `json:"@type"`
		NewName string `json:"newName"`
	}{Type: "rename", NewName: newName}
}

// NewJobTemplateUpdateCommentRequest updates a job template comment.
func NewJobTemplateUpdateCommentRequest(newComment string) interface{} {
	return struct {
		Type       string `json:"@type"`
		NewComment string `json:"newComment"`
	}{Type: "updateComment", NewComment: newComment}
}

// JobTemplateContentUpdate is the newTemplate payload of an updateTemplate
// request. Fields are pointers so unchanged settings are omitted from the
// request instead of being cleared.
type JobTemplateContentUpdate struct {
	Type            string             `json:"@type"`
	NewExecutable   *string            `json:"newExecutable,omitempty"`
	NewArguments    *[]string          `json:"newArguments,omitempty"`
	NewEnvironments *map[string]string `json:"newEnvironments,omitempty"`
	NewCustomFields *map[string]string `json:"newCustomFields,omitempty"`
	NewScripts      *[]string          `json:"newScripts,omitempty"`
	NewClassName    *string            `json:"newClassName,omitempty"`
	NewJars         *[]string          `json:"newJars,omitempty"`
	NewFiles        *[]string          `json:"newFiles,omitempty"`
	NewArchives     *[]string          `json:"newArchives,omitempty"`
	NewConfigs      *map[string]string `json:"newConfigs,omitempty"`
}

// NewJobTemplateUpdateContentRequest updates the content of a job template.
func NewJobTemplateUpdateContentRequest(update *JobTemplateContentUpdate) interface{} {
	return struct {
		Type        string                    `json:"@type"`
		NewTemplate *JobTemplateContentUpdate `json:"newTemplate"`
	}{Type: "updateTemplate", NewTemplate: update}
}
