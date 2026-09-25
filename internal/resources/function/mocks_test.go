package function_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/function"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Payloads taken from the v1.3.0 OpenAPI spec (/tmp/gravspec/functions.yaml,
// components/examples). They are used both as mock responses and as the expected
// request bodies, so the tests fail when the provider stops speaking the spec.

// components/examples/FunctionResponse
const specFunctionResponse = `{
  "code": 0,
  "function": {
    "name": "add_one",
    "functionType": "SCALAR",
    "deterministic": true,
    "comment": "A simple scalar function that adds one",
    "definitions": [
      {
        "parameters": [
          {"name": "x", "dataType": "integer"}
        ],
        "returnType": "integer",
        "impls": [
          {
            "language": "SQL",
            "runtime": "SPARK",
            "sql": "x + 1"
          }
        ]
      }
    ],
    "audit": {
      "creator": "user1",
      "createTime": "2021-01-01T00:00:00Z",
      "lastModifier": "user1",
      "lastModifiedTime": "2021-01-01T00:00:00Z"
    }
  }
}`

// components/examples/NoSuchFunctionException. The application code is 1003 even
// though the response is a 404: the status code is the only reliable signal.
const specNoSuchFunctionException = `{"code":1003,"type":"NoSuchFunctionException",` +
	`"message":"Function does not exist","stack":["org.apache.gravitino.exceptions.NoSuchFunctionException: Function does not exist"]}`

// components/examples/FunctionRegisterRequest, which registers the function of
// specFunctionResponse. The spec example also lists a JAVA implementation on
// SPARK, but a real 1.3.0 server rejects a definition with two implementations on
// the same runtime:
//
//	HTTP 400 {"code":1001,"type":"IllegalArgumentException","message":"... Cannot
//	register function: definition at index 0 has duplicate runtime 'SPARK'. Each
//	definition must have at most one implementation per runtime."}
const specRegisterRequest = `{
  "name": "add_one",
  "functionType": "SCALAR",
  "deterministic": true,
  "comment": "A simple scalar function that adds one",
  "definitions": [
    {
      "parameters": [
        {"name": "x", "dataType": "integer"}
      ],
      "returnType": "integer",
      "impls": [
        {
          "language": "SQL",
          "runtime": "SPARK",
          "sql": "x + 1"
        }
      ]
    }
  ]
}`

// The JAVA implementation of components/examples/FunctionRegisterRequest, moved to
// TRINO because of the runtime uniqueness rule described above.
const specRegisterRequestWithJavaImpl = `{
  "name": "add_one_java",
  "functionType": "SCALAR",
  "deterministic": true,
  "comment": "A simple scalar function that adds one",
  "definitions": [
    {
      "parameters": [
        {"name": "x", "dataType": "integer"}
      ],
      "returnType": "integer",
      "impls": [
        {
          "language": "JAVA",
          "runtime": "TRINO",
          "className": "com.example.AddOneFunction",
          "resources": {
            "jars": ["hdfs:///path/to/udf.jar"]
          }
        }
      ]
    }
  ]
}`

// components/examples/TableFunctionRegisterRequest with the JAVA implementation
// dropped (two SPARK implementations are rejected by the server).
const specTableRegisterRequest = `{
  "name": "generate_series",
  "functionType": "TABLE",
  "deterministic": true,
  "comment": "A table function that generates a series of integers",
  "definitions": [
    {
      "parameters": [
        {"name": "start_val", "dataType": "integer"},
        {"name": "end_val", "dataType": "integer"}
      ],
      "returnColumns": [
        {"name": "value", "dataType": "integer", "comment": "The generated integer value"}
      ],
      "impls": [
        {
          "language": "PYTHON",
          "runtime": "SPARK",
          "handler": "generate_series_handler",
          "codeBlock": "def generate_series_handler(start_val, end_val):\n  for i in range(start_val, end_val + 1):\n    yield (i,)"
        }
      ]
    }
  ]
}`

// mockRequest is one request the provider sent to the mocked Gravitino API.
type mockRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   []byte
}

// functionMock serves the function endpoints of the v1.3.0 API. It stores the
// functions that were registered and replays the behaviours of a real server that
// matter to the provider:
//
//   - the response spells functionType in lower case ("scalar"), while the request
//     requires "SCALAR" (verified against a real 1.3.0 server);
//   - every implementation carries resources/properties objects, even when the
//     request omitted them;
//   - data type documents are re-encoded, so key order differs from the request;
//   - an unknown function is a 404 with the spec's NoSuchFunctionException body.
type functionMock struct {
	t         *testing.T
	server    *httptest.Server
	mu        sync.Mutex
	requests  []mockRequest
	functions map[string]map[string]any
}

func newFunctionMock(t *testing.T) *functionMock {
	t.Helper()
	mock := &functionMock{t: t, functions: map[string]map[string]any{}}
	mock.server = httptest.NewServer(mock)
	t.Cleanup(mock.server.Close)
	return mock
}

func (m *functionMock) client() *client.Client {
	m.t.Helper()
	c, err := client.New(m.server.URL, nil)
	if err != nil {
		m.t.Fatalf("client.New() error = %v", err)
	}
	return c
}

func (m *functionMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	m.mu.Lock()
	m.requests = append(m.requests, mockRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.Query(),
		Body:   body,
	})
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")

	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/functions"):
		var function map[string]any
		if err := json.Unmarshal(body, &function); err != nil {
			m.t.Errorf("mock: failed to decode register request: %v", err)
		}
		name := asString(function["name"])
		m.store(name, function)
		stored, _ := m.load(name)
		m.write(w, map[string]any{"code": 0, "function": stored})

	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/functions"):
		functions := make([]any, 0)
		for _, f := range m.all() {
			functions = append(functions, f)
		}
		m.write(w, map[string]any{"code": 0, "functions": functions})

	case r.Method == http.MethodGet:
		function, ok := m.load(lastPathSegment(r.URL.Path))
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(specNoSuchFunctionException))
			return
		}
		m.write(w, map[string]any{"code": 0, "function": function})

	case r.Method == http.MethodPut:
		name := lastPathSegment(r.URL.Path)
		current, ok := m.load(name)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(specNoSuchFunctionException))
			return
		}

		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			m.t.Errorf("mock: failed to decode update request: %v", err)
		}
		updates, _ := request["updates"].([]any)
		updated := m.applyUpdates(current, updates)
		m.store(name, updated)
		stored, _ := m.load(name)
		m.write(w, map[string]any{"code": 0, "function": stored})

	case r.Method == http.MethodDelete:
		name := lastPathSegment(r.URL.Path)
		m.mu.Lock()
		_, existed := m.functions[name]
		delete(m.functions, name)
		m.mu.Unlock()
		m.write(w, map[string]any{"code": 0, "dropped": existed})

	default:
		http.NotFound(w, r)
	}
}

// applyUpdates mimics the alter semantics of the API (FunctionUpdateRequest).
func (m *functionMock) applyUpdates(function map[string]any, updates []any) map[string]any {
	definitions, _ := function["definitions"].([]any)

	for _, raw := range updates {
		update, _ := raw.(map[string]any)
		switch asString(update["@type"]) {
		case "updateComment":
			function["comment"] = update["newComment"]
		case "addDefinition":
			definitions = append(definitions, update["definition"])
		case "removeDefinition":
			parameters, _ := update["parameters"].([]any)
			definitions = filterDefinitions(m.t, definitions, func(d map[string]any) bool {
				return !parametersEqual(definitionParameters(d), parameters)
			})
		case "addImpl":
			definitions = m.mutateDefinition(definitions, update["parameters"], func(impls []any) []any {
				return append(impls, update["implementation"])
			})
		case "updateImpl":
			definitions = m.mutateDefinition(definitions, update["parameters"], func(impls []any) []any {
				updated := make([]any, 0, len(impls))
				for _, raw := range impls {
					impl, _ := raw.(map[string]any)
					if asString(impl["runtime"]) == asString(update["runtime"]) {
						updated = append(updated, update["implementation"])
						continue
					}
					updated = append(updated, raw)
				}
				return updated
			})
		case "removeImpl":
			definitions = m.mutateDefinition(definitions, update["parameters"], func(impls []any) []any {
				kept := make([]any, 0, len(impls))
				for _, raw := range impls {
					impl, _ := raw.(map[string]any)
					if asString(impl["runtime"]) == asString(update["runtime"]) {
						continue
					}
					kept = append(kept, raw)
				}
				return kept
			})
		default:
			m.t.Errorf("mock: unsupported update @type %v", update["@type"])
		}

		// A real server rewrites the audit of an altered function.
		function["audit"] = map[string]any{
			"creator":          "anonymous",
			"createTime":       "2021-01-01T00:00:00Z",
			"lastModifier":     "anonymous",
			"lastModifiedTime": "2021-01-01T00:00:01Z",
		}
	}

	function["definitions"] = definitions
	return function
}

func (m *functionMock) mutateDefinition(definitions []any, parameters any, mutate func([]any) []any) []any {
	params, _ := parameters.([]any)
	mutated := make([]any, 0, len(definitions))
	for _, raw := range definitions {
		definition, _ := raw.(map[string]any)
		if parametersEqual(definitionParameters(definition), params) {
			impls, _ := definition["impls"].([]any)
			definition["impls"] = mutate(impls)
		}
		mutated = append(mutated, definition)
	}
	return mutated
}

func filterDefinitions(t *testing.T, definitions []any, keep func(map[string]any) bool) []any {
	t.Helper()
	kept := make([]any, 0, len(definitions))
	for _, raw := range definitions {
		definition, _ := raw.(map[string]any)
		if keep(definition) {
			kept = append(kept, raw)
		}
	}
	return kept
}

func definitionParameters(definition map[string]any) []any {
	parameters, _ := definition["parameters"].([]any)
	return parameters
}

func (m *functionMock) store(name string, function map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.functions[name] = normalizeFunction(function)
}

func (m *functionMock) load(name string) (map[string]any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	function, ok := m.functions[name]
	return function, ok
}

func (m *functionMock) all() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.functions))
	for name := range m.functions {
		names = append(names, name)
	}
	sort.Strings(names)

	functions := make([]map[string]any, 0, len(names))
	for _, name := range names {
		functions = append(functions, m.functions[name])
	}
	return functions
}

func (m *functionMock) write(w http.ResponseWriter, payload any) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

// normalizeFunction applies the quirks of a real Gravitino 1.3.0 response.
func normalizeFunction(function map[string]any) map[string]any {
	function["functionType"] = strings.ToLower(asString(function["functionType"]))

	if _, ok := function["audit"]; !ok {
		function["audit"] = map[string]any{
			"creator":    "anonymous",
			"createTime": "2021-01-01T00:00:00Z",
		}
	}

	definitions, _ := function["definitions"].([]any)
	for _, rawDefinition := range definitions {
		definition, _ := rawDefinition.(map[string]any)
		impls, _ := definition["impls"].([]any)
		for _, rawImpl := range impls {
			impl, _ := rawImpl.(map[string]any)
			if _, ok := impl["resources"]; !ok {
				impl["resources"] = map[string]any{"jars": []any{}, "files": []any{}, "archives": []any{}}
			}
			if _, ok := impl["properties"]; !ok {
				impl["properties"] = map[string]any{}
			}
		}
	}

	return function
}

func lastPathSegment(path string) string {
	trimmed := strings.TrimSuffix(path, "/")
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}

func asString(value any) string {
	s, _ := value.(string)
	return s
}

// ---------------------------------------------------------------------------
// Request helpers
// ---------------------------------------------------------------------------

func functionResourceSchema(t *testing.T) schema.Schema {
	t.Helper()
	r := res.New()
	response := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func auditNullValue(t *testing.T, schemaObj schema.Schema) types.Object {
	t.Helper()
	attributes := schemaObj.Type().(types.ObjectType).AttributeTypes()
	auditType, ok := attributes["audit"].(types.ObjectType)
	if !ok {
		t.Fatalf("audit attribute type = %T, want types.ObjectType", attributes["audit"])
	}
	return types.ObjectNull(auditType.AttrTypes)
}

func terraformValue(t *testing.T, schemaObj schema.Schema, model any) tftypes.Value {
	t.Helper()
	object, diags := types.ObjectValueFrom(context.Background(), schemaObj.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("ObjectValueFrom(%T) diagnostics = %v", model, diags)
	}
	value, err := object.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("ToTerraformValue() error = %v", err)
	}
	return value
}

func modelFromFunctionJSON(t *testing.T, schemaObj schema.Schema, metalake, catalog, sch, functionJSON string) res.FunctionResourceModel {
	t.Helper()
	var function models.Function
	if err := json.Unmarshal([]byte(functionJSON), &function); err != nil {
		t.Fatalf("failed to decode function: %v", err)
	}

	definitions, diags := models.FunctionDefinitionsToTF(context.Background(), function.Definitions)
	if diags.HasError() {
		t.Fatalf("FunctionDefinitionsToTF() diagnostics = %v", diags)
	}

	comment := types.StringNull()
	if function.Comment != "" {
		comment = types.StringValue(function.Comment)
	}

	return res.FunctionResourceModel{
		ID:            types.StringValue(fmt.Sprintf("%s.%s.%s.%s", metalake, catalog, sch, function.Name)),
		Metalake:      types.StringValue(metalake),
		Catalog:       types.StringValue(catalog),
		Schema:        types.StringValue(sch),
		Name:          types.StringValue(function.Name),
		FunctionType:  types.StringValue(models.NormalizeFunctionType(function.FunctionType)),
		Deterministic: types.BoolValue(function.Deterministic),
		Comment:       comment,
		Definitions:   definitions,
		Audit:         auditNullValue(t, schemaObj),
	}
}

// modelFromRegisterRequest builds the plan of a create from a register request
// JSON document: the definitions are read back with the same conversion the data
// sources use, so the plan mirrors the spec's example payload.
func modelFromRegisterRequest(t *testing.T, schemaObj schema.Schema, metalake, catalog, sch, requestJSON string) res.FunctionResourceModel {
	t.Helper()
	var request models.FunctionRegisterRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("failed to decode register request: %v", err)
	}

	definitions, diags := models.FunctionDefinitionsToTF(context.Background(), request.Definitions)
	if diags.HasError() {
		t.Fatalf("FunctionDefinitionsToTF() diagnostics = %v", diags)
	}

	comment := types.StringNull()
	if request.Comment != "" {
		comment = types.StringValue(request.Comment)
	}

	return res.FunctionResourceModel{
		ID:            types.StringNull(),
		Metalake:      types.StringValue(metalake),
		Catalog:       types.StringValue(catalog),
		Schema:        types.StringValue(sch),
		Name:          types.StringValue(request.Name),
		FunctionType:  types.StringValue(request.FunctionType),
		Deterministic: types.BoolValue(request.Deterministic),
		Comment:       comment,
		Definitions:   definitions,
		Audit:         auditNullValue(t, schemaObj),
	}
}

func planFor(t *testing.T, schemaObj schema.Schema, model res.FunctionResourceModel) tfsdk.Plan {
	t.Helper()
	return tfsdk.Plan{Schema: schemaObj, Raw: terraformValue(t, schemaObj, model)}
}

func stateFor(t *testing.T, schemaObj schema.Schema, model res.FunctionResourceModel) tfsdk.State {
	t.Helper()
	return tfsdk.State{Schema: schemaObj, Raw: terraformValue(t, schemaObj, model)}
}

func definitionsOf(t *testing.T, list types.List) []models.FunctionDefinitionTFSDK {
	t.Helper()
	var definitions []models.FunctionDefinitionTFSDK
	if list.IsNull() || list.IsUnknown() {
		return definitions
	}
	if diags := list.ElementsAs(context.Background(), &definitions, false); diags.HasError() {
		t.Fatalf("ElementsAs() diagnostics = %v", diags)
	}
	return definitions
}

func implsOf(t *testing.T, list types.List) []models.FunctionImplTFSDK {
	t.Helper()
	var impls []models.FunctionImplTFSDK
	if list.IsNull() || list.IsUnknown() {
		return impls
	}
	if diags := list.ElementsAs(context.Background(), &impls, false); diags.HasError() {
		t.Fatalf("ElementsAs() diagnostics = %v", diags)
	}
	return impls
}

func parametersOf(t *testing.T, list types.List) []models.FunctionParamTFSDK {
	t.Helper()
	var parameters []models.FunctionParamTFSDK
	if list.IsNull() || list.IsUnknown() {
		return parameters
	}
	if diags := list.ElementsAs(context.Background(), &parameters, false); diags.HasError() {
		t.Fatalf("ElementsAs() diagnostics = %v", diags)
	}
	return parameters
}

func columnsOf(t *testing.T, list types.List) []models.FunctionColumnTFSDK {
	t.Helper()
	var columns []models.FunctionColumnTFSDK
	if list.IsNull() || list.IsUnknown() {
		return columns
	}
	if diags := list.ElementsAs(context.Background(), &columns, false); diags.HasError() {
		t.Fatalf("ElementsAs() diagnostics = %v", diags)
	}
	return columns
}

// requestsOf returns every request with the given method.
func (m *functionMock) requestsOf(method string) []mockRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockRequest, 0)
	for _, request := range m.requests {
		if request.Method == method {
			requests = append(requests, request)
		}
	}
	return requests
}

func (m *functionMock) lastRequest(t *testing.T, method string) mockRequest {
	t.Helper()
	requests := m.requestsOf(method)
	if len(requests) == 0 {
		t.Fatalf("no %s request was sent to the API", method)
	}
	return requests[len(requests)-1]
}

// assertJSONEqual compares two JSON documents semantically.
func assertJSONEqual(t *testing.T, label string, got []byte, want string) {
	t.Helper()

	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("%s: sent body is not valid JSON: %v\n%s", label, err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("%s: expected body is not valid JSON: %v", label, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("%s mismatch\n got: %s\nwant: %s", label, mustMarshal(t, gotValue), mustMarshal(t, wantValue))
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return encoded
}

func parametersEqual(a, b []any) bool {
	aJSON, errA := json.Marshal(a)
	bJSON, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(aJSON) == string(bJSON)
}
