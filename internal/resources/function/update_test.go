package function_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/function"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// baseScalarFunction is the function of components/examples/FunctionResponse in a
// config shaped form (the plan of an update).
const baseScalarFunction = `{
  "name": "add_one",
  "functionType": "SCALAR",
  "deterministic": true,
  "comment": "A simple scalar function that adds one",
  "definitions": [
    {
      "parameters": [{"name": "x", "dataType": "integer"}],
      "returnType": "integer",
      "impls": [{"language": "SQL", "runtime": "SPARK", "sql": "x + 1"}]
    }
  ]
}`

// TestFunctionResourceUpdate_Payloads asserts the exact alter request
// (components/schemas/FunctionUpdatesRequest) for every update type that Gravitino
// supports, and that the resulting state matches the plan.
func TestFunctionResourceUpdate_Payloads(t *testing.T) {
	tests := []struct {
		name    string
		state   string
		plan    string
		updates string
	}{
		{
			name:    "comment",
			state:   normalizedFunction(t, baseScalarFunction),
			plan:    withComment(t, baseScalarFunction, "This is a new comment"),
			updates: `{"updates":[{"@type":"updateComment","newComment":"This is a new comment"}]}`,
		},
		{
			name:  "addDefinition",
			state: normalizedFunction(t, baseScalarFunction),
			plan:  withSecondDefinition(t, baseScalarFunction),
			updates: `{"updates":[{"@type":"addDefinition","definition":{
				"parameters":[{"name":"y","dataType":"integer"}],
				"returnType":"integer",
				"impls":[{"language":"SQL","runtime":"SPARK","sql":"y + 1"}]}}]}`,
		},
		{
			name:    "removeDefinition",
			state:   normalizedFunction(t, withSecondDefinition(t, baseScalarFunction)),
			plan:    baseScalarFunction,
			updates: `{"updates":[{"@type":"removeDefinition","parameters":[{"name":"y","dataType":"integer"}]}]}`,
		},
		{
			name:  "addImpl",
			state: normalizedFunction(t, baseScalarFunction),
			plan:  withSecondImpl(t, baseScalarFunction),
			updates: `{"updates":[{"@type":"addImpl","parameters":[{"name":"x","dataType":"integer"}],
				"implementation":{"language":"JAVA","runtime":"TRINO","className":"com.example.AddOneFunction"}}]}`,
		},
		{
			name:  "updateImpl",
			state: normalizedFunction(t, withSecondImpl(t, baseScalarFunction)),
			plan:  withUpdatedSecondImpl(t, withSecondImpl(t, baseScalarFunction)),
			updates: `{"updates":[{"@type":"updateImpl","parameters":[{"name":"x","dataType":"integer"}],
				"runtime":"TRINO","implementation":{"language":"JAVA","runtime":"TRINO","className":"com.example.AddOneFunctionV2"}}]}`,
		},
		{
			name:    "removeImpl",
			state:   normalizedFunction(t, withSecondImpl(t, baseScalarFunction)),
			plan:    baseScalarFunction,
			updates: `{"updates":[{"@type":"removeImpl","parameters":[{"name":"x","dataType":"integer"}],"runtime":"TRINO"}]}`,
		},
		{
			// The API has no "update definition" request: parameters are the
			// identity of a definition, so a changed return type is a replace.
			name:  "replacedDefinition",
			state: normalizedFunction(t, baseScalarFunction),
			plan:  withReturnType(t, baseScalarFunction, "long"),
			updates: `{"updates":[
				{"@type":"removeDefinition","parameters":[{"name":"x","dataType":"integer"}]},
				{"@type":"addDefinition","definition":{
					"parameters":[{"name":"x","dataType":"integer"}],
					"returnType":"long",
					"impls":[{"language":"SQL","runtime":"SPARK","sql":"x + 1"}]}}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemaObj := functionResourceSchema(t)
			mock := newFunctionMock(t)
			mock.store("add_one", decodeObject(t, tt.state))

			r := res.New()
			r.(*res.FunctionResource).SetClient(mock.client())

			plan := modelFromFunctionJSON(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", tt.plan)
			state := stateFor(t, schemaObj, modelFromFunctionJSON(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", tt.state))
			response := &resource.UpdateResponse{State: state}

			r.Update(context.Background(), resource.UpdateRequest{
				Plan:  planFor(t, schemaObj, plan),
				State: state,
			}, response)

			if response.Diagnostics.HasError() {
				t.Fatalf("Update() diagnostics = %v", response.Diagnostics)
			}

			requests := mock.requestsOf(http.MethodPut)
			if len(requests) != 1 {
				t.Fatalf("PUT requests = %d, want 1", len(requests))
			}
			if got, want := requests[0].Path, "/api/metalakes/probe_ml/catalogs/probe_cat/schemas/probe_sch/functions/add_one"; got != want {
				t.Errorf("alter path = %q, want %q", got, want)
			}
			assertJSONEqual(t, "alter request body", requests[0].Body, tt.updates)

			// The state after the alter must be the plan: the mock applies the
			// request to the stored function, so a wrong request also shows up here.
			var updated res.FunctionResourceModel
			if diags := response.State.Get(context.Background(), &updated); diags.HasError() {
				t.Fatalf("State.Get() diagnostics = %v", diags)
			}
			assertStateMatchesPlan(t, updated, plan)
		})
	}
}

// TestFunctionResourceUpdate_NoChangesSendsNoAlter proves the provider does not
// send an empty updates array (the server would reject {"updates":[]}).
func TestFunctionResourceUpdate_NoChangesSendsNoAlter(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	mock := newFunctionMock(t)
	function := normalizedFunction(t, baseScalarFunction)
	mock.store("add_one", decodeObject(t, function))

	r := res.New()
	r.(*res.FunctionResource).SetClient(mock.client())

	plan := modelFromFunctionJSON(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", function)
	state := stateFor(t, schemaObj, modelFromFunctionJSON(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", function))
	response := &resource.UpdateResponse{State: state}

	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  planFor(t, schemaObj, plan),
		State: state,
	}, response)

	if response.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics = %v", response.Diagnostics)
	}
	if requests := mock.requestsOf(http.MethodPut); len(requests) != 0 {
		t.Errorf("Update() sent an alter request without changes: %s", requests[0].Body)
	}
	if requests := mock.requestsOf(http.MethodGet); len(requests) != 1 {
		t.Errorf("GET requests = %d, want 1 refresh of the unchanged function", len(requests))
	}
}

// TestFunctionResourceUpdate_SpecExamplePayload asserts the alter body of
// components/schemas/UpdateFunctionCommentRequest, exactly as the spec documents
// it ({"@type":"updateComment","newComment":"This is a new comment"}).
func TestFunctionResourceUpdate_SpecExamplePayload(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	mock := newFunctionMock(t)
	mock.store("add_one", decodeObject(t, normalizedFunction(t, baseScalarFunction)))

	r := res.New()
	r.(*res.FunctionResource).SetClient(mock.client())

	plan := modelFromFunctionJSON(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", withComment(t, baseScalarFunction, "This is a new comment"))
	state := stateFor(t, schemaObj, modelFromFunctionJSON(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", normalizedFunction(t, baseScalarFunction)))
	response := &resource.UpdateResponse{State: state}

	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  planFor(t, schemaObj, plan),
		State: state,
	}, response)

	if response.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics = %v", response.Diagnostics)
	}
	assertJSONEqual(t, "alter request body", mock.lastRequest(t, http.MethodPut).Body,
		`{"updates":[{"@type":"updateComment","newComment":"This is a new comment"}]}`)
}

// assertStateMatchesPlan compares the attributes the API owns.
func assertStateMatchesPlan(t *testing.T, updated, plan res.FunctionResourceModel) {
	t.Helper()

	if !updated.Comment.Equal(plan.Comment) {
		t.Errorf("comment = %s, want %s", updated.Comment, plan.Comment)
	}
	if !updated.FunctionType.Equal(plan.FunctionType) {
		t.Errorf("function_type = %s, want %s", updated.FunctionType, plan.FunctionType)
	}
	if !updated.Definitions.Equal(plan.Definitions) {
		t.Errorf("definitions = %s, want %s", updated.Definitions, plan.Definitions)
	}
}

// ---------------------------------------------------------------------------
// document helpers
// ---------------------------------------------------------------------------

// normalizedFunction returns a function document as a real 1.3.0 server answers
// it (lower case functionType, empty resources/properties objects, an audit).
func normalizedFunction(t *testing.T, document string) string {
	t.Helper()
	return string(mustMarshal(t, normalizeFunction(decodeObject(t, document))))
}

func withComment(t *testing.T, document, comment string) string {
	t.Helper()
	return patchFunction(t, document, func(function map[string]any) {
		function["comment"] = comment
	})
}

func withSecondDefinition(t *testing.T, document string) string {
	t.Helper()
	return patchFunction(t, document, func(function map[string]any) {
		definitions, _ := function["definitions"].([]any)
		var second map[string]any
		if err := json.Unmarshal([]byte(`{
			"parameters": [{"name": "y", "dataType": "integer"}],
			"returnType": "integer",
			"impls": [{"language": "SQL", "runtime": "SPARK", "sql": "y + 1"}]
		}`), &second); err != nil {
			t.Fatalf("failed to decode second definition: %v", err)
		}
		function["definitions"] = append(definitions, second)
	})
}

func withSecondImpl(t *testing.T, document string) string {
	t.Helper()
	return patchImpls(t, document, func(impls []any) []any {
		return append(impls, map[string]any{
			"language":  "JAVA",
			"runtime":   "TRINO",
			"className": "com.example.AddOneFunction",
		})
	})
}

func withUpdatedSecondImpl(t *testing.T, document string) string {
	t.Helper()
	return patchImpls(t, document, func(impls []any) []any {
		updated := make([]any, 0, len(impls))
		for _, raw := range impls {
			impl, _ := raw.(map[string]any)
			if asString(impl["runtime"]) == "TRINO" {
				impl["className"] = "com.example.AddOneFunctionV2"
			}
			updated = append(updated, impl)
		}
		return updated
	})
}

func withReturnType(t *testing.T, document, returnType string) string {
	t.Helper()
	return patchFunction(t, document, func(function map[string]any) {
		definitions, _ := function["definitions"].([]any)
		if len(definitions) > 0 {
			definitions[0].(map[string]any)["returnType"] = returnType
		}
	})
}

func patchFunction(t *testing.T, document string, patch func(map[string]any)) string {
	t.Helper()
	function := decodeObject(t, document)
	patch(function)
	return string(mustMarshal(t, function))
}

func patchImpls(t *testing.T, document string, patch func([]any) []any) string {
	t.Helper()
	return patchFunction(t, document, func(function map[string]any) {
		definitions, _ := function["definitions"].([]any)
		definition, _ := definitions[0].(map[string]any)
		impls, _ := definition["impls"].([]any)
		definition["impls"] = patch(impls)
	})
}
