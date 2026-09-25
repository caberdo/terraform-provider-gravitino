package models

// Metalake mirrors the `Metalake` schema of metalakes.yaml (Apache Gravitino v1.3.0).
type Metalake struct {
	Name       string            `json:"name"`
	Comment    string            `json:"comment,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
	Audit      *Audit            `json:"audit,omitempty"`
}

type MetalakeListResponse struct {
	Code      int        `json:"code"`
	Metalakes []Metalake `json:"metalakes"`
}

type MetalakeResponse struct {
	Code     int      `json:"code"`
	Metalake Metalake `json:"metalake"`
}

// MetalakeCreateRequest mirrors `MetalakeCreateRequest`.
type MetalakeCreateRequest struct {
	Name       string            `json:"name"`
	Comment    string            `json:"comment,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// MetalakeSetRequest mirrors `MetalakeSetRequest`, used by
// PATCH /metalakes/{metalake} to mark a metalake in-use (or not).
type MetalakeSetRequest struct {
	InUse bool `json:"inUse"`
}

// MetalakeUpdateRequest mirrors `MetalakeUpdatesRequest`.
type MetalakeUpdateRequest struct {
	Updates []interface{} `json:"updates"`
}

// RenameMetalakeRequest mirrors `RenameMetalakeRequest`.
type RenameMetalakeRequest struct {
	Type    string `json:"@type"`
	NewName string `json:"newName"`
}

// UpdateMetalakeCommentRequest mirrors `UpdateMetalakeCommentRequest`.
type UpdateMetalakeCommentRequest struct {
	Type       string `json:"@type"`
	NewComment string `json:"newComment"`
}

// SetMetalakePropertyRequest mirrors `SetMetalakePropertyRequest`.
type SetMetalakePropertyRequest struct {
	Type     string `json:"@type"`
	Property string `json:"property"`
	Value    string `json:"value"`
}

// RemoveMetalakePropertyRequest mirrors `RemoveMetalakePropertyRequest`.
type RemoveMetalakePropertyRequest struct {
	Type     string `json:"@type"`
	Property string `json:"property"`
}

func NewRenameMetalakeRequest(newName string) RenameMetalakeRequest {
	return RenameMetalakeRequest{Type: "rename", NewName: newName}
}

func NewUpdateMetalakeCommentRequest(newComment string) UpdateMetalakeCommentRequest {
	return UpdateMetalakeCommentRequest{Type: "updateComment", NewComment: newComment}
}

func NewSetMetalakePropertyRequest(property, value string) SetMetalakePropertyRequest {
	return SetMetalakePropertyRequest{Type: "setProperty", Property: property, Value: value}
}

func NewRemoveMetalakePropertyRequest(property string) RemoveMetalakePropertyRequest {
	return RemoveMetalakePropertyRequest{Type: "removeProperty", Property: property}
}
