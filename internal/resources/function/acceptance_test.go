package function_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// These tests run the real provider against a mock of the Gravitino function API.
// They only run with TF_ACC=1 (`go test -run TestAcc ./internal/resources/function/...`).
// Their value over the unit tests is the framework contract check: a computed
// attribute that stays unknown (or a state that does not equal the plan) fails the
// apply with "provider produced inconsistent result after apply".

func functionTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

const configScalarFunction = `
resource "gravitino_function" "this" {
  metalake       = "probe_ml"
  catalog        = "probe_cat"
  schema         = "probe_sch"
  name           = "add_one"
  function_type  = "SCALAR"
  deterministic  = true
  comment        = "A simple scalar function that adds one"

  definitions = [
    {
      parameters = [
        {
          name      = "x"
          data_type = "integer"
        }
      ]
      return_type = "integer"

      impls = [
        {
          language = "SQL"
          runtime  = "SPARK"
          sql      = "x + 1"
        }
      ]
    }
  ]
}
`

// TestAccFunctionResource_CreateAndRead registers the spec's scalar function and
// reads it back. The response lower cases functionType and adds empty
// resources/properties objects, so this test fails if the provider does not
// normalize them (drift after apply).
func TestAccFunctionResource_CreateAndRead(t *testing.T) {
	mock := newFunctionMock(t)
	t.Setenv("GRAVITINO_URI", mock.server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: functionTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: configScalarFunction,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_function.this", "id", "probe_ml.probe_cat.probe_sch.add_one"),
					resource.TestCheckResourceAttr("gravitino_function.this", "function_type", "SCALAR"),
					resource.TestCheckResourceAttr("gravitino_function.this", "deterministic", "true"),
					resource.TestCheckResourceAttr("gravitino_function.this", "definitions.#", "1"),
					resource.TestCheckResourceAttr("gravitino_function.this", "definitions.0.parameters.0.data_type", "integer"),
					resource.TestCheckResourceAttr("gravitino_function.this", "definitions.0.return_type", "integer"),
					resource.TestCheckResourceAttr("gravitino_function.this", "definitions.0.impls.0.sql", "x + 1"),
				),
			},
			// The follow up plan must be empty: no drift after refresh.
			{
				Config:   configScalarFunction,
				PlanOnly: true,
			},
		},
	})
}

// TestAccFunctionResource_UpdateCommentAndImpl changes the comment (in place, with
// updateComment) and the SQL implementation (updateImpl).
func TestAccFunctionResource_UpdateCommentAndImpl(t *testing.T) {
	mock := newFunctionMock(t)
	t.Setenv("GRAVITINO_URI", mock.server.URL)

	updated := strings.Replace(configScalarFunction,
		`comment        = "A simple scalar function that adds one"`,
		`comment        = "This is a new comment"`, 1)
	updated = strings.Replace(updated, `sql      = "x + 1"`, `sql      = "x + 2"`, 1)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: functionTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: configScalarFunction,
			},
			{
				Config: updated,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_function.this", "comment", "This is a new comment"),
					resource.TestCheckResourceAttr("gravitino_function.this", "definitions.0.impls.0.sql", "x + 2"),
				),
			},
		},
	})

	requests := mock.requestsOf(http.MethodPut)
	if len(requests) == 0 {
		t.Fatal("no alter request was sent")
	}

	var alterRequest struct {
		Updates []map[string]any `json:"updates"`
	}
	if err := json.Unmarshal(requests[len(requests)-1].Body, &alterRequest); err != nil {
		t.Fatalf("failed to decode the alter request: %v", err)
	}
	types := make([]string, 0, len(alterRequest.Updates))
	for _, update := range alterRequest.Updates {
		types = append(types, update["@type"].(string))
	}
	for _, updateType := range types {
		switch updateType {
		case "updateComment", "addDefinition", "removeDefinition", "addImpl", "updateImpl", "removeImpl":
		default:
			t.Errorf("alter request contains @type %q, which Gravitino does not support for functions", updateType)
		}
	}
	if !slices.Contains(types, "updateComment") || !slices.Contains(types, "updateImpl") {
		t.Errorf("alter @types = %v, want updateComment and updateImpl", types)
	}
}

// TestAccFunctionResource_TableFunctionWithPythonImpl covers the TABLE function
// shape (returnColumns) and the PYTHON implementation branch.
func TestAccFunctionResource_TableFunctionWithPythonImpl(t *testing.T) {
	mock := newFunctionMock(t)
	t.Setenv("GRAVITINO_URI", mock.server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: functionTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_function" "series" {
  metalake      = "probe_ml"
  catalog       = "probe_cat"
  schema        = "probe_sch"
  name          = "generate_series"
  function_type = "TABLE"
  deterministic = true

  definitions = [
    {
      parameters = [
        { name = "start_val", data_type = "integer" },
        { name = "end_val", data_type = "integer" },
      ]

      return_columns = [
        { name = "value", data_type = "integer", comment = "The generated integer value" }
      ]

      impls = [
        {
          language   = "PYTHON"
          runtime    = "SPARK"
          handler    = "generate_series_handler"
          code_block = "def generate_series_handler(start_val, end_val):\n  for i in range(start_val, end_val + 1):\n    yield (i,)"
        }
      ]
    }
  ]
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_function.series", "function_type", "TABLE"),
					resource.TestCheckResourceAttr("gravitino_function.series", "definitions.0.return_columns.0.name", "value"),
					resource.TestCheckResourceAttr("gravitino_function.series", "definitions.0.impls.0.language", "PYTHON"),
					resource.TestCheckResourceAttr("gravitino_function.series", "definitions.0.impls.0.handler", "generate_series_handler"),
				),
			},
		},
	})
}

// TestAccFunctionResource_Import imports an existing function by its compound id.
func TestAccFunctionResource_Import(t *testing.T) {
	mock := newFunctionMock(t)
	t.Setenv("GRAVITINO_URI", mock.server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: functionTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: configScalarFunction,
			},
			{
				ResourceName:      "gravitino_function.this",
				ImportState:       true,
				ImportStateId:     "probe_ml.probe_cat.probe_sch.add_one",
				ImportStateVerify: true,
			},
		},
	})
}
