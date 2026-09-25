package models

import "fmt"

// Column mirrors tables.yaml#/Column.
//
// nullable and autoIncrement are always serialised because the spec defaults
// them to true and false respectively: omitting "nullable" would let a catalog
// default a NOT NULL column back to nullable.
type Column struct {
	Name          string   `json:"name"`
	Type          DataType `json:"type"`
	Comment       string   `json:"comment,omitempty"`
	Nullable      bool     `json:"nullable"`
	AutoIncrement bool     `json:"autoIncrement"`
	DefaultValue  *Literal `json:"defaultValue,omitempty"`
}

// SortOrder mirrors tables.yaml#/SortOrder.
type SortOrder struct {
	SortTerm     Expression `json:"sortTerm"`
	Direction    string     `json:"direction,omitempty"`
	NullOrdering string     `json:"nullOrdering,omitempty"`
}

// Distribution mirrors tables.yaml#/Distribution.
type Distribution struct {
	Strategy string       `json:"strategy,omitempty"`
	Number   int32        `json:"number"`
	FuncArgs []Expression `json:"funcArgs"`
}

// Partitioning mirrors partitioning.yaml#/PartitioningSpec. Every partition
// strategy is expressed with the same field set; only the fields required by
// the strategy are serialised.
type Partitioning struct {
	Strategy   string       `json:"strategy"`
	FieldName  []string     `json:"fieldName,omitempty"`
	FieldNames [][]string   `json:"fieldNames,omitempty"`
	NumBuckets *int         `json:"numBuckets,omitempty"`
	Width      *int         `json:"width,omitempty"`
	FuncName   string       `json:"funcName,omitempty"`
	FuncArgs   []Expression `json:"funcArgs,omitempty"`
}

// Index mirrors indexes.yaml#/IndexSpec.
type Index struct {
	IndexType  string     `json:"indexType"`
	Name       string     `json:"name,omitempty"`
	FieldNames [][]string `json:"fieldNames"`
}

// Table mirrors tables.yaml#/Table.
type Table struct {
	Name         string            `json:"name"`
	Columns      []Column          `json:"columns"`
	Comment      string            `json:"comment,omitempty"`
	Audit        *Audit            `json:"audit,omitempty"`
	Properties   map[string]string `json:"properties,omitempty"`
	Distribution *Distribution     `json:"distribution,omitempty"`
	SortOrders   []SortOrder       `json:"sortOrders,omitempty"`
	Partitioning []Partitioning    `json:"partitioning,omitempty"`
	Indexes      []Index           `json:"indexes,omitempty"`
}

// TableResponse mirrors the TableResponse schema of tables.yaml.
type TableResponse struct {
	Code  int   `json:"code"`
	Table Table `json:"table"`
}

// TableCreateRequest mirrors tables.yaml#/TableCreateRequest.
type TableCreateRequest struct {
	Name         string            `json:"name"`
	Columns      []Column          `json:"columns"`
	Comment      string            `json:"comment,omitempty"`
	Properties   map[string]string `json:"properties,omitempty"`
	SortOrders   []SortOrder       `json:"sortOrders,omitempty"`
	Distribution *Distribution     `json:"distribution,omitempty"`
	Partitioning []Partitioning    `json:"partitioning,omitempty"`
	Indexes      []Index           `json:"indexes,omitempty"`
}

// Table update types of tables.yaml#/TableUpdateRequest.
const (
	TableUpdateRename             = "rename"
	TableUpdateComment            = "updateComment"
	TableUpdateSetProperty        = "setProperty"
	TableUpdateRemoveProperty     = "removeProperty"
	TableUpdateColumnType         = "updateColumnType"
	TableUpdateColumnComment      = "updateColumnComment"
	TableUpdateColumnPosition     = "updateColumnPosition"
	TableUpdateColumnNullability  = "updateColumnNullability"
	TableUpdateColumnDefaultValue = "updateColumnDefaultValue"
)

// TableUpdatesRequest mirrors tables.yaml#/TableUpdatesRequest, the body of the
// PUT /tables/{table} operation.
type TableUpdatesRequest struct {
	Updates []TableUpdateRequest `json:"updates"`
}

// TableUpdateRequest mirrors tables.yaml#/TableUpdateRequest.
//
// Only the update types this provider supports are modelled. A field of the
// union is serialised only for the update types that define it, so the request
// body carries exactly the members of the matching schema.
type TableUpdateRequest struct {
	Type string `json:"@type"`

	// rename
	NewName       string `json:"newName,omitempty"`
	NewSchemaName string `json:"newSchemaName,omitempty"`

	// updateComment, updateColumnComment
	NewComment string `json:"newComment,omitempty"`

	// setProperty, removeProperty
	Property string `json:"property,omitempty"`
	Value    string `json:"value,omitempty"`

	// updateColumnType, updateColumnComment, updateColumnPosition,
	// updateColumnNullability, updateColumnDefaultValue
	FieldName []string `json:"fieldName,omitempty"`

	// updateColumnType
	NewType *DataType `json:"newType,omitempty"`

	// updateColumnPosition
	NewPosition *ColumnPosition `json:"newPosition,omitempty"`

	// updateColumnNullability
	Nullable *bool `json:"nullable,omitempty"`

	// updateColumnDefaultValue; a nil value clears the default value.
	NewDefaultValue *Literal `json:"newDefaultValue"`
}

// MarshalJSON serialises only the fields defined for the update type.
func (u TableUpdateRequest) MarshalJSON() ([]byte, error) {
	body := map[string]interface{}{"@type": u.Type}

	switch u.Type {
	case TableUpdateRename:
		body["newName"] = u.NewName
		if u.NewSchemaName != "" {
			body["newSchemaName"] = u.NewSchemaName
		}
	case TableUpdateComment:
		body["newComment"] = u.NewComment
	case TableUpdateSetProperty:
		body["property"] = u.Property
		body["value"] = u.Value
	case TableUpdateRemoveProperty:
		body["property"] = u.Property
	case TableUpdateColumnType:
		body["fieldName"] = u.FieldName
		body["newType"] = u.NewType
	case TableUpdateColumnComment:
		body["fieldName"] = u.FieldName
		body["newComment"] = u.NewComment
	case TableUpdateColumnPosition:
		body["fieldName"] = u.FieldName
		body["newPosition"] = u.NewPosition
	case TableUpdateColumnNullability:
		body["fieldName"] = u.FieldName
		body["nullable"] = u.Nullable
	case TableUpdateColumnDefaultValue:
		body["fieldName"] = u.FieldName
		body["newDefaultValue"] = u.NewDefaultValue
	default:
		return nil, fmt.Errorf("unsupported table update type %q", u.Type)
	}

	return marshalCompactJSON(body)
}

// NewRenameTableRequest builds a "rename" table update.
func NewRenameTableRequest(newName string) TableUpdateRequest {
	return TableUpdateRequest{Type: TableUpdateRename, NewName: newName}
}

// NewUpdateTableCommentRequest builds an "updateComment" table update.
func NewUpdateTableCommentRequest(newComment string) TableUpdateRequest {
	return TableUpdateRequest{Type: TableUpdateComment, NewComment: newComment}
}

// NewSetTablePropertyRequest builds a "setProperty" table update.
func NewSetTablePropertyRequest(property, value string) TableUpdateRequest {
	return TableUpdateRequest{Type: TableUpdateSetProperty, Property: property, Value: value}
}

// NewRemoveTablePropertyRequest builds a "removeProperty" table update.
func NewRemoveTablePropertyRequest(property string) TableUpdateRequest {
	return TableUpdateRequest{Type: TableUpdateRemoveProperty, Property: property}
}

// NewUpdateTableColumnTypeRequest builds an "updateColumnType" table update.
func NewUpdateTableColumnTypeRequest(fieldName []string, newType DataType) TableUpdateRequest {
	return TableUpdateRequest{Type: TableUpdateColumnType, FieldName: fieldName, NewType: &newType}
}

// NewUpdateTableColumnCommentRequest builds an "updateColumnComment" update.
func NewUpdateTableColumnCommentRequest(fieldName []string, newComment string) TableUpdateRequest {
	return TableUpdateRequest{Type: TableUpdateColumnComment, FieldName: fieldName, NewComment: newComment}
}

// NewUpdateTableColumnPositionRequest builds an "updateColumnPosition" update.
func NewUpdateTableColumnPositionRequest(fieldName []string, position ColumnPosition) TableUpdateRequest {
	return TableUpdateRequest{Type: TableUpdateColumnPosition, FieldName: fieldName, NewPosition: &position}
}

// NewUpdateTableColumnNullabilityRequest builds an "updateColumnNullability"
// table update.
func NewUpdateTableColumnNullabilityRequest(fieldName []string, nullable bool) TableUpdateRequest {
	return TableUpdateRequest{Type: TableUpdateColumnNullability, FieldName: fieldName, Nullable: &nullable}
}

// NewUpdateTableColumnDefaultValueRequest builds an "updateColumnDefaultValue"
// table update. A nil default value clears the column default.
func NewUpdateTableColumnDefaultValueRequest(fieldName []string, defaultValue *Literal) TableUpdateRequest {
	return TableUpdateRequest{Type: TableUpdateColumnDefaultValue, FieldName: fieldName, NewDefaultValue: defaultValue}
}
