package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Structured data type kinds from datatype.yaml#/DataType.
const (
	DataTypeStruct   = "struct"
	DataTypeList     = "list"
	DataTypeMap      = "map"
	DataTypeUnion    = "union"
	DataTypeUnparsed = "unparsed"
)

// LiteralType is the discriminator value of expression.yaml#/Literal.
const LiteralType = "literal"

// Column position modes of tables.yaml#/ColumnPosition.
const (
	ColumnPositionFirst   = "first"
	ColumnPositionAfter   = "after"
	ColumnPositionDefault = "default"
)

// primitiveTypePattern accepts the PrimitiveType names documented in
// datatype.yaml, including their parameterised form ("varchar(255)",
// "decimal(10,2)", "byte unsigned", "timestamp(3)").
var primitiveTypePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_ ]*(\([0-9]+(,[0-9]+)?\))?$`)

// DataType mirrors datatype.yaml#/DataType.
//
// Gravitino serialises a primitive type as a bare JSON string ("integer",
// "varchar(255)") and the structured kinds as JSON objects, so DataType
// implements marshalling and unmarshalling for both shapes.
type DataType struct {
	// Type is either a primitive type name or one of the structured kinds.
	Type string `json:"-"`

	// Fields holds the struct fields of a "struct" type.
	Fields []StructField `json:"-"`

	// ContainsNull and ElementType describe a "list" type.
	ContainsNull *bool     `json:"-"`
	ElementType  *DataType `json:"-"`

	// KeyType, ValueType and ValueContainsNull describe a "map" type.
	KeyType           *DataType `json:"-"`
	ValueType         *DataType `json:"-"`
	ValueContainsNull *bool     `json:"-"`

	// Types holds the member types of a "union" type.
	Types []DataType `json:"-"`

	// UnparsedType holds the opaque catalog type of an "unparsed" type.
	UnparsedType string `json:"-"`
}

// IsStructured reports whether the type is one of the object shaped kinds.
func (d DataType) IsStructured() bool {
	switch d.Type {
	case DataTypeStruct, DataTypeList, DataTypeMap, DataTypeUnion, DataTypeUnparsed:
		return true
	}
	return false
}

// MarshalJSON encodes primitives as a JSON string and structured types as a
// JSON object with alphabetically ordered keys, matching the output of HCL's
// jsonencode so a type written as jsonencode({...}) round-trips unchanged.
func (d DataType) MarshalJSON() ([]byte, error) {
	if !d.IsStructured() {
		return marshalCompactJSON(d.Type)
	}

	obj := map[string]interface{}{"type": d.Type}
	switch d.Type {
	case DataTypeStruct:
		obj["fields"] = d.Fields
	case DataTypeList:
		obj["elementType"] = d.ElementType
		if d.ContainsNull != nil {
			obj["containsNull"] = *d.ContainsNull
		}
	case DataTypeMap:
		obj["keyType"] = d.KeyType
		obj["valueType"] = d.ValueType
		if d.ValueContainsNull != nil {
			obj["valueContainsNull"] = *d.ValueContainsNull
		}
	case DataTypeUnion:
		obj["types"] = d.Types
	case DataTypeUnparsed:
		obj["unparsedType"] = d.UnparsedType
	}

	return marshalCompactJSON(obj)
}

// UnmarshalJSON decodes both the string form (primitive types) and the object
// form (structured types) of datatype.yaml#/DataType.
func (d *DataType) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*d = DataType{}
		return nil
	}

	if trimmed[0] == '"' {
		var name string
		if err := json.Unmarshal(trimmed, &name); err != nil {
			return err
		}
		*d = DataType{Type: name}
		return nil
	}

	var raw struct {
		Type              string          `json:"type"`
		Fields            []StructField   `json:"fields"`
		ContainsNull      *bool           `json:"containsNull"`
		ElementType       *DataType       `json:"elementType"`
		KeyType           *DataType       `json:"keyType"`
		ValueType         *DataType       `json:"valueType"`
		ValueContainsNull *bool           `json:"valueContainsNull"`
		Types             []DataType      `json:"types"`
		UnparsedType      json.RawMessage `json:"unparsedType"`
	}
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return err
	}

	if raw.Type == "" {
		return fmt.Errorf("data type object is missing the required %q field", "type")
	}

	*d = DataType{
		Type:              raw.Type,
		Fields:            raw.Fields,
		ContainsNull:      raw.ContainsNull,
		ElementType:       raw.ElementType,
		KeyType:           raw.KeyType,
		ValueType:         raw.ValueType,
		ValueContainsNull: raw.ValueContainsNull,
		Types:             raw.Types,
		UnparsedType:      decodeUnparsedType(raw.UnparsedType),
	}

	return nil
}

// String returns the canonical Terraform representation of the type: the bare
// primitive name, or the structured type as compact JSON.
func (d DataType) String() string {
	if d.Type == "" {
		return ""
	}
	if !d.IsStructured() {
		return d.Type
	}

	encoded, err := marshalCompactJSON(d)
	if err != nil {
		return d.Type
	}
	return string(encoded)
}

// Validate reports whether the type is complete enough to be sent to Gravitino.
func (d DataType) Validate() error {
	if strings.TrimSpace(d.Type) == "" {
		return fmt.Errorf("data type is missing the required %q field", "type")
	}
	if !d.IsStructured() {
		return nil
	}

	switch d.Type {
	case DataTypeStruct:
		if len(d.Fields) == 0 {
			return fmt.Errorf("data type %q requires at least one entry in %q", d.Type, "fields")
		}
		for i, field := range d.Fields {
			if strings.TrimSpace(field.Name) == "" {
				return fmt.Errorf("data type %q: field %d is missing a name", d.Type, i)
			}
			if err := field.Type.Validate(); err != nil {
				return fmt.Errorf("data type %q: field %q: %w", d.Type, field.Name, err)
			}
		}
	case DataTypeList:
		if d.ElementType == nil {
			return fmt.Errorf("data type %q requires %q", d.Type, "elementType")
		}
		return d.ElementType.Validate()
	case DataTypeMap:
		if d.KeyType == nil {
			return fmt.Errorf("data type %q requires %q", d.Type, "keyType")
		}
		if d.ValueType == nil {
			return fmt.Errorf("data type %q requires %q", d.Type, "valueType")
		}
		if err := d.KeyType.Validate(); err != nil {
			return err
		}
		return d.ValueType.Validate()
	case DataTypeUnion:
		if len(d.Types) == 0 {
			return fmt.Errorf("data type %q requires at least one entry in %q", d.Type, "types")
		}
		for i, member := range d.Types {
			if err := member.Validate(); err != nil {
				return fmt.Errorf("data type %q: member %d: %w", d.Type, i, err)
			}
		}
	case DataTypeUnparsed:
		if strings.TrimSpace(d.UnparsedType) == "" {
			return fmt.Errorf("data type %q requires %q", d.Type, "unparsedType")
		}
	}

	return nil
}

// ParseDataType parses the Terraform representation of a column type: either a
// Gravitino primitive type name (optionally parameterised) or a JSON object
// describing one of the structured kinds.
func ParseDataType(value string) (DataType, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return DataType{}, fmt.Errorf("column type must not be empty")
	}

	switch trimmed[0] {
	case '{':
		var dataType DataType
		if err := json.Unmarshal([]byte(trimmed), &dataType); err != nil {
			return DataType{}, fmt.Errorf("invalid structured column type: %w", err)
		}
		if err := dataType.Validate(); err != nil {
			return DataType{}, err
		}
		return dataType, nil
	case '"':
		var name string
		if err := json.Unmarshal([]byte(trimmed), &name); err != nil {
			return DataType{}, fmt.Errorf("invalid column type: %w", err)
		}
		if !primitiveTypePattern.MatchString(name) {
			return DataType{}, invalidPrimitiveTypeError(name)
		}
		return DataType{Type: name}, nil
	}

	if !primitiveTypePattern.MatchString(trimmed) {
		return DataType{}, invalidPrimitiveTypeError(trimmed)
	}
	return DataType{Type: trimmed}, nil
}

func invalidPrimitiveTypeError(value string) error {
	return fmt.Errorf("invalid column type %q: expected a Gravitino primitive type such as \"integer\" or \"varchar(255)\", or a JSON object describing a struct, list, map, union or unparsed type", value)
}

// decodeUnparsedType accepts both the string documented by the schema and the
// single element array shown in the datatype.yaml example.
func decodeUnparsedType(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}

	if trimmed[0] == '[' {
		var values []string
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return ""
		}
		if len(values) == 0 {
			return ""
		}
		return values[0]
	}

	var value string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return ""
	}
	return value
}

// StructField mirrors datatype.yaml#/StructField.
type StructField struct {
	Name     string   `json:"name"`
	Type     DataType `json:"type"`
	Nullable *bool    `json:"nullable,omitempty"`
	Comment  string   `json:"comment,omitempty"`
}

// MarshalJSON emits the field with alphabetically ordered keys so it matches
// the output of HCL's jsonencode.
func (f StructField) MarshalJSON() ([]byte, error) {
	obj := map[string]interface{}{
		"name": f.Name,
		"type": f.Type,
	}
	if f.Nullable != nil {
		obj["nullable"] = *f.Nullable
	}
	if f.Comment != "" {
		obj["comment"] = f.Comment
	}
	return marshalCompactJSON(obj)
}

// Literal mirrors expression.yaml#/Literal: the only FunctionArg shape this
// provider uses for column default values and partition values.
type Literal struct {
	Type     string    `json:"type,omitempty"`
	DataType *DataType `json:"dataType"`
	Value    string    `json:"value"`
}

// NewLiteral builds a literal with the "literal" discriminator set.
func NewLiteral(dataType DataType, value string) Literal {
	return Literal{Type: LiteralType, DataType: &dataType, Value: value}
}

// MarshalJSON always emits the literal discriminator required by the
// expression.yaml discriminator mapping.
func (l Literal) MarshalJSON() ([]byte, error) {
	return marshalCompactJSON(map[string]interface{}{
		"type":     LiteralType,
		"dataType": l.DataType,
		"value":    l.Value,
	})
}

// Expression mirrors expression.yaml#/FunctionArg (literal, field or function).
// It is used for sort terms, distribution arguments and partitioning arguments.
type Expression struct {
	Type      string       `json:"type"`
	DataType  *DataType    `json:"dataType,omitempty"`
	Value     string       `json:"value,omitempty"`
	FieldName []string     `json:"fieldName,omitempty"`
	FuncName  string       `json:"funcName,omitempty"`
	FuncArgs  []Expression `json:"funcArgs,omitempty"`
}

// NewFieldExpression builds a "field" FunctionArg for the given field path.
func NewFieldExpression(fieldPath []string) Expression {
	return Expression{Type: "field", FieldName: fieldPath}
}

// ColumnPosition mirrors tables.yaml#/ColumnPosition: the strings "first" and
// "default", or the object {"after": "<column>"}.
type ColumnPosition struct {
	Mode  string
	After string
}

// FirstColumnPosition places a column at the front of the table.
func FirstColumnPosition() ColumnPosition {
	return ColumnPosition{Mode: ColumnPositionFirst}
}

// AfterColumnPosition places a column directly after another column.
func AfterColumnPosition(column string) ColumnPosition {
	return ColumnPosition{Mode: ColumnPositionAfter, After: column}
}

// DefaultColumnPosition lets the catalog decide where to place a column.
func DefaultColumnPosition() ColumnPosition {
	return ColumnPosition{Mode: ColumnPositionDefault}
}

// MarshalJSON encodes the union shape of ColumnPosition.
func (p ColumnPosition) MarshalJSON() ([]byte, error) {
	switch p.Mode {
	case ColumnPositionAfter:
		return marshalCompactJSON(map[string]string{"after": p.After})
	case ColumnPositionFirst:
		return marshalCompactJSON(ColumnPositionFirst)
	default:
		return marshalCompactJSON(ColumnPositionDefault)
	}
}

// marshalCompactJSON encodes a value as compact JSON without HTML escaping,
// matching the representation produced by HCL's jsonencode.
func marshalCompactJSON(value interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buffer.Bytes(), "\n"), nil
}
