package models

// Schema mirrors the `Schema` component of the Gravitino v1.3.0 OpenAPI spec
// (docs/open-api/schemas.yaml).
type Schema struct {
	Name       string            `json:"name"`
	Comment    string            `json:"comment,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
	Audit      *Audit            `json:"audit,omitempty"`
}

type SchemaResponse struct {
	Code   int    `json:"code"`
	Schema Schema `json:"schema"`
}

// SchemaCreateRequest mirrors `SchemaCreateRequest`: name (required), comment
// and properties. The spec defines no other create fields.
type SchemaCreateRequest struct {
	Name       string            `json:"name"`
	Comment    string            `json:"comment,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// SchemaUpdateRequest mirrors `SchemaUpdatesRequest`. The spec's
// `SchemaUpdateRequest` oneOf only allows SetSchemaPropertyRequest and
// RemoveSchemaPropertyRequest, so there is deliberately no rename/comment
// update here: those schema fields are not updateable through the REST API.
type SchemaUpdateRequest struct {
	Updates []interface{} `json:"updates"`
}

// NewSetSchemaPropertyRequest builds a `SetSchemaPropertyRequest`.
func NewSetSchemaPropertyRequest(property, value string) interface{} {
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

// NewRemoveSchemaPropertyRequest builds a `RemoveSchemaPropertyRequest`.
func NewRemoveSchemaPropertyRequest(property string) interface{} {
	return struct {
		Type     string `json:"@type"`
		Property string `json:"property"`
	}{
		Type:     "removeProperty",
		Property: property,
	}
}
