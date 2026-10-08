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

// CatalogUpdateRequest mirrors `CatalogUpdatesRequest`. It is the update list
// wrapper used by PUT /metalakes/{metalake}/catalogs/{catalog} and, since
// Gravitino 1.3.1, the optional body of
// POST /metalakes/{metalake}/catalogs/{catalog}/testConnection.
type CatalogUpdateRequest struct {
	Updates []interface{} `json:"updates"`
}

// CatalogTestConnectionResponse mirrors the 200 response of the two connection
// test endpoints: POST /metalakes/{metalake}/catalogs/testConnection (v1.3.0)
// and POST /metalakes/{metalake}/catalogs/{catalog}/testConnection (v1.3.1).
//
// Expected test failures are reported inside an HTTP 200 response as an
// application `code` (1000-1100 range) plus `type` and `message`; only `code ==
// 0` means the connection test succeeded. Real HTTP failures (400/403/5xx) are
// returned by the client as an *HTTPError instead.
type CatalogTestConnectionResponse struct {
	Code    int      `json:"code"`
	Type    string   `json:"type,omitempty"`
	Message string   `json:"message,omitempty"`
	Stack   []string `json:"stack,omitempty"`
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
