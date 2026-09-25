package models

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestDataTypeMarshalPrimitive asserts a primitive column type is sent as the
// bare JSON string documented by the PrimitiveType examples of datatype.yaml.
// Gravitino silently decodes the object form as an unparsed type, which is why
// the wire format matters.
func TestDataTypeMarshalPrimitive(t *testing.T) {
	for _, name := range []string{"boolean", "integer", "long", "decimal(10,2)", "varchar(255)", "timestamp(3)", "byte unsigned", "binary"} {
		encoded, err := json.Marshal(DataType{Type: name})
		if err != nil {
			t.Fatalf("marshal %q: %v", name, err)
		}
		if string(encoded) != `"`+name+`"` {
			t.Errorf("marshal %q = %s", name, encoded)
		}
	}
}

// TestDataTypeMarshalStruct asserts the structured kinds are sent as the JSON
// objects of datatype.yaml with the field names of the spec.
func TestDataTypeMarshalStruct(t *testing.T) {
	nullable := false
	dataType := DataType{
		Type: DataTypeStruct,
		Fields: []StructField{
			{Name: "position", Type: DataType{Type: "string"}},
			{
				Name:     "contact",
				Type:     DataType{Type: DataTypeList, ElementType: &DataType{Type: "integer"}, ContainsNull: &nullable},
				Nullable: &nullable,
			},
		},
	}

	if err := dataType.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	encoded, err := json.Marshal(dataType)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	expected := `{"fields":[{"name":"position","type":"string"},{"name":"contact","nullable":false,` +
		`"type":{"containsNull":false,"elementType":"integer","type":"list"}}],"type":"struct"}`
	if string(encoded) != expected {
		t.Errorf("marshal struct =\n%s\nwant\n%s", encoded, expected)
	}
}

// TestDataTypeRoundTripUnmarshal asserts the types Gravitino reports decode and
// re-encode to the same canonical form.
func TestDataTypeRoundTripUnmarshal(t *testing.T) {
	tests := map[string]string{
		"primitive":         `"varchar(255)"`,
		"object primitive":  `{"type":"integer"}`,
		"struct":            `{"type":"struct","fields":[{"name":"id","type":"integer","nullable":false,"comment":"The id"}]}`,
		"list":              `{"type":"list","elementType":"integer","containsNull":false}`,
		"map":               `{"type":"map","keyType":"string","valueType":"integer","valueContainsNull":false}`,
		"union":             `{"type":"union","types":["string","integer"]}`,
		"unparsed":          `{"type":"unparsed","unparsedType":"unknown-type"}`,
		"unparsed in array": `{"type":"unparsed","unparsedType":["unknown-type"]}`,
	}

	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			var dataType DataType
			if err := json.Unmarshal([]byte(raw), &dataType); err != nil {
				t.Fatalf("unmarshal %s: %v", raw, err)
			}
			if err := dataType.Validate(); err != nil {
				t.Fatalf("validate: %v", err)
			}

			canonical := dataType.String()
			if canonical == "" {
				t.Fatal("String() must not be empty")
			}

			reparsed, err := ParseDataType(canonical)
			if err != nil {
				t.Fatalf("ParseDataType(%q): %v", canonical, err)
			}
			if reparsed.String() != canonical {
				t.Errorf("the canonical form is not stable: %q -> %q", canonical, reparsed.String())
			}

			if !dataType.IsStructured() && canonical != dataType.Type {
				t.Errorf("String() = %q, want the bare primitive name %q", canonical, dataType.Type)
			}
		})
	}
}

// TestDataTypeMarshalObjectPrimitiveIsNormalised asserts the object form of a
// primitive type decodes to the primitive and is re-encoded as the bare string.
func TestDataTypeMarshalObjectPrimitiveIsNormalised(t *testing.T) {
	var dataType DataType
	if err := json.Unmarshal([]byte(`{"type":"integer"}`), &dataType); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if dataType.String() != "integer" {
		t.Errorf("String() = %q, want integer", dataType.String())
	}
}

func TestParseDataType(t *testing.T) {
	valid := []string{
		"integer",
		"varchar(255)",
		"decimal(10,2)",
		"  date  ",
		`"string"`,
		`{"type":"struct","fields":[{"name":"id","type":"integer"}]}`,
	}
	for _, value := range valid {
		if _, err := ParseDataType(value); err != nil {
			t.Errorf("ParseDataType(%q) = %v, want no error", value, err)
		}
	}

	invalid := []string{
		"",
		"   ",
		"{",
		`{"fields":[{"name":"id","type":"integer"}]}`,
		`{"type":"struct","fields":[]}`,
		`{"type":"list"}`,
		`{"type":"map","keyType":"string"}`,
		`{"type":"union","types":[]}`,
		`{"type":"unparsed"}`,
		"decimal(10,2",
		"1invalid",
	}
	for _, value := range invalid {
		if _, err := ParseDataType(value); err == nil {
			t.Errorf("ParseDataType(%q) = nil, want an error", value)
		}
	}
}

func TestLiteralMarshal(t *testing.T) {
	dataType := DataType{Type: "varchar(255)"}
	encoded, err := json.Marshal(NewLiteral(dataType, "default_name"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	expected := `{"dataType":"varchar(255)","type":"literal","value":"default_name"}`
	if string(encoded) != expected {
		t.Errorf("marshal literal = %s, want %s", encoded, expected)
	}
}

func TestColumnPositionMarshal(t *testing.T) {
	tests := map[string]struct {
		position ColumnPosition
		expected string
	}{
		"first":   {FirstColumnPosition(), `"first"`},
		"default": {DefaultColumnPosition(), `"default"`},
		"after":   {AfterColumnPosition("id"), `{"after":"id"}`},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.position)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(encoded) != tt.expected {
				t.Errorf("marshal = %s, want %s", encoded, tt.expected)
			}
		})
	}
}

// TestTableUpdateRequestMarshalsOnlyItsOwnFields asserts every update request
// carries exactly the members of its schema: Gravitino rejects an unknown @type
// and the union members must not leak into each other.
func TestTableUpdateRequestMarshalsOnlyItsOwnFields(t *testing.T) {
	dataType := DataType{Type: "long"}
	position := AfterColumnPosition("id")
	literal := NewLiteral(DataType{Type: "integer"}, "7")

	tests := map[string]struct {
		update   TableUpdateRequest
		expected string
	}{
		"rename": {
			update:   NewRenameTableRequest("new_name"),
			expected: `{"@type":"rename","newName":"new_name"}`,
		},
		"updateComment": {
			update:   NewUpdateTableCommentRequest("comment"),
			expected: `{"@type":"updateComment","newComment":"comment"}`,
		},
		"setProperty": {
			update:   NewSetTablePropertyRequest("key", "value"),
			expected: `{"@type":"setProperty","property":"key","value":"value"}`,
		},
		"removeProperty": {
			update:   NewRemoveTablePropertyRequest("key"),
			expected: `{"@type":"removeProperty","property":"key"}`,
		},
		"updateColumnType": {
			update:   NewUpdateTableColumnTypeRequest([]string{"age"}, dataType),
			expected: `{"@type":"updateColumnType","fieldName":["age"],"newType":"long"}`,
		},
		"updateColumnComment": {
			update:   NewUpdateTableColumnCommentRequest([]string{"age"}, "c"),
			expected: `{"@type":"updateColumnComment","fieldName":["age"],"newComment":"c"}`,
		},
		"updateColumnPosition": {
			update:   NewUpdateTableColumnPositionRequest([]string{"age"}, position),
			expected: `{"@type":"updateColumnPosition","fieldName":["age"],"newPosition":{"after":"id"}}`,
		},
		"updateColumnNullability": {
			update:   NewUpdateTableColumnNullabilityRequest([]string{"age"}, false),
			expected: `{"@type":"updateColumnNullability","fieldName":["age"],"nullable":false}`,
		},
		"updateColumnDefaultValue": {
			update:   NewUpdateTableColumnDefaultValueRequest([]string{"age"}, &literal),
			expected: `{"@type":"updateColumnDefaultValue","fieldName":["age"],"newDefaultValue":{"dataType":"integer","type":"literal","value":"7"}}`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.update)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(encoded) != tt.expected {
				t.Errorf("marshal =\n%s\nwant\n%s", encoded, tt.expected)
			}
		})
	}
}

// TestTableUpdateRequestUnsupportedType asserts an update type the provider does
// not model is refused instead of being sent with the wrong members.
func TestTableUpdateRequestUnsupportedType(t *testing.T) {
	_, err := json.Marshal(TableUpdateRequest{Type: "deleteColumn"})
	if err == nil {
		t.Fatal("expected an error for an unsupported update type")
	}
	if !strings.Contains(err.Error(), "deleteColumn") {
		t.Errorf("error = %v", err)
	}
}

func TestPartitionMarshalUsesSpecFieldNames(t *testing.T) {
	partition := Partition{
		Type:       PartitionTypeIdentity,
		FieldNames: [][]string{{"hive_col_name2"}, {"hive_col_name3"}},
		Values: []Literal{
			NewLiteral(DataType{Type: "date"}, "2023-01-02"),
			NewLiteral(DataType{Type: "string"}, "gravitino_it_test2"),
		},
	}

	encoded, err := json.Marshal(AddPartitionsRequest{Partitions: []Partition{partition}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	expected := `{"partitions":[{"type":"identity","fieldNames":[["hive_col_name2"],["hive_col_name3"]],` +
		`"values":[{"dataType":"date","type":"literal","value":"2023-01-02"},` +
		`{"dataType":"string","type":"literal","value":"gravitino_it_test2"}]}]}`
	if string(encoded) != expected {
		t.Errorf("marshal =\n%s\nwant\n%s", encoded, expected)
	}
}
