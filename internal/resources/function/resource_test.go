package function_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/function"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

func TestFunctionResourceMetadata(t *testing.T) {
	r := res.New()
	var req resource.MetadataRequest
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), req, &resp)
	if resp.TypeName != "gravitino_function" {
		t.Errorf("Expected type name gravitino_function, got %s", resp.TypeName)
	}
}

// TestFunctionResourceSchema_SpecFields pins the schema on the v1.3.0 contract
// (docs/open-api/functions.yaml) and on the attribute types the provider converts
// to, so a schema/model drift cannot slip through.
func TestFunctionResourceSchema_SpecFields(t *testing.T) {
	schemaObj := functionResourceSchema(t)

	for _, name := range []string{"id", "metalake", "catalog", "schema", "name", "function_type", "deterministic", "comment", "definitions", "audit"} {
		if _, ok := schemaObj.Attributes[name]; !ok {
			t.Errorf("schema is missing attribute %q", name)
		}
	}

	// functionBody and the function level properties of the old model do not exist
	// in the API: the server rejects them with UnrecognizedPropertyException.
	for _, removed := range []string{"function_body", "properties"} {
		if _, ok := schemaObj.Attributes[removed]; ok {
			t.Errorf("attribute %q is not part of the v1.3.0 Function and must not exist", removed)
		}
	}

	functionType, ok := schemaObj.Attributes["function_type"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("function_type = %T, want schema.StringAttribute", schemaObj.Attributes["function_type"])
	}
	if !functionType.Required {
		t.Error("function_type must be required")
	}
	if len(functionType.Validators) != 1 {
		t.Errorf("function_type validators = %d, want the OneOf validator with the spec enum", len(functionType.Validators))
	}

	definitions, ok := schemaObj.Attributes["definitions"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("definitions = %T, want schema.ListNestedAttribute", schemaObj.Attributes["definitions"])
	}
	if !definitions.Required {
		t.Error("definitions must be required (FunctionRegisterRequest requires at least one definition)")
	}
	if got, want := definitions.GetType(), models.FunctionDefinitionsListType(); !got.Equal(want) {
		t.Errorf("definitions type = %s, want %s", got, want)
	}
	if got := attributeNames(definitions.NestedObject.Attributes); !equalStrings(got, []string{"impls", "parameters", "return_columns", "return_type"}) {
		t.Errorf("definition attributes = %v, want [impls parameters return_columns return_type]", got)
	}

	parameters, ok := definitions.NestedObject.Attributes["parameters"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("definitions.parameters = %T, want schema.ListNestedAttribute", definitions.NestedObject.Attributes["parameters"])
	}
	if got, want := parameters.GetType(), (types.ListType{ElemType: types.ObjectType{AttrTypes: models.FunctionParamAttrTypes()}}); !got.Equal(want) {
		t.Errorf("parameters type = %s, want %s", got, want)
	}
	if got := attributeNames(parameters.NestedObject.Attributes); !equalStrings(got, []string{"comment", "data_type", "default_value", "name"}) {
		t.Errorf("parameter attributes = %v, want [comment data_type default_value name]", got)
	}
	if !parameters.NestedObject.Attributes["data_type"].IsRequired() {
		t.Error("definitions.parameters.data_type must be required (FunctionParam requires dataType)")
	}

	returnColumns, ok := definitions.NestedObject.Attributes["return_columns"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("definitions.return_columns = %T, want schema.ListNestedAttribute", definitions.NestedObject.Attributes["return_columns"])
	}
	if got, want := returnColumns.GetType(), (types.ListType{ElemType: types.ObjectType{AttrTypes: models.FunctionColumnAttrTypes()}}); !got.Equal(want) {
		t.Errorf("return_columns type = %s, want %s", got, want)
	}

	impls, ok := definitions.NestedObject.Attributes["impls"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("definitions.impls = %T, want schema.ListNestedAttribute", definitions.NestedObject.Attributes["impls"])
	}
	if got, want := impls.GetType(), (types.ListType{ElemType: types.ObjectType{AttrTypes: models.FunctionImplAttrTypes()}}); !got.Equal(want) {
		t.Errorf("impls type = %s, want %s", got, want)
	}
	if got := attributeNames(impls.NestedObject.Attributes); !equalStrings(got, []string{"class_name", "code_block", "handler", "language", "properties", "resources", "runtime", "sql"}) {
		t.Errorf("implementation attributes = %v, want [class_name code_block handler language properties resources runtime sql]", got)
	}
	for _, required := range []string{"language", "runtime"} {
		if !impls.NestedObject.Attributes[required].IsRequired() {
			t.Errorf("definitions.impls.%s must be required (the API requires it on every implementation)", required)
		}
	}

	resourcesAttribute, ok := impls.NestedObject.Attributes["resources"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("definitions.impls.resources = %T, want schema.SingleNestedAttribute", impls.NestedObject.Attributes["resources"])
	}
	if got, want := resourcesAttribute.GetType(), (types.ObjectType{AttrTypes: models.FunctionResourcesAttrTypes()}); !got.Equal(want) {
		t.Errorf("resources type = %s, want %s", got, want)
	}
	if got := attributeNames(resourcesAttribute.Attributes); !equalStrings(got, []string{"archives", "files", "jars"}) {
		t.Errorf("resources attributes = %v, want [archives files jars]", got)
	}

	// returnType, comment and impls are optional in FunctionDefinition.
	for _, optional := range []string{"return_type", "impls"} {
		if !definitions.NestedObject.Attributes[optional].IsOptional() {
			t.Errorf("definitions.%s must be optional", optional)
		}
	}

	audit, ok := schemaObj.Attributes["audit"].(schema.ObjectAttribute)
	if !ok {
		t.Fatalf("audit = %T, want schema.ObjectAttribute", schemaObj.Attributes["audit"])
	}
	if !audit.Computed {
		t.Error("audit must be computed")
	}
}

// TestFunctionResource_OnlySupportedUpdatesAreInPlace pins which attributes
// Gravitino can alter (FunctionUpdateRequest discriminator: updateComment,
// addDefinition, removeDefinition, addImpl, updateImpl, removeImpl) and which have
// no update request at all and therefore must replace the function.
func TestFunctionResource_OnlySupportedUpdatesAreInPlace(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	ctx := context.Background()
	base := modelFromFunctionJSON(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", baseScalarFunction)

	setValue := func(m *res.FunctionResourceModel, attribute, value string) {
		switch attribute {
		case "metalake":
			m.Metalake = types.StringValue(value)
		case "catalog":
			m.Catalog = types.StringValue(value)
		case "schema":
			m.Schema = types.StringValue(value)
		case "name":
			m.Name = types.StringValue(value)
		case "function_type":
			m.FunctionType = types.StringValue(value)
		default:
			t.Fatalf("unknown attribute %q", attribute)
		}
	}

	for _, tt := range []struct{ attribute, before, after string }{
		{"metalake", "probe_ml", "other_ml"},
		{"catalog", "probe_cat", "other_cat"},
		{"schema", "probe_sch", "other_sch"},
		{"name", "add_one", "add_two"},
		{"function_type", "SCALAR", "TABLE"},
	} {
		t.Run(tt.attribute, func(t *testing.T) {
			stateModel, planModel := base, base
			setValue(&stateModel, tt.attribute, tt.before)
			setValue(&planModel, tt.attribute, tt.after)

			attribute, ok := schemaObj.Attributes[tt.attribute].(schema.StringAttribute)
			if !ok {
				t.Fatalf("%s = %T, want schema.StringAttribute", tt.attribute, schemaObj.Attributes[tt.attribute])
			}

			request := planmodifier.StringRequest{
				Path:        path.Root(tt.attribute),
				State:       stateFor(t, schemaObj, stateModel),
				Plan:        planFor(t, schemaObj, planModel),
				Config:      tfsdk.Config{Schema: schemaObj, Raw: terraformValue(t, schemaObj, planModel)},
				StateValue:  types.StringValue(tt.before),
				PlanValue:   types.StringValue(tt.after),
				ConfigValue: types.StringValue(tt.after),
			}
			if !requiresReplaceString(ctx, attribute.PlanModifiers, request) {
				t.Errorf("%s changed in place, but Gravitino has no update request for it", tt.attribute)
			}
		})
	}

	t.Run("deterministic", func(t *testing.T) {
		stateModel, planModel := base, base
		stateModel.Deterministic = types.BoolValue(false)
		planModel.Deterministic = types.BoolValue(true)

		attribute, ok := schemaObj.Attributes["deterministic"].(schema.BoolAttribute)
		if !ok {
			t.Fatalf("deterministic = %T, want schema.BoolAttribute", schemaObj.Attributes["deterministic"])
		}
		if len(attribute.PlanModifiers) == 0 {
			t.Fatal("deterministic has no plan modifiers, want RequiresReplace")
		}

		request := planmodifier.BoolRequest{
			Path:        path.Root("deterministic"),
			State:       stateFor(t, schemaObj, stateModel),
			Plan:        planFor(t, schemaObj, planModel),
			Config:      tfsdk.Config{Schema: schemaObj, Raw: terraformValue(t, schemaObj, planModel)},
			StateValue:  types.BoolValue(false),
			PlanValue:   types.BoolValue(true),
			ConfigValue: types.BoolValue(true),
		}
		replace := false
		for _, modifier := range attribute.PlanModifiers {
			response := &planmodifier.BoolResponse{PlanValue: request.PlanValue}
			modifier.PlanModifyBool(ctx, request, response)
			replace = replace || response.RequiresReplace
		}
		if !replace {
			t.Error("deterministic changed in place, but Gravitino has no update request for it")
		}
	})

	t.Run("comment", func(t *testing.T) {
		stateModel, planModel := base, base
		stateModel.Comment = types.StringValue("before")
		planModel.Comment = types.StringValue("after")

		attribute, ok := schemaObj.Attributes["comment"].(schema.StringAttribute)
		if !ok {
			t.Fatalf("comment = %T, want schema.StringAttribute", schemaObj.Attributes["comment"])
		}
		if requiresReplaceString(ctx, attribute.PlanModifiers, planmodifier.StringRequest{
			Path:        path.Root("comment"),
			State:       stateFor(t, schemaObj, stateModel),
			Plan:        planFor(t, schemaObj, planModel),
			Config:      tfsdk.Config{Schema: schemaObj, Raw: terraformValue(t, schemaObj, planModel)},
			StateValue:  types.StringValue("before"),
			PlanValue:   types.StringValue("after"),
			ConfigValue: types.StringValue("after"),
		}) {
			t.Error("comment must be updated in place with the API's updateComment request")
		}
	})
}

// TestCommentPlanModifier covers the three comment states: an empty string is not
// a comment, a new comment is an in place update, and clearing a comment replaces
// the function because the alter endpoint rejects a null comment ("New comment
// cannot be null or empty", verified against Gravitino 1.3.0).
func TestCommentPlanModifier(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	comment := schemaObj.Attributes["comment"].(schema.StringAttribute)
	ctx := context.Background()

	tests := []struct {
		name        string
		state       types.String
		plan        types.String
		wantPlan    types.String
		wantReplace bool
	}{
		{name: "empty becomes null", state: types.StringNull(), plan: types.StringValue(""), wantPlan: types.StringNull()},
		{name: "new comment", state: types.StringNull(), plan: types.StringValue("hello"), wantPlan: types.StringValue("hello")},
		{name: "changed comment", state: types.StringValue("one"), plan: types.StringValue("two"), wantPlan: types.StringValue("two")},
		{name: "cleared comment replaces", state: types.StringValue("one"), plan: types.StringNull(), wantPlan: types.StringNull(), wantReplace: true},
		{name: "cleared via empty string replaces", state: types.StringValue("one"), plan: types.StringValue(""), wantPlan: types.StringNull(), wantReplace: true},
		{name: "still absent", state: types.StringNull(), plan: types.StringNull(), wantPlan: types.StringNull()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := planmodifier.StringRequest{
				Path:        path.Root("comment"),
				StateValue:  tt.state,
				PlanValue:   tt.plan,
				ConfigValue: tt.plan,
			}
			response := &planmodifier.StringResponse{PlanValue: tt.plan}
			for _, modifier := range comment.PlanModifiers {
				modifier.PlanModifyString(ctx, request, response)
			}

			if !response.PlanValue.Equal(tt.wantPlan) {
				t.Errorf("planned comment = %s, want %s", response.PlanValue, tt.wantPlan)
			}
			if response.RequiresReplace != tt.wantReplace {
				t.Errorf("RequiresReplace = %t, want %t", response.RequiresReplace, tt.wantReplace)
			}
		})
	}
}

// TestFunctionResourceRead_MapsSpecExample checks the refresh path against
// components/examples/FunctionResponse, including the server behaviours that a
// real 1.3.0 server shows: functionType comes back in lower case and every
// implementation carries empty resources/properties objects.
func TestFunctionResourceRead_MapsSpecExample(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	mock := newFunctionMock(t)
	mock.store("add_one", decodeObject(t, specFunctionResponse)["function"].(map[string]any))

	r := res.New()
	r.(*res.FunctionResource).SetClient(mock.client())

	state := stateFor(t, schemaObj, modelFromFunctionJSON(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", functionOf(t, specFunctionResponse)))
	response := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
	}

	var refreshed res.FunctionResourceModel
	if diags := response.State.Get(context.Background(), &refreshed); diags.HasError() {
		t.Fatalf("State.Get() diagnostics = %v", diags)
	}

	if got, want := refreshed.ID.ValueString(), "probe_ml.probe_cat.probe_sch.add_one"; got != want {
		t.Errorf("id = %q, want %q", got, want)
	}
	if got, want := refreshed.FunctionType.ValueString(), "SCALAR"; got != want {
		t.Errorf("function_type = %q, want %q (the API answers %q)", got, want, "scalar")
	}
	if !refreshed.Deterministic.ValueBool() {
		t.Error("deterministic = false, want true")
	}
	if got, want := refreshed.Comment.ValueString(), "A simple scalar function that adds one"; got != want {
		t.Errorf("comment = %q, want %q", got, want)
	}

	definitions := definitionsOf(t, refreshed.Definitions)
	if len(definitions) != 1 {
		t.Fatalf("definitions = %d, want 1", len(definitions))
	}
	parameters := parametersOf(t, definitions[0].Parameters)
	if len(parameters) != 1 || parameters[0].Name.ValueString() != "x" || parameters[0].DataType.ValueString() != "integer" {
		t.Errorf("parameters = %+v, want one parameter x of type integer", parameters)
	}
	if got, want := definitions[0].ReturnType.ValueString(), "integer"; got != want {
		t.Errorf("return_type = %q, want %q", got, want)
	}
	if !definitions[0].ReturnColumns.IsNull() {
		t.Errorf("return_columns = %s, want null for a SCALAR function", definitions[0].ReturnColumns)
	}

	impls := implsOf(t, definitions[0].Impls)
	if len(impls) != 1 {
		t.Fatalf("impls = %d, want 1", len(impls))
	}
	if got, want := impls[0].Language.ValueString(), "SQL"; got != want {
		t.Errorf("impl language = %q, want %q", got, want)
	}
	if got, want := impls[0].Runtime.ValueString(), "SPARK"; got != want {
		t.Errorf("impl runtime = %q, want %q", got, want)
	}
	if got, want := impls[0].SQL.ValueString(), "x + 1"; got != want {
		t.Errorf("impl sql = %q, want %q", got, want)
	}
	if !impls[0].Resources.IsNull() {
		t.Errorf("impl resources = %s, want null for an empty resources object", impls[0].Resources)
	}
	if !impls[0].Properties.IsNull() {
		t.Errorf("impl properties = %s, want null for an empty properties object", impls[0].Properties)
	}
	if refreshed.Audit.IsNull() {
		t.Error("audit = null, want the audit of the response")
	}
}

// TestFunctionResourceRead_NotFoundRemovesResource proves a real Gravitino 404
// (status 404 with {"code":1003,"type":"NoSuchFunctionException",...}) drops the
// resource from state instead of failing.
func TestFunctionResourceRead_NotFoundRemovesResource(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	mock := newFunctionMock(t)

	r := res.New()
	r.(*res.FunctionResource).SetClient(mock.client())

	state := stateFor(t, schemaObj, modelFromFunctionJSON(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", functionOf(t, specFunctionResponse)))
	response := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, response)

	if response.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v, want the 404 to be handled as a removed resource", response.Diagnostics)
	}
	if !response.State.Raw.IsNull() {
		t.Error("state was not removed for a 404")
	}

	requests := mock.requestsOf(http.MethodGet)
	if len(requests) != 1 || requests[0].Path != "/api/metalakes/probe_ml/catalogs/probe_cat/schemas/probe_sch/functions/add_one" {
		t.Errorf("GET requests = %+v, want one GET of the function path", requests)
	}
}

// TestFunctionResourceCreate_RegisterPayloadMatchesSpecExample sends the register
// request of components/examples/FunctionRegisterRequest (the implementation list
// matches the spec's FunctionResponse, see specRegisterRequest) and asserts the
// exact body, which the API validates field by field.
func TestFunctionResourceCreate_RegisterPayloadMatchesSpecExample(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	mock := newFunctionMock(t)

	r := res.New()
	r.(*res.FunctionResource).SetClient(mock.client())

	plan := modelFromRegisterRequest(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", specRegisterRequest)
	response := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{Plan: planFor(t, schemaObj, plan)}, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics = %v", response.Diagnostics)
	}

	registerRequests := mock.requestsOf(http.MethodPost)
	if len(registerRequests) != 1 {
		t.Fatalf("POST requests = %d, want 1", len(registerRequests))
	}
	if got, want := registerRequests[0].Path, "/api/metalakes/probe_ml/catalogs/probe_cat/schemas/probe_sch/functions"; got != want {
		t.Errorf("register path = %q, want %q", got, want)
	}
	assertJSONEqual(t, "register request body", registerRequests[0].Body, specRegisterRequest)

	var state res.FunctionResourceModel
	if diags := response.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("State.Get() diagnostics = %v", diags)
	}
	if got, want := state.ID.ValueString(), "probe_ml.probe_cat.probe_sch.add_one"; got != want {
		t.Errorf("id = %q, want %q", got, want)
	}
	if got, want := state.FunctionType.ValueString(), "SCALAR"; got != want {
		t.Errorf("function_type = %q, want %q", got, want)
	}
	definitions := definitionsOf(t, state.Definitions)
	if len(definitions) != 1 {
		t.Fatalf("definitions = %d, want 1", len(definitions))
	}
	impls := implsOf(t, definitions[0].Impls)
	if len(impls) != 1 || impls[0].SQL.ValueString() != "x + 1" {
		t.Errorf("impls = %+v, want the SQL implementation of the spec example", impls)
	}
}

// TestFunctionResourceCreate_JavaImplementationPayload covers the polymorphic
// JAVA branch of FunctionImpl, including the nested resources list. The spec's JAVA
// example runs on SPARK, but the server rejects two implementations on the same
// runtime, so it is registered on TRINO here.
func TestFunctionResourceCreate_JavaImplementationPayload(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	mock := newFunctionMock(t)

	r := res.New()
	r.(*res.FunctionResource).SetClient(mock.client())

	plan := modelFromRegisterRequest(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", specRegisterRequestWithJavaImpl)
	response := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{Plan: planFor(t, schemaObj, plan)}, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics = %v", response.Diagnostics)
	}

	assertJSONEqual(t, "register request body", mock.lastRequest(t, http.MethodPost).Body, specRegisterRequestWithJavaImpl)

	var state res.FunctionResourceModel
	if diags := response.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("State.Get() diagnostics = %v", diags)
	}
	definitions := definitionsOf(t, state.Definitions)
	impls := implsOf(t, definitions[0].Impls)
	if len(impls) != 1 {
		t.Fatalf("impls = %d, want 1", len(impls))
	}
	if got, want := impls[0].ClassName.ValueString(), "com.example.AddOneFunction"; got != want {
		t.Errorf("class_name = %q, want %q", got, want)
	}
	if impls[0].Resources.IsNull() {
		t.Fatal("resources = null, want the configured jars")
	}
	var resources models.FunctionResourcesTFSDK
	if diags := impls[0].Resources.As(context.Background(), &resources, basetypes.ObjectAsOptions{}); diags.HasError() {
		t.Fatalf("resources.As() diagnostics = %v", diags)
	}
	var jars []string
	if diags := resources.Jars.ElementsAs(context.Background(), &jars, false); diags.HasError() {
		t.Fatalf("jars.ElementsAs() diagnostics = %v", diags)
	}
	if len(jars) != 1 || jars[0] != "hdfs:///path/to/udf.jar" {
		t.Errorf("resources.jars = %v, want the jar of the spec example", jars)
	}
}

// TestFunctionResourceCreate_TableFunctionPayload covers returnColumns and the
// PYTHON implementation of components/examples/TableFunctionRegisterRequest.
func TestFunctionResourceCreate_TableFunctionPayload(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	mock := newFunctionMock(t)

	r := res.New()
	r.(*res.FunctionResource).SetClient(mock.client())

	plan := modelFromRegisterRequest(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", specTableRegisterRequest)
	response := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{Plan: planFor(t, schemaObj, plan)}, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics = %v", response.Diagnostics)
	}

	assertJSONEqual(t, "register request body", mock.lastRequest(t, http.MethodPost).Body, specTableRegisterRequest)

	var state res.FunctionResourceModel
	if diags := response.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("State.Get() diagnostics = %v", diags)
	}
	if got, want := state.FunctionType.ValueString(), "TABLE"; got != want {
		t.Errorf("function_type = %q, want %q", got, want)
	}

	definitions := definitionsOf(t, state.Definitions)
	if !definitions[0].ReturnType.IsNull() {
		t.Errorf("return_type = %s, want null for a TABLE function", definitions[0].ReturnType)
	}
	columns := columnsOf(t, definitions[0].ReturnColumns)
	if len(columns) != 1 || columns[0].Name.ValueString() != "value" || columns[0].Comment.ValueString() != "The generated integer value" {
		t.Errorf("return_columns = %+v, want the value column of the spec example", columns)
	}
	impls := implsOf(t, definitions[0].Impls)
	if len(impls) != 1 {
		t.Fatalf("impls = %d, want 1", len(impls))
	}
	if got, want := impls[0].Language.ValueString(), "PYTHON"; got != want {
		t.Errorf("impl language = %q, want %q", got, want)
	}
	if got, want := impls[0].Handler.ValueString(), "generate_series_handler"; got != want {
		t.Errorf("impl handler = %q, want %q", got, want)
	}
	if !strings.Contains(impls[0].CodeBlock.ValueString(), "yield (i,)") {
		t.Errorf("impl code_block = %q, want the Python code block of the spec example", impls[0].CodeBlock.ValueString())
	}
}

// TestFunctionResourceCreate_ComplexDataTypeKeepsConfiguredSpelling registers a
// definition with the JSON document forms of datatype.yaml (a list parameter and a
// struct return type with a literal default value binding) and checks that the
// server's re-encoded answer does not show up as drift.
func TestFunctionResourceCreate_ComplexDataTypeKeepsConfiguredSpelling(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	mock := newFunctionMock(t)

	r := res.New()
	r.(*res.FunctionResource).SetClient(mock.client())

	request := `{
	  "name": "fn_complex",
	  "functionType": "SCALAR",
	  "deterministic": false,
	  "definitions": [
	    {
	      "parameters": [
	        {
	          "name": "nums",
	          "dataType": {"type": "list", "elementType": "integer", "containsNull": false},
	          "comment": "the numbers",
	          "defaultValue": {"type": "literal", "dataType": "integer", "value": "1"}
	        }
	      ],
	      "returnType": {"type": "struct", "fields": [{"name": "id", "type": "integer", "nullable": false, "comment": "the id"}]},
	      "impls": [{"language": "SQL", "runtime": "SPARK", "sql": "nums"}]
	    }
	  ]
	}`

	plan := modelFromRegisterRequest(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", request)
	response := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{Plan: planFor(t, schemaObj, plan)}, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics = %v", response.Diagnostics)
	}
	assertJSONEqual(t, "register request body", mock.lastRequest(t, http.MethodPost).Body, request)

	var state res.FunctionResourceModel
	if diags := response.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("State.Get() diagnostics = %v", diags)
	}

	definitions := definitionsOf(t, state.Definitions)
	parameters := parametersOf(t, definitions[0].Parameters)
	if got, want := parameters[0].DataType.ValueString(), `{"type": "list", "elementType": "integer", "containsNull": false}`; got != want {
		t.Errorf("data_type = %q, want the configured document %q", got, want)
	}
	if got, want := parameters[0].DefaultValue.ValueString(), `{"type": "literal", "dataType": "integer", "value": "1"}`; got != want {
		t.Errorf("default_value = %q, want the configured document %q", got, want)
	}
	if got, want := definitions[0].ReturnType.ValueString(), `{"type": "struct", "fields": [{"name": "id", "type": "integer", "nullable": false, "comment": "the id"}]}`; got != want {
		t.Errorf("return_type = %q, want the configured document %q", got, want)
	}
}

// TestFunctionResourceCreate_RejectsDuplicateRuntime mirrors the server rule
// ("Each definition must have at most one implementation per runtime") with an
// actionable message instead of a 400.
func TestFunctionResourceCreate_RejectsDuplicateRuntime(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	mock := newFunctionMock(t)

	r := res.New()
	r.(*res.FunctionResource).SetClient(mock.client())

	request := `{
	  "name": "fn_dupe",
	  "functionType": "SCALAR",
	  "deterministic": true,
	  "definitions": [
	    {
	      "parameters": [{"name": "x", "dataType": "integer"}],
	      "returnType": "integer",
	      "impls": [
	        {"language": "SQL", "runtime": "SPARK", "sql": "x + 1"},
	        {"language": "JAVA", "runtime": "SPARK", "className": "com.example.AddOne"}
	      ]
	    }
	  ]
	}`

	plan := modelFromRegisterRequest(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", request)
	response := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{Plan: planFor(t, schemaObj, plan)}, response)

	if !response.Diagnostics.HasError() {
		t.Fatal("Create() accepted two implementations on the same runtime")
	}
	if len(mock.requestsOf(http.MethodPost)) != 0 {
		t.Error("Create() sent an invalid request to the API")
	}
	if detail := response.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "at most one implementation per runtime") {
		t.Errorf("diagnostic = %q, want an explanation of the runtime uniqueness rule", detail)
	}
}

// TestFunctionResourceCreate_RejectsImplementationWithoutRequiredField covers the
// language specific required fields (SQLImpl requires sql, JavaImpl requires
// className).
func TestFunctionResourceCreate_RejectsImplementationWithoutRequiredField(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	mock := newFunctionMock(t)

	r := res.New()
	r.(*res.FunctionResource).SetClient(mock.client())

	request := `{
	  "name": "fn_incomplete",
	  "functionType": "SCALAR",
	  "definitions": [
	    {
	      "parameters": [{"name": "x", "dataType": "integer"}],
	      "returnType": "integer",
	      "impls": [{"language": "SQL", "runtime": "SPARK"}]
	    }
	  ]
	}`

	plan := modelFromRegisterRequest(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", request)
	response := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{Plan: planFor(t, schemaObj, plan)}, response)

	if !response.Diagnostics.HasError() {
		t.Fatal("Create() accepted a SQL implementation without sql")
	}
	if len(mock.requestsOf(http.MethodPost)) != 0 {
		t.Error("Create() sent an incomplete request to the API")
	}
}

func TestFunctionResourceDelete(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	mock := newFunctionMock(t)
	mock.store("add_one", decodeObject(t, specFunctionResponse)["function"].(map[string]any))

	r := res.New()
	r.(*res.FunctionResource).SetClient(mock.client())

	state := stateFor(t, schemaObj, modelFromFunctionJSON(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", functionOf(t, specFunctionResponse)))
	response := &resource.DeleteResponse{State: state}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, response)

	if response.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics = %v", response.Diagnostics)
	}
	if requests := mock.requestsOf(http.MethodDelete); len(requests) != 1 ||
		requests[0].Path != "/api/metalakes/probe_ml/catalogs/probe_cat/schemas/probe_sch/functions/add_one" {
		t.Errorf("DELETE requests = %+v, want one DELETE of the function path without query parameters", requests)
	}
	if _, ok := mock.load("add_one"); ok {
		t.Error("the function still exists after Delete()")
	}
}

// TestFunctionResourceDelete_NotFoundIsSuccess keeps Delete idempotent.
func TestFunctionResourceDelete_NotFoundIsSuccess(t *testing.T) {
	schemaObj := functionResourceSchema(t)
	mock := newFunctionMock(t)

	r := res.New()
	r.(*res.FunctionResource).SetClient(mock.client())

	state := stateFor(t, schemaObj, modelFromFunctionJSON(t, schemaObj, "probe_ml", "probe_cat", "probe_sch", functionOf(t, specFunctionResponse)))
	response := &resource.DeleteResponse{State: state}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, response)

	if response.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics = %v, want a 404 to be treated as success", response.Diagnostics)
	}
}

func TestFunctionResourceImportState(t *testing.T) {
	schemaObj := functionResourceSchema(t)

	tests := []struct {
		name      string
		id        string
		wantError bool
		wantParts []string
	}{
		{name: "valid", id: "ml.cat.sch.fn", wantParts: []string{"ml", "cat", "sch", "fn"}},
		{name: "missing function", id: "ml.cat.sch", wantError: true},
		{name: "missing schema", id: "ml.cat", wantError: true},
		{name: "only metalake", id: "ml", wantError: true},
		{name: "empty", id: "", wantError: true},
		{name: "empty segment", id: "ml..sch.fn", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := res.New().(resource.ResourceWithImportState)
			nullModel := res.FunctionResourceModel{
				ID:            types.StringNull(),
				Metalake:      types.StringNull(),
				Catalog:       types.StringNull(),
				Schema:        types.StringNull(),
				Name:          types.StringNull(),
				FunctionType:  types.StringNull(),
				Deterministic: types.BoolNull(),
				Comment:       types.StringNull(),
				Definitions:   types.ListNull(models.FunctionDefinitionObjectType()),
				Audit:         auditNullValue(t, schemaObj),
			}
			response := &resource.ImportStateResponse{State: tfsdk.State{Schema: schemaObj, Raw: terraformValue(t, schemaObj, nullModel)}}
			r.ImportState(context.Background(), resource.ImportStateRequest{ID: tt.id}, response)

			if response.Diagnostics.HasError() != tt.wantError {
				t.Fatalf("ImportState(%q) diagnostics = %v, wantError = %t", tt.id, response.Diagnostics, tt.wantError)
			}
			if tt.wantError {
				return
			}

			for i, part := range tt.wantParts {
				var value types.String
				if diags := response.State.GetAttribute(context.Background(), path.Root([]string{"metalake", "catalog", "schema", "name"}[i]), &value); diags.HasError() {
					t.Fatalf("GetAttribute() diagnostics = %v", diags)
				}
				if value.ValueString() != part {
					t.Errorf("imported segment %d = %q, want %q", i, value.ValueString(), part)
				}
			}
		})
	}
}

func requiresReplaceString(ctx context.Context, modifiers []planmodifier.String, request planmodifier.StringRequest) bool {
	replace := false
	for _, modifier := range modifiers {
		response := &planmodifier.StringResponse{PlanValue: request.PlanValue}
		modifier.PlanModifyString(ctx, request, response)
		replace = replace || response.RequiresReplace
	}
	return replace
}

func attributeNames(attributes map[string]schema.Attribute) []string {
	names := make([]string, 0, len(attributes))
	for name := range attributes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func decodeObject(t *testing.T, document string) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatalf("failed to decode %q: %v", document, err)
	}
	return decoded
}

// functionOf returns the function object of a FunctionResponse example.
func functionOf(t *testing.T, response string) string {
	t.Helper()
	return string(mustMarshal(t, decodeObject(t, response)["function"]))
}
