package function_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	dsfunction "github.com/gravitino/terraform-provider-gravitino/internal/datasources/function"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Payloads from the v1.3.0 spec, docs/open-api/functions.yaml -> components/examples.
const (
	specFunctionResponse = `{
  "code": 0,
  "function": {
    "name": "add_one",
    "functionType": "scalar",
    "deterministic": true,
    "comment": "A simple scalar function that adds one",
    "definitions": [
      {
        "parameters": [{"name": "x", "dataType": "integer"}],
        "returnType": "integer",
        "impls": [
          {
            "language": "SQL",
            "runtime": "SPARK",
            "resources": {"jars": [], "files": [], "archives": []},
            "properties": {},
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

	specFunctionListResponse = `{
  "code": 0,
  "functions": [
    {
      "name": "add_one",
      "functionType": "scalar",
      "deterministic": true,
      "comment": "A simple scalar function",
      "definitions": [
        {
          "parameters": [{"name": "x", "dataType": "integer"}],
          "returnType": "integer",
          "impls": [
            {
              "language": "SQL",
              "runtime": "SPARK",
              "resources": {"jars": [], "files": [], "archives": []},
              "properties": {},
              "sql": "x + 1"
            }
          ]
        }
      ],
      "audit": {"creator": "user1", "createTime": "2021-01-01T00:00:00Z"}
    }
  ]
}`
)

func TestFunctionDataSourceMetadata(t *testing.T) {
	d := dsfunction.NewFunctionDataSource()
	var req datasource.MetadataRequest
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), req, &resp)
	if resp.TypeName != "gravitino_function" {
		t.Errorf("Expected type name gravitino_function, got %s", resp.TypeName)
	}
}

func TestFunctionsDataSourceMetadata(t *testing.T) {
	d := dsfunction.NewFunctionsDataSource()
	var req datasource.MetadataRequest
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), req, &resp)
	if resp.TypeName != "gravitino_functions" {
		t.Errorf("Expected type name gravitino_functions, got %s", resp.TypeName)
	}
}

// TestFunctionDataSourceSchema_SpecFields checks the data source mirrors the
// Function contract, including the definitions attribute type.
func TestFunctionDataSourceSchema_SpecFields(t *testing.T) {
	d := dsfunction.NewFunctionDataSource()
	schemaResponse := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, schemaResponse)
	schemaObj := schemaResponse.Schema

	for _, name := range []string{"metalake", "catalog", "schema", "name", "function_type", "deterministic", "comment", "definitions", "audit"} {
		if _, ok := schemaObj.Attributes[name]; !ok {
			t.Errorf("schema is missing attribute %q", name)
		}
	}
	for _, removed := range []string{"function_body", "properties"} {
		if _, ok := schemaObj.Attributes[removed]; ok {
			t.Errorf("attribute %q is not part of the v1.3.0 Function and must not exist", removed)
		}
	}
	if got, want := schemaObj.Attributes["definitions"].GetType(), models.FunctionDefinitionsListType(); !got.Equal(want) {
		t.Errorf("definitions type = %s, want %s", got, want)
	}
}

// TestFunctionDataSourceRead reads components/examples/FunctionResponse.
func TestFunctionDataSourceRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/metalakes/probe_ml/catalogs/probe_cat/schemas/probe_sch/functions/add_one"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(specFunctionResponse))
	}))
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}

	d := dsfunction.NewFunctionDataSource()
	d.(*dsfunction.FunctionDataSource).SetClient(c)

	ctx := context.Background()
	schemaResponse := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResponse)
	schemaObj := schemaResponse.Schema

	config := dsfunction.FunctionDataSourceModel{
		Metalake:      types.StringValue("probe_ml"),
		Catalog:       types.StringValue("probe_cat"),
		Schema:        types.StringValue("probe_sch"),
		Name:          types.StringValue("add_one"),
		FunctionType:  types.StringNull(),
		Deterministic: types.BoolNull(),
		Comment:       types.StringNull(),
		Definitions:   types.ListNull(models.FunctionDefinitionObjectType()),
		Audit:         types.ObjectNull(auditAttrTypes(t, schemaObj)),
	}
	configValue, diags := types.ObjectValueFrom(ctx, schemaObj.Type().(types.ObjectType).AttributeTypes(), config)
	if diags.HasError() {
		t.Fatalf("ObjectValueFrom() diagnostics = %v", diags)
	}
	raw, err := configValue.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("ToTerraformValue() error = %v", err)
	}

	response := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: raw}}, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
	}

	var state dsfunction.FunctionDataSourceModel
	if diags := response.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("State.Get() diagnostics = %v", diags)
	}

	if got, want := state.FunctionType.ValueString(), "SCALAR"; got != want {
		t.Errorf("function_type = %q, want %q (the API answers %q)", got, want, "scalar")
	}
	if !state.Deterministic.ValueBool() {
		t.Error("deterministic = false, want true")
	}
	if got, want := state.Comment.ValueString(), "A simple scalar function that adds one"; got != want {
		t.Errorf("comment = %q, want %q", got, want)
	}
	if state.Audit.IsNull() {
		t.Error("audit = null, want the audit of the response")
	}

	var definitions []models.FunctionDefinitionTFSDK
	if diags := state.Definitions.ElementsAs(ctx, &definitions, false); diags.HasError() {
		t.Fatalf("definitions.ElementsAs() diagnostics = %v", diags)
	}
	if len(definitions) != 1 {
		t.Fatalf("definitions = %d, want 1", len(definitions))
	}
	if got, want := definitions[0].ReturnType.ValueString(), "integer"; got != want {
		t.Errorf("return_type = %q, want %q", got, want)
	}

	var impls []models.FunctionImplTFSDK
	if diags := definitions[0].Impls.ElementsAs(ctx, &impls, false); diags.HasError() {
		t.Fatalf("impls.ElementsAs() diagnostics = %v", diags)
	}
	if len(impls) != 1 || impls[0].SQL.ValueString() != "x + 1" {
		t.Errorf("impls = %+v, want the SQL implementation of the spec example", impls)
	}
	if !impls[0].Resources.IsNull() {
		t.Errorf("resources = %s, want null for the empty resources object of the response", impls[0].Resources)
	}
	if !impls[0].Properties.IsNull() {
		t.Errorf("properties = %s, want null for the empty properties object of the response", impls[0].Properties)
	}
}

// TestFunctionsDataSourceRead uses the documented details=true listing, which
// returns the function objects in one request
// (GET /metalakes/{m}/catalogs/{c}/schemas/{s}/functions?details=true).
func TestFunctionsDataSourceRead(t *testing.T) {
	var details string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		details = r.URL.Query().Get("details")
		if got, want := r.URL.Path, "/api/metalakes/probe_ml/catalogs/probe_cat/schemas/probe_sch/functions"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(specFunctionListResponse))
	}))
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}

	d := dsfunction.NewFunctionsDataSource()
	d.(*dsfunction.FunctionsDataSource).SetClient(c)

	ctx := context.Background()
	schemaResponse := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResponse)
	schemaObj := schemaResponse.Schema

	listType, ok := schemaObj.Attributes["functions"].GetType().(types.ListType)
	if !ok {
		t.Fatalf("functions attribute type = %T, want types.ListType", schemaObj.Attributes["functions"].GetType())
	}
	config := dsfunction.FunctionsDataSourceModel{
		Metalake:  types.StringValue("probe_ml"),
		Catalog:   types.StringValue("probe_cat"),
		Schema:    types.StringValue("probe_sch"),
		Functions: types.ListNull(listType.ElemType),
	}
	configValue, diags := types.ObjectValueFrom(ctx, schemaObj.Type().(types.ObjectType).AttributeTypes(), config)
	if diags.HasError() {
		t.Fatalf("ObjectValueFrom() diagnostics = %v", diags)
	}
	raw, err := configValue.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("ToTerraformValue() error = %v", err)
	}

	response := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: raw}}, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
	}

	if details != "true" {
		t.Errorf("details query parameter = %q, want true", details)
	}

	var state dsfunction.FunctionsDataSourceModel
	if diags := response.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("State.Get() diagnostics = %v", diags)
	}
	if state.Functions.IsNull() || len(state.Functions.Elements()) != 1 {
		t.Fatalf("functions = %s, want one function", state.Functions)
	}

	var items []struct {
		Name          types.String `tfsdk:"name"`
		FunctionType  types.String `tfsdk:"function_type"`
		Deterministic types.Bool   `tfsdk:"deterministic"`
		Comment       types.String `tfsdk:"comment"`
		Definitions   types.List   `tfsdk:"definitions"`
		Audit         types.Object `tfsdk:"audit"`
	}
	if diags := state.Functions.ElementsAs(ctx, &items, false); diags.HasError() {
		t.Fatalf("functions.ElementsAs() diagnostics = %v", diags)
	}
	if got, want := items[0].Name.ValueString(), "add_one"; got != want {
		t.Errorf("functions[0].name = %q, want %q", got, want)
	}
	if got, want := items[0].FunctionType.ValueString(), "SCALAR"; got != want {
		t.Errorf("functions[0].function_type = %q, want %q", got, want)
	}

	var definitions []models.FunctionDefinitionTFSDK
	if diags := items[0].Definitions.ElementsAs(ctx, &definitions, false); diags.HasError() {
		t.Fatalf("definitions.ElementsAs() diagnostics = %v", diags)
	}
	var impls []models.FunctionImplTFSDK
	if diags := definitions[0].Impls.ElementsAs(ctx, &impls, false); diags.HasError() {
		t.Fatalf("impls.ElementsAs() diagnostics = %v", diags)
	}
	if len(impls) != 1 || impls[0].SQL.ValueString() != "x + 1" || impls[0].Runtime.ValueString() != "SPARK" {
		t.Errorf("impls = %+v, want the SQL implementation of the spec example", impls)
	}
}

// TestFunctionDataSourceRead_NotFound surfaces a real Gravitino 404 with the
// server's error type.
func TestFunctionDataSourceRead_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":1003,"type":"NoSuchFunctionException","message":"Function does not exist",` +
			`"stack":["org.apache.gravitino.exceptions.NoSuchFunctionException: Function does not exist"]}`))
	}))
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}

	d := dsfunction.NewFunctionDataSource()
	d.(*dsfunction.FunctionDataSource).SetClient(c)

	ctx := context.Background()
	schemaResponse := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResponse)
	schemaObj := schemaResponse.Schema

	config := dsfunction.FunctionDataSourceModel{
		Metalake:      types.StringValue("probe_ml"),
		Catalog:       types.StringValue("probe_cat"),
		Schema:        types.StringValue("probe_sch"),
		Name:          types.StringValue("nope"),
		FunctionType:  types.StringNull(),
		Deterministic: types.BoolNull(),
		Comment:       types.StringNull(),
		Definitions:   types.ListNull(models.FunctionDefinitionObjectType()),
		Audit:         types.ObjectNull(auditAttrTypes(t, schemaObj)),
	}
	configValue, diags := types.ObjectValueFrom(ctx, schemaObj.Type().(types.ObjectType).AttributeTypes(), config)
	if diags.HasError() {
		t.Fatalf("ObjectValueFrom() diagnostics = %v", diags)
	}
	raw, err := configValue.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("ToTerraformValue() error = %v", err)
	}

	response := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: raw}}, response)

	if !response.Diagnostics.HasError() {
		t.Fatal("Read() did not report the missing function")
	}
	detail := ""
	for _, diagnostic := range response.Diagnostics.Errors() {
		detail += diagnostic.Summary() + " | " + diagnostic.Detail() + "\n"
	}
	if !strings.Contains(detail, "NoSuchFunctionException") || !strings.Contains(detail, "404") {
		t.Errorf("diagnostics do not mention the server error:\n%s", detail)
	}
}

// auditAttrTypes returns the audit attribute types of a data source schema.
func auditAttrTypes(t *testing.T, schemaObj schema.Schema) map[string]attr.Type {
	t.Helper()
	auditType, ok := schemaObj.Attributes["audit"].GetType().(types.ObjectType)
	if !ok {
		t.Fatalf("audit attribute type = %T, want types.ObjectType", schemaObj.Attributes["audit"].GetType())
	}
	return auditType.AttrTypes
}
