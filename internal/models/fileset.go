package models

// Fileset mirrors the `Fileset` component of the Gravitino v1.3.0 OpenAPI spec
// (docs/open-api/filesets.yaml).
type Fileset struct {
	Name            string            `json:"name"`
	Comment         string            `json:"comment,omitempty"`
	Type            string            `json:"type,omitempty"`
	StorageLocation string            `json:"storageLocation,omitempty"`
	Properties      map[string]string `json:"properties,omitempty"`
	Audit           *Audit            `json:"audit,omitempty"`
}

type FilesetResponse struct {
	Code    int     `json:"code"`
	Fileset Fileset `json:"fileset"`
}

// FilesetCreateRequest mirrors `FilesetCreateRequest`: name (required), type,
// comment, storageLocation, storageLocations and properties.
//
// `storageLocations` is intentionally not modelled: the spec's `Fileset`
// response does not return it, so it could never be reconciled into Terraform
// state and is therefore not exposed by the provider.
type FilesetCreateRequest struct {
	Name            string            `json:"name"`
	Comment         string            `json:"comment,omitempty"`
	Type            string            `json:"type,omitempty"`
	StorageLocation string            `json:"storageLocation,omitempty"`
	Properties      map[string]string `json:"properties,omitempty"`
}

// FilesetUpdateRequest mirrors `FilesetUpdatesRequest`.
type FilesetUpdateRequest struct {
	Updates []interface{} `json:"updates"`
}

// NewRenameFilesetRequest builds a `RenameFilesetRequest`.
func NewRenameFilesetRequest(newName string) interface{} {
	return struct {
		Type    string `json:"@type"`
		NewName string `json:"newName"`
	}{Type: "rename", NewName: newName}
}

// NewUpdateFilesetCommentRequest builds an `UpdateFilesetCommentRequest`.
func NewUpdateFilesetCommentRequest(newComment string) interface{} {
	return struct {
		Type       string `json:"@type"`
		NewComment string `json:"newComment"`
	}{Type: "updateComment", NewComment: newComment}
}

// NewRemoveFilesetCommentRequest builds a `RemoveFilesetCommentRequest`.
func NewRemoveFilesetCommentRequest() interface{} {
	return struct {
		Type string `json:"@type"`
	}{Type: "removeComment"}
}

// NewSetFilesetPropertyRequest builds a `SetFilesetPropertyRequest`.
func NewSetFilesetPropertyRequest(property, value string) interface{} {
	return struct {
		Type     string `json:"@type"`
		Property string `json:"property"`
		Value    string `json:"value"`
	}{Type: "setProperty", Property: property, Value: value}
}

// NewRemoveFilesetPropertyRequest builds a `RemoveFilesetPropertyRequest`.
func NewRemoveFilesetPropertyRequest(property string) interface{} {
	return struct {
		Type     string `json:"@type"`
		Property string `json:"property"`
	}{Type: "removeProperty", Property: property}
}

// FilesetFile mirrors the `FileInfo` component returned by the
// `listFilesetFiles` operation.
type FilesetFile struct {
	Name         string `json:"name"`
	IsDir        bool   `json:"isDir"`
	Size         int64  `json:"size"`
	LastModified int64  `json:"lastModified"`
	Path         string `json:"path"`
}

type FilesetFileListResponse struct {
	Code  int32         `json:"code"`
	Files []FilesetFile `json:"files"`
}
