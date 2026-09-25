package models

// Catalog mirrors the `Catalog` schema of catalogs.yaml (Apache Gravitino v1.3.0).
type Catalog struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Provider   string            `json:"provider"`
	Comment    string            `json:"comment,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
	Audit      *Audit            `json:"audit,omitempty"`
}

type CatalogListResponse struct {
	Code        int              `json:"code"`
	Identifiers []NameIdentifier `json:"identifiers"`
}

type CatalogInfoListResponse struct {
	Code     int       `json:"code"`
	Catalogs []Catalog `json:"catalogs"`
}

type CatalogResponse struct {
	Code    int     `json:"code"`
	Catalog Catalog `json:"catalog"`
}

// CatalogCreateRequest mirrors `CatalogCreateRequest`. `type` is required, the
// provider is optional for fileset and model catalogs.
type CatalogCreateRequest struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Provider   string            `json:"provider,omitempty"`
	Comment    string            `json:"comment,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// CatalogSetRequest mirrors `CatalogSetRequest`, used by
// PATCH /metalakes/{metalake}/catalogs/{catalog} to mark a catalog in-use (or not).
type CatalogSetRequest struct {
	InUse bool `json:"inUse"`
}

// CatalogUpdateRequest mirrors `CatalogUpdatesRequest`.
type CatalogUpdateRequest struct {
	Updates []interface{} `json:"updates"`
}

// RenameCatalogRequest mirrors `RenameCatalogRequest`.
type RenameCatalogRequest struct {
	Type    string `json:"@type"`
	NewName string `json:"newName"`
}

// UpdateCatalogCommentRequest mirrors `UpdateCatalogCommentRequest`.
type UpdateCatalogCommentRequest struct {
	Type       string `json:"@type"`
	NewComment string `json:"newComment"`
}

// SetCatalogPropertyRequest mirrors `SetCatalogPropertyRequest`.
type SetCatalogPropertyRequest struct {
	Type     string `json:"@type"`
	Property string `json:"property"`
	Value    string `json:"value"`
}

// RemoveCatalogPropertyRequest mirrors `RemoveCatalogPropertyRequest`.
type RemoveCatalogPropertyRequest struct {
	Type     string `json:"@type"`
	Property string `json:"property"`
}

func NewRenameCatalogRequest(newName string) RenameCatalogRequest {
	return RenameCatalogRequest{Type: "rename", NewName: newName}
}

func NewUpdateCatalogCommentRequest(newComment string) UpdateCatalogCommentRequest {
	return UpdateCatalogCommentRequest{Type: "updateComment", NewComment: newComment}
}

func NewSetCatalogPropertyRequest(property, value string) SetCatalogPropertyRequest {
	return SetCatalogPropertyRequest{Type: "setProperty", Property: property, Value: value}
}

func NewRemoveCatalogPropertyRequest(property string) RemoveCatalogPropertyRequest {
	return RemoveCatalogPropertyRequest{Type: "removeProperty", Property: property}
}
