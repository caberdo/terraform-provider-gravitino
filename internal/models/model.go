package models

// Model mirrors the `Model` schema of the Gravitino v1.3.0 OpenAPI spec
// (docs/open-api/models.yaml). A model does not carry a URI: model artifacts are
// attached to a model version (see ModelVersion.URIs).
type Model struct {
	Name          string            `json:"name"`
	LatestVersion int32             `json:"latestVersion"`
	Comment       string            `json:"comment,omitempty"`
	Properties    map[string]string `json:"properties,omitempty"`
	Audit         *Audit            `json:"audit,omitempty"`
}

type ModelResponse struct {
	Code  int   `json:"code"`
	Model Model `json:"model"`
}

// ModelRegisterRequest mirrors the spec's `ModelRegisterRequest` schema.
type ModelRegisterRequest struct {
	Name       string            `json:"name"`
	Comment    string            `json:"comment,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// ModelUpdatesRequest mirrors the spec's `ModelUpdatesRequest` schema. Gravitino
// only accepts the update types rename, setProperty, removeProperty and
// updateComment.
type ModelUpdatesRequest struct {
	Updates []interface{} `json:"updates"`
}

func NewRenameModelRequest(newName string) interface{} {
	return struct {
		Type    string `json:"@type"`
		NewName string `json:"newName"`
	}{
		Type:    "rename",
		NewName: newName,
	}
}

func NewUpdateModelCommentRequest(newComment string) interface{} {
	return struct {
		Type       string `json:"@type"`
		NewComment string `json:"newComment"`
	}{
		Type:       "updateComment",
		NewComment: newComment,
	}
}

func NewSetModelPropertyRequest(property, value string) interface{} {
	return struct {
		Type     string `json:"@type"`
		Property string `json:"property"`
		Value    string `json:"value"`
	}{
		Type:     "setProperty",
		Property: property,
		Value:    value,
	}
}

func NewRemoveModelPropertyRequest(property string) interface{} {
	return struct {
		Type     string `json:"@type"`
		Property string `json:"property"`
	}{
		Type:     "removeProperty",
		Property: property,
	}
}
