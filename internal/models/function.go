package models

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// This file models the Gravitino v1.3.0 function REST contract
// (docs/open-api/functions.yaml, datatype.yaml and expression.yaml).
//
// A Function is:
//
//	{name, functionType, deterministic, comment, definitions[], audit}
//
// with each FunctionDefinition carrying {parameters[], returnType, returnColumns[], impls[]}.
// There is no "functionBody" and no function level "properties" in the API.

// Function types (components/schemas/Function -> functionType).
const (
	FunctionTypeScalar    = "SCALAR"
	FunctionTypeAggregate = "AGGREGATE"
	FunctionTypeTable     = "TABLE"
)

// AllFunctionTypes lists every functionType accepted by the Gravitino API.
var AllFunctionTypes = []string{FunctionTypeScalar, FunctionTypeAggregate, FunctionTypeTable}

// Implementation languages (components/schemas/FunctionImpl discriminator).
const (
	FunctionLanguageSQL    = "SQL"
	FunctionLanguageJava   = "JAVA"
	FunctionLanguagePython = "PYTHON"
)

// AllFunctionLanguages lists every implementation language of the API.
var AllFunctionLanguages = []string{FunctionLanguageSQL, FunctionLanguageJava, FunctionLanguagePython}

// Implementation runtimes (SQLImpl/JavaImpl/PythonImpl -> runtime).
const (
	FunctionRuntimeSpark = "SPARK"
	FunctionRuntimeTrino = "TRINO"
)

// AllFunctionRuntimes lists every implementation runtime of the API.
var AllFunctionRuntimes = []string{FunctionRuntimeSpark, FunctionRuntimeTrino}

// Function is components/schemas/Function.
type Function struct {
	Name          string               `json:"name"`
	FunctionType  string               `json:"functionType"`
	Deterministic bool                 `json:"deterministic"`
	Comment       string               `json:"comment,omitempty"`
	Definitions   []FunctionDefinition `json:"definitions"`
	Audit         *Audit               `json:"audit,omitempty"`
}

// FunctionDefinition is components/schemas/FunctionDefinition.
type FunctionDefinition struct {
	Parameters    []FunctionParam  `json:"parameters,omitempty"`
	ReturnType    json.RawMessage  `json:"returnType,omitempty"`
	ReturnColumns []FunctionColumn `json:"returnColumns,omitempty"`
	Impls         []FunctionImpl   `json:"impls,omitempty"`
}

// FunctionParam is components/schemas/FunctionParam.
type FunctionParam struct {
	Name         string          `json:"name"`
	DataType     json.RawMessage `json:"dataType"`
	Comment      string          `json:"comment,omitempty"`
	DefaultValue json.RawMessage `json:"defaultValue,omitempty"`
}

// FunctionColumn is components/schemas/FunctionColumn.
type FunctionColumn struct {
	Name     string          `json:"name"`
	DataType json.RawMessage `json:"dataType"`
	Comment  string          `json:"comment,omitempty"`
}

// FunctionImpl is components/schemas/FunctionImpl. The API discriminates the
// implementation by language and requires {@code language} and {@code runtime} on
// every implementation, plus {@code sql} for SQL, {@code className} for JAVA.
type FunctionImpl struct {
	Language   string             `json:"language"`
	Runtime    string             `json:"runtime"`
	SQL        string             `json:"sql,omitempty"`
	ClassName  string             `json:"className,omitempty"`
	Handler    string             `json:"handler,omitempty"`
	CodeBlock  string             `json:"codeBlock,omitempty"`
	Resources  *FunctionResources `json:"resources,omitempty"`
	Properties map[string]string  `json:"properties,omitempty"`
}

// FunctionResources is components/schemas/FunctionResources.
type FunctionResources struct {
	Jars     []string `json:"jars,omitempty"`
	Files    []string `json:"files,omitempty"`
	Archives []string `json:"archives,omitempty"`
}

// FunctionRegisterRequest is components/schemas/FunctionRegisterRequest.
type FunctionRegisterRequest struct {
	Name          string               `json:"name"`
	FunctionType  string               `json:"functionType"`
	Deterministic bool                 `json:"deterministic"`
	Comment       string               `json:"comment,omitempty"`
	Definitions   []FunctionDefinition `json:"definitions"`
}

// FunctionUpdatesRequest is components/schemas/FunctionUpdatesRequest.
type FunctionUpdatesRequest struct {
	Updates []any `json:"updates"`
}

// FunctionResponse is components/responses/FunctionResponse.
type FunctionResponse struct {
	Code     int      `json:"code"`
	Function Function `json:"function"`
}

// FunctionListResponse is components/schemas/FunctionListResponse, returned by
// GET /functions?details=true.
type FunctionListResponse struct {
	Code      int        `json:"code"`
	Functions []Function `json:"functions"`
}

// NewUpdateFunctionCommentRequest builds an "updateComment" update
// (components/schemas/UpdateFunctionCommentRequest). A nil comment marshals as
// {"newComment":null}, which is how the API clears a comment.
func NewUpdateFunctionCommentRequest(newComment *string) any {
	return struct {
		Type       string  `json:"@type"`
		NewComment *string `json:"newComment"`
	}{Type: "updateComment", NewComment: newComment}
}

// NewAddFunctionDefinitionRequest builds an "addDefinition" update
// (components/schemas/AddFunctionDefinitionRequest).
func NewAddFunctionDefinitionRequest(definition FunctionDefinition) any {
	return struct {
		Type       string             `json:"@type"`
		Definition FunctionDefinition `json:"definition"`
	}{Type: "addDefinition", Definition: definition}
}

// NewRemoveFunctionDefinitionRequest builds a "removeDefinition" update
// (components/schemas/RemoveFunctionDefinitionRequest). Definitions are
// identified by their parameters.
func NewRemoveFunctionDefinitionRequest(parameters []FunctionParam) any {
	if parameters == nil {
		parameters = []FunctionParam{}
	}
	return struct {
		Type       string          `json:"@type"`
		Parameters []FunctionParam `json:"parameters"`
	}{Type: "removeDefinition", Parameters: parameters}
}

// NewAddFunctionImplRequest builds an "addImpl" update
// (components/schemas/AddFunctionImplRequest).
func NewAddFunctionImplRequest(parameters []FunctionParam, implementation FunctionImpl) any {
	if parameters == nil {
		parameters = []FunctionParam{}
	}
	return struct {
		Type           string          `json:"@type"`
		Parameters     []FunctionParam `json:"parameters"`
		Implementation FunctionImpl    `json:"implementation"`
	}{Type: "addImpl", Parameters: parameters, Implementation: implementation}
}

// NewUpdateFunctionImplRequest builds an "updateImpl" update
// (components/schemas/UpdateFunctionImplRequest). The implementation is
// identified by the definition parameters plus the runtime.
func NewUpdateFunctionImplRequest(parameters []FunctionParam, runtime string, implementation FunctionImpl) any {
	if parameters == nil {
		parameters = []FunctionParam{}
	}
	return struct {
		Type           string          `json:"@type"`
		Parameters     []FunctionParam `json:"parameters"`
		Runtime        string          `json:"runtime"`
		Implementation FunctionImpl    `json:"implementation"`
	}{Type: "updateImpl", Parameters: parameters, Runtime: runtime, Implementation: implementation}
}

// NewRemoveFunctionImplRequest builds a "removeImpl" update
// (components/schemas/RemoveFunctionImplRequest).
func NewRemoveFunctionImplRequest(parameters []FunctionParam, runtime string) any {
	if parameters == nil {
		parameters = []FunctionParam{}
	}
	return struct {
		Type       string          `json:"@type"`
		Parameters []FunctionParam `json:"parameters"`
		Runtime    string          `json:"runtime"`
	}{Type: "removeImpl", Parameters: parameters, Runtime: runtime}
}

// Signature identifies a function definition the way the API does: by its
// parameters (removeDefinition/addImpl/updateImpl/removeImpl all take the
// parameters of the target definition). Two definitions with the same parameter
// names and data types are the same definition for Gravitino.
func (d FunctionDefinition) Signature() string {
	parts := make([]string, 0, len(d.Parameters))
	for _, p := range d.Parameters {
		parts = append(parts, p.Name+":"+DataTypeToString(p.DataType))
	}
	return strings.Join(parts, ",")
}

// Signature identifies a definition from a Terraform list of parameters.
func FunctionDefinitionSignature(parameters []FunctionParam) string {
	return FunctionDefinition{Parameters: parameters}.Signature()
}

// Shape renders a definition without its implementations; used to detect changes
// that Gravitino can only express as removeDefinition + addDefinition.
func (d FunctionDefinition) Shape() string {
	d.Impls = nil
	encoded, err := json.Marshal(d)
	if err != nil {
		return fmt.Sprintf("%#v", d)
	}
	return string(encoded)
}

// ---------------------------------------------------------------------------
// Data type / expression encoding
// ---------------------------------------------------------------------------
//
// Gravitino's DataType (datatype.yaml) is a union: primitive types are plain
// strings ("integer", "decimal(10,2)"), complex types are JSON documents
// ({"type":"list","elementType":"integer"}). Function arguments (expression.yaml)
// are JSON documents as well.
//
// Both are exposed to Terraform as strings using the same encoding: anything that
// starts with { or [ is the JSON document of the API, everything else is used as
// the literal string of the API (a primitive type name). jsonencode() makes the
// document forms readable in HCL.

// DataTypeFromString converts a Terraform data type expression into the JSON value
// sent to the API. It validates the expression with the same rules as a
// table/view column, so functions and columns agree on what a valid Gravitino
// data type is (a primitive name, a structured kind or the external variant).
func DataTypeFromString(value string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("must not be empty")
	}

	if _, err := ParseDataType(trimmed); err != nil {
		return nil, err
	}

	return encodeDataTypeExpression(trimmed)
}

// RawJSONFromString converts a Terraform JSON expression into the raw JSON sent
// to the API without data-type validation. It is used for function parameter
// default values, which are Gravitino argument documents rather than data types.
func RawJSONFromString(value string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("must not be empty")
	}

	return encodeDataTypeExpression(trimmed)
}

// encodeDataTypeExpression returns the JSON value for an already parsed data
// type or expression: structured/string documents are passed through, bare
// primitive names are encoded as a JSON string.
func encodeDataTypeExpression(trimmed string) (json.RawMessage, error) {
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if !json.Valid([]byte(trimmed)) {
			return nil, fmt.Errorf("%q is not valid JSON", trimmed)
		}
		return json.RawMessage(trimmed), nil
	}

	if strings.HasPrefix(trimmed, "\"") {
		if !json.Valid([]byte(trimmed)) {
			return nil, fmt.Errorf("%q is not valid JSON", trimmed)
		}
		return json.RawMessage(trimmed), nil
	}

	encoded, err := json.Marshal(trimmed)
	if err != nil {
		return nil, fmt.Errorf("failed to encode %q: %w", trimmed, err)
	}
	return encoded, nil
}

// DataTypeToString converts a data type received from the API into its Terraform
// representation. Values that came from Terraform are returned unchanged.
func DataTypeToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var primitive string
	if err := json.Unmarshal(raw, &primitive); err == nil {
		return primitive
	}

	return strings.TrimSpace(string(raw))
}

// RawJSONEqual reports whether two API values are semantically equal, so that a
// hand written (or jsonencode produced) data type is not reported as drift when
// the server returns the same type with different formatting.
func RawJSONEqual(a, b json.RawMessage) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}

	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

// preferRaw keeps the value that came from Terraform when the server returned an
// equivalent one, so state keeps the exact formatting of the configuration.
func preferRaw(previous, returned json.RawMessage) json.RawMessage {
	if len(previous) == 0 || !RawJSONEqual(previous, returned) {
		return returned
	}
	return previous
}
