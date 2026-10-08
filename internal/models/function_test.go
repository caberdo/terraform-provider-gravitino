package models

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// TestDataTypeFromStringValidatesLikeColumnType asserts that function data types
// are validated with the same rules as table/view column types, so a typo that is
// rejected for a column is not silently accepted for a function parameter.
func TestDataTypeFromStringValidatesLikeColumnType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "primitive", input: "integer"},
		{name: "parameterised primitive", input: "varchar(255)"},
		{name: "quoted primitive", input: `"integer"`},
		{name: "structured", input: `{"type":"struct","fields":[{"name":"a","type":"integer"}]}`},
		{name: "external", input: `{"type":"external","catalogString":"user-defined"}`},
		{name: "structured kind as bare string", input: "struct", wantErr: true},
		{name: "empty", input: "", wantErr: true},
		{name: "bare array", input: `[1]`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DataTypeFromString(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("DataTypeFromString(%q) = nil error, want an error", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("DataTypeFromString(%q) error = %v", tt.input, err)
			}
		})
	}
}

// TestRawJSONFromStringKeepsExpressionDocuments asserts a parameter default value
// is passed through as an argument document rather than validated as a data type.
func TestRawJSONFromStringKeepsExpressionDocuments(t *testing.T) {
	expression := `[1, 2]`

	got, err := RawJSONFromString(expression)
	if err != nil {
		t.Fatalf("RawJSONFromString(%q) error = %v", expression, err)
	}
	if string(got) != expression {
		t.Errorf("RawJSONFromString(%q) = %s, want it unchanged", expression, got)
	}

	if _, err := DataTypeFromString(expression); err == nil {
		t.Errorf("DataTypeFromString accepted the expression %q as a data type", expression)
	}
}

// TestFunctionExternalTypeDiagnostics asserts the function surface reports the
// external variant on a server that predates Gravitino 1.3.1 and stays silent
// when the server accepts it.
func TestFunctionExternalTypeDiagnostics(t *testing.T) {
	external := json.RawMessage(`{"type":"external","catalogString":"user-defined"}`)
	integer := json.RawMessage(`"integer"`)

	tests := []struct {
		name   string
		defs   []FunctionDefinition
		expect string
	}{
		{
			name: "parameter",
			defs: []FunctionDefinition{{
				Parameters: []FunctionParam{{Name: "x", DataType: external}},
				ReturnType: integer,
			}},
			expect: `definitions[0].parameters[0].data_type`,
		},
		{
			name: "return type",
			defs: []FunctionDefinition{{
				ReturnType: external,
			}},
			expect: `definitions[0].return_type`,
		},
		{
			name: "return column",
			defs: []FunctionDefinition{{
				ReturnType:    integer,
				ReturnColumns: []FunctionColumn{{Name: "c", DataType: external}},
			}},
			expect: `definitions[0].return_columns[0].data_type`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diags := FunctionExternalTypeDiagnostics(tt.defs, "1.3.1", true); diags.HasError() {
				t.Fatalf("supported server reported an error: %v", diags)
			}

			diags := FunctionExternalTypeDiagnostics(tt.defs, "1.3.0", false)
			if !diags.HasError() {
				t.Fatal("expected a diagnostic for the external data type")
			}
			if !strings.Contains(diags[0].Summary(), "1.3.1") {
				t.Errorf("summary = %q, want the required server version", diags[0].Summary())
			}
			if !strings.Contains(diags[0].Detail(), "1.3.0") {
				t.Errorf("detail = %q, want the detected server version", diags[0].Detail())
			}
			if got, ok := diags[0].(diag.DiagnosticWithPath); !ok {
				t.Errorf("diagnostic has no attribute path: %v", diags[0])
			} else if got.Path().String() != tt.expect {
				t.Errorf("path = %q, want %q", got.Path().String(), tt.expect)
			}
		})
	}
}
