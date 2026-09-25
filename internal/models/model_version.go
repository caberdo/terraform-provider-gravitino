package models

// ModelVersionURINameUnknown is the reserved URI name Gravitino uses for the
// unnamed URI of a model version (ModelVersion.URI_NAME_UNKNOWN). The `uri`
// field of a model version is the convenience view of uris[ModelVersionURINameUnknown].
const ModelVersionURINameUnknown = "unknown"

// ModelVersion mirrors the `ModelVersion` schema of the Gravitino v1.3.0
// OpenAPI spec. The version number is assigned by the server and the artifact
// locations live in URIs, keyed by URI name.
type ModelVersion struct {
	URI        string            `json:"uri,omitempty"`
	URIs       map[string]string `json:"uris,omitempty"`
	Version    int32             `json:"version"`
	Aliases    []string          `json:"aliases,omitempty"`
	Comment    string            `json:"comment,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
	Audit      *Audit            `json:"audit,omitempty"`
}

type ModelVersionResponse struct {
	Code         int32        `json:"code"`
	ModelVersion ModelVersion `json:"modelVersion"`
}

// ModelVersionLinkRequest mirrors the spec's `ModelVersionLinkRequest` schema.
// The request has no version field: Gravitino assigns the next version number
// when a version is linked to a model.
type ModelVersionLinkRequest struct {
	URI        string            `json:"uri,omitempty"`
	URIs       map[string]string `json:"uris,omitempty"`
	Aliases    []string          `json:"aliases,omitempty"`
	Comment    string            `json:"comment,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// ModelVersionListResponse covers both shapes of the `listModelVersions`
// response: `versions` (version numbers, returned when details is false) and
// `infos` (full model version objects, returned when details is true).
type ModelVersionListResponse struct {
	Code     int32          `json:"code"`
	Versions []int32        `json:"versions,omitempty"`
	Infos    []ModelVersion `json:"infos,omitempty"`
}

type ModelVersionURIResponse struct {
	Code int32  `json:"code"`
	URI  string `json:"uri"`
}

// ModelVersionUpdatesRequest mirrors the spec's `ModelVersionUpdatesRequest`
// schema. Gravitino only accepts the update types updateComment, setProperty,
// removeProperty, updateUri, addUri, removeUri and updateAliases.
type ModelVersionUpdatesRequest struct {
	Updates []interface{} `json:"updates"`
}

func NewUpdateModelVersionCommentRequest(newComment string) interface{} {
	return struct {
		Type       string `json:"@type"`
		NewComment string `json:"newComment"`
	}{
		Type:       "updateComment",
		NewComment: newComment,
	}
}

func NewSetModelVersionPropertyRequest(property, value string) interface{} {
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

func NewRemoveModelVersionPropertyRequest(property string) interface{} {
	return struct {
		Type     string `json:"@type"`
		Property string `json:"property"`
	}{
		Type:     "removeProperty",
		Property: property,
	}
}

// NewUpdateModelVersionURIRequest updates an existing URI of a model version.
// An empty uriName updates the unnamed URI of the model version.
func NewUpdateModelVersionURIRequest(newURI, uriName string) interface{} {
	return struct {
		Type    string `json:"@type"`
		NewURI  string `json:"newUri"`
		URIName string `json:"uriName,omitempty"`
	}{
		Type:    "updateUri",
		NewURI:  newURI,
		URIName: uriName,
	}
}

func NewAddModelVersionURIRequest(uriName, uri string) interface{} {
	return struct {
		Type    string `json:"@type"`
		URIName string `json:"uriName"`
		URI     string `json:"uri"`
	}{
		Type:    "addUri",
		URIName: uriName,
		URI:     uri,
	}
}

func NewRemoveModelVersionURIRequest(uriName string) interface{} {
	return struct {
		Type    string `json:"@type"`
		URIName string `json:"uriName"`
	}{
		Type:    "removeUri",
		URIName: uriName,
	}
}

// NewUpdateModelVersionAliasesRequest adds and removes aliases in a single
// update. Gravitino requires aliasesToAdd and aliasesToRemove to be present, so
// pass empty slices instead of nil.
func NewUpdateModelVersionAliasesRequest(aliasesToAdd, aliasesToRemove []string) interface{} {
	if aliasesToAdd == nil {
		aliasesToAdd = []string{}
	}
	if aliasesToRemove == nil {
		aliasesToRemove = []string{}
	}
	return struct {
		Type            string   `json:"@type"`
		AliasesToAdd    []string `json:"aliasesToAdd"`
		AliasesToRemove []string `json:"aliasesToRemove"`
	}{
		Type:            "updateAliases",
		AliasesToAdd:    aliasesToAdd,
		AliasesToRemove: aliasesToRemove,
	}
}
