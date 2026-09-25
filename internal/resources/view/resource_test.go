package view_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	resourceview "github.com/gravitino/terraform-provider-gravitino/internal/resources/view"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// testViewAudit is the audit block of the ViewResponse example in views.yaml.
func testViewAudit() *models.Audit {
	create, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return &models.Audit{
		Creator:          "gravitino",
		CreateTime:       &create,
		LastModifier:     "gravitino",
		LastModifiedTime: &create,
	}
}

// testViewResponse is the ViewResponse example in views.yaml.
func testViewResponse(name string) models.ViewResponse {
	return models.ViewResponse{
		Code: 0,
		View: models.View{
			Name:    name,
			Comment: "This is a view",
			Columns: []models.Column{
				{Name: "id", Type: models.DataType{Type: "long"}, Comment: "id column", Nullable: true},
			},
			Representations: []models.ViewRepresentation{
				{Type: "sql", Dialect: "trino", SQL: "SELECT id FROM t"},
			},
			Properties: map[string]string{"key": "value"},
			Audit:      testViewAudit(),
		},
	}
}

func newViewResource(t *testing.T, serverURL string) *resourceview.ViewResource {
	t.Helper()
	c, err := client.New(serverURL, nil)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}
	r := resourceview.NewViewResource()
	r.(*resourceview.ViewResource).SetClient(c)
	return r.(*resourceview.ViewResource)
}

func viewSchema(t *testing.T, r *resourceview.ViewResource) rschema.Schema {
	t.Helper()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func viewObjectType(t *testing.T, schemaObj rschema.Schema) types.ObjectType {
	t.Helper()
	objType, ok := schemaObj.Type().(types.ObjectType)
	if !ok {
		t.Fatalf("expected object type, got %T", schemaObj.Type())
	}
	return objType
}

// viewCreateModel builds a plan that mirrors the ViewCreateRequest example in
// views.yaml.
func viewCreateModel() resourceview.ViewResourceModel {
	return resourceview.ViewResourceModel{
		Metalake: types.StringValue("ml"),
		Catalog:  types.StringValue("cat"),
		Schema:   types.StringValue("sch"),
		Name:     types.StringValue("view1"),
		Comment:  types.StringValue("This is a view"),
		Columns: []models.ColumnTFSDK{
			{
				Name:          types.StringValue("id"),
				Type:          types.StringValue("long"),
				Comment:       types.StringValue("id column"),
				Nullable:      types.BoolValue(true),
				AutoIncrement: types.BoolValue(false),
			},
		},
		Representations: []models.ViewRepresentationTFSDK{
			{
				Type:    types.StringValue("sql"),
				Dialect: types.StringValue("trino"),
				SQL:     types.StringValue("SELECT id FROM t"),
			},
		},
		DefaultCatalog: types.StringNull(),
		DefaultSchema:  types.StringNull(),
		Properties: types.MapValueMust(types.StringType, map[string]attr.Value{
			"key": types.StringValue("value"),
		}),
		Audit: types.ObjectNull(models.AuditAttrTypes),
	}
}

func TestViewResourceMetadata(t *testing.T) {
	r := resourceview.NewViewResource()
	var req resource.MetadataRequest
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), req, &resp)
	if resp.TypeName != "gravitino_view" {
		t.Errorf("Expected type name gravitino_view, got %s", resp.TypeName)
	}
}

func TestViewResourceSchema(t *testing.T) {
	schemaObj := viewSchema(t, newViewResource(t, "http://127.0.0.1:1"))

	for _, name := range []string{
		"id", "metalake", "catalog", "schema", "name", "comment",
		"column", "representation", "default_catalog", "default_schema", "properties", "audit",
	} {
		if _, ok := schemaObj.Attributes[name]; !ok {
			t.Errorf("schema is missing attribute %q", name)
		}
	}

	representation, ok := schemaObj.Attributes["representation"].(rschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("representation is not a list nested attribute: %T", schemaObj.Attributes["representation"])
	}
	if !representation.Required {
		t.Error("representation must be required (views.yaml ViewCreateRequest minItems 1)")
	}
	if len(representation.Validators) == 0 {
		t.Error("representation must validate at least one element")
	}
	for _, attr := range []string{"type", "dialect", "sql"} {
		if _, ok := representation.NestedObject.Attributes[attr]; !ok {
			t.Errorf("representation is missing %q", attr)
		}
	}
	if len(representation.NestedObject.Attributes["type"].(rschema.StringAttribute).Validators) == 0 {
		t.Error("representation.type must enforce the spec enum")
	}

	column, ok := schemaObj.Attributes["column"].(rschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("column is not a list nested attribute: %T", schemaObj.Attributes["column"])
	}
	if !column.Optional || !column.Computed {
		t.Error("column must be optional+computed so Gravitino derived columns do not drift")
	}
	for _, attr := range []string{"name", "type", "comment", "nullable", "auto_increment", "default_value"} {
		if _, ok := column.NestedObject.Attributes[attr]; !ok {
			t.Errorf("column is missing %q", attr)
		}
	}

	if _, ok := schemaObj.Attributes["view_def"]; ok {
		t.Error("view_def must not exist: Gravitino v1.3.0 has no viewDef field")
	}
}

// TestViewResourceCreatePayload asserts the exact request body sent to
// Gravitino. The expectation is the ViewCreateRequest example from views.yaml.
func TestViewResourceCreatePayload(t *testing.T) {
	var received []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/metalakes/ml/catalogs/cat/schemas/sch/views" {
			http.NotFound(w, r)
			return
		}
		received, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_ = json.NewEncoder(w).Encode(testViewResponse("view1"))
	}))
	defer server.Close()

	r := newViewResource(t, server.URL)
	ctx := context.Background()
	schemaObj := viewSchema(t, r)

	planModel := viewCreateModel()
	planObj, diags := types.ObjectValueFrom(ctx, viewObjectType(t, schemaObj).AttributeTypes(), planModel)
	if diags.HasError() {
		t.Fatalf("failed to build plan: %v", diags)
	}
	tfVal, err := planObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert plan: %v", err)
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: schemaObj, Raw: tfVal}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	const want = `{
		"name": "view1",
		"comment": "This is a view",
		"columns": [
			{"name": "id", "type": "long", "comment": "id column", "nullable": true, "autoIncrement": false}
		],
		"representations": [
			{"type": "sql", "dialect": "trino", "sql": "SELECT id FROM t"}
		],
		"properties": {"key": "value"}
	}`

	assertJSONEqual(t, want, received)

	var state resourceview.ViewResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.ID.ValueString() != "ml.cat.sch.view1" {
		t.Errorf("id = %q, want ml.cat.sch.view1", state.ID.ValueString())
	}
	if len(state.Columns) != 1 || state.Columns[0].Type.ValueString() != "long" {
		t.Errorf("columns not mapped back into state: %#v", state.Columns)
	}
	if len(state.Representations) != 1 || state.Representations[0].SQL.ValueString() != "SELECT id FROM t" {
		t.Errorf("representations not mapped back into state: %#v", state.Representations)
	}
	if state.Audit.IsNull() {
		t.Error("audit must be populated from the response")
	}
}

// TestViewResourceUpdatePayload asserts the exact update request body for an
// in-place rename plus property changes.
func TestViewResourceUpdatePayload(t *testing.T) {
	var received []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/metalakes/ml/catalogs/cat/schemas/sch/views/view1" {
			http.NotFound(w, r)
			return
		}
		received, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		resp := testViewResponse("view2")
		resp.View.Properties = map[string]string{"key": "value", "env": "dev"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	r := newViewResource(t, server.URL)
	ctx := context.Background()
	schemaObj := viewSchema(t, r)
	attrTypes := viewObjectType(t, schemaObj).AttributeTypes()

	stateModel := viewCreateModel()
	stateModel.ID = types.StringValue("ml.cat.sch.view1")

	planModel := viewCreateModel()
	planModel.Name = types.StringValue("view2")
	planModel.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{
		"key": types.StringValue("value"),
		"env": types.StringValue("dev"),
	})

	stateObj, diags := types.ObjectValueFrom(ctx, attrTypes, stateModel)
	if diags.HasError() {
		t.Fatalf("failed to build state: %v", diags)
	}
	stateVal, err := stateObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert state: %v", err)
	}
	planObj, diags := types.ObjectValueFrom(ctx, attrTypes, planModel)
	if diags.HasError() {
		t.Fatalf("failed to build plan: %v", diags)
	}
	planVal, err := planObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert plan: %v", err)
	}

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaObj}}
	req := resource.UpdateRequest{
		State: tfsdk.State{Schema: schemaObj, Raw: stateVal},
		Plan:  tfsdk.Plan{Schema: schemaObj, Raw: planVal},
	}
	r.Update(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	const want = `{
		"updates": [
			{"@type": "rename", "newName": "view2"},
			{"@type": "setProperty", "property": "env", "value": "dev"}
		]
	}`
	assertJSONEqual(t, want, received)

	var state resourceview.ViewResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.Name.ValueString() != "view2" {
		t.Errorf("name = %q, want view2", state.Name.ValueString())
	}
	if state.ID.ValueString() != "ml.cat.sch.view2" {
		t.Errorf("id = %q, want ml.cat.sch.view2", state.ID.ValueString())
	}
}

// TestViewResourceUpdateRemovesProperty asserts that dropped keys are sent as
// removeProperty updates.
func TestViewResourceUpdateRemovesProperty(t *testing.T) {
	var received []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		received, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		resp := testViewResponse("view1")
		resp.View.Properties = map[string]string{"env": "dev"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	r := newViewResource(t, server.URL)
	ctx := context.Background()
	schemaObj := viewSchema(t, r)
	attrTypes := viewObjectType(t, schemaObj).AttributeTypes()

	stateModel := viewCreateModel()
	stateModel.ID = types.StringValue("ml.cat.sch.view1")

	planModel := viewCreateModel()
	planModel.Properties = types.MapValueMust(types.StringType, map[string]attr.Value{
		"env": types.StringValue("dev"),
	})

	stateObj, _ := types.ObjectValueFrom(ctx, attrTypes, stateModel)
	stateVal, _ := stateObj.ToTerraformValue(ctx)
	planObj, _ := types.ObjectValueFrom(ctx, attrTypes, planModel)
	planVal, _ := planObj.ToTerraformValue(ctx)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Update(ctx, resource.UpdateRequest{
		State: tfsdk.State{Schema: schemaObj, Raw: stateVal},
		Plan:  tfsdk.Plan{Schema: schemaObj, Raw: planVal},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	const want = `{
		"updates": [
			{"@type": "removeProperty", "property": "key"},
			{"@type": "setProperty", "property": "env", "value": "dev"}
		]
	}`
	assertJSONEqual(t, want, received)
}

func TestViewResourceDelete(t *testing.T) {
	var deletedPaths []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		deletedPaths = append(deletedPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if len(deletedPaths) == 1 {
			_ = json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
			return
		}
		// Second delete: already gone, must be treated as success.
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(noSuchViewBody))
	}))
	defer server.Close()

	r := newViewResource(t, server.URL)
	ctx := context.Background()
	schemaObj := viewSchema(t, r)

	stateModel := viewCreateModel()
	stateModel.ID = types.StringValue("ml.cat.sch.view1")
	stateObj, _ := types.ObjectValueFrom(ctx, viewObjectType(t, schemaObj).AttributeTypes(), stateModel)
	stateVal, _ := stateObj.ToTerraformValue(ctx)

	for range 2 {
		resp := &resource.DeleteResponse{State: tfsdk.State{Schema: schemaObj, Raw: stateVal}}
		r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: schemaObj, Raw: stateVal}}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("delete diagnostics: %v", resp.Diagnostics)
		}
	}

	wantPath := "/api/metalakes/ml/catalogs/cat/schemas/sch/views/view1"
	for _, p := range deletedPaths {
		if p != wantPath {
			t.Errorf("delete path = %q, want %q", p, wantPath)
		}
	}
	if len(deletedPaths) != 2 {
		t.Errorf("expected 2 delete calls, got %d", len(deletedPaths))
	}
}

// noSuchViewBody is the NoSuchViewException example from views.yaml.
const noSuchViewBody = `{
	"code": 1003,
	"type": "NoSuchViewException",
	"message": "Failed to operate view(s) [test_view] operation [LOAD] under schema [test_schema], reason [NoSuchViewException]",
	"stack": [
		"org.apache.gravitino.exceptions.NoSuchViewException: View test_view does not exist",
		"..."
	]
}`

// TestViewResourceReadNotFoundRemovesState proves that a real Gravitino 404
// removes the view from state instead of failing.
func TestViewResourceReadNotFoundRemovesState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(noSuchViewBody))
	}))
	defer server.Close()

	r := newViewResource(t, server.URL)
	ctx := context.Background()
	schemaObj := viewSchema(t, r)

	stateModel := viewCreateModel()
	stateModel.ID = types.StringValue("ml.cat.sch.view1")
	stateObj, _ := types.ObjectValueFrom(ctx, viewObjectType(t, schemaObj).AttributeTypes(), stateModel)
	stateVal, _ := stateObj.ToTerraformValue(ctx)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: schemaObj, Raw: stateVal}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: schemaObj, Raw: stateVal}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a 404 must remove the view from state")
	}
}

func TestViewResourceReadPopulatesState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/metalakes/ml/catalogs/cat/schemas/sch/views/view1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_ = json.NewEncoder(w).Encode(testViewResponse("view1"))
	}))
	defer server.Close()

	r := newViewResource(t, server.URL)
	ctx := context.Background()
	schemaObj := viewSchema(t, r)

	stateModel := viewCreateModel()
	stateModel.ID = types.StringValue("ml.cat.sch.view1")
	stateObj, _ := types.ObjectValueFrom(ctx, viewObjectType(t, schemaObj).AttributeTypes(), stateModel)
	stateVal, _ := stateObj.ToTerraformValue(ctx)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: schemaObj, Raw: stateVal}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: schemaObj, Raw: stateVal}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var state resourceview.ViewResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.Comment.ValueString() != "This is a view" {
		t.Errorf("comment = %q", state.Comment.ValueString())
	}
	if len(state.Representations) != 1 || state.Representations[0].Dialect.ValueString() != "trino" {
		t.Errorf("representations = %#v", state.Representations)
	}
	if state.Properties.IsNull() {
		t.Error("properties must be populated")
	}
}

func TestViewImportIDParsing(t *testing.T) {
	tests := []struct {
		id          string
		wantParts   []string
		expectError bool
	}{
		{"metalake.catalog.schema.view", []string{"metalake", "catalog", "schema", "view"}, false},
		{"ml.ctlg.sch.vw", []string{"ml", "ctlg", "sch", "vw"}, false},
		{"ml.ctlg.sch.vw.extra", []string{"ml", "ctlg", "sch", "vw.extra"}, false},
		{"ml.ctlg.sch", nil, true},
		{"ml.ctlg", nil, true},
		{"ml", nil, true},
		{"", nil, true},
		{".ctlg.sch.vw", nil, true},
	}

	r := newViewResource(t, "http://127.0.0.1:1")
	ctx := context.Background()
	schemaObj := viewSchema(t, r)

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			nullObj, _ := types.ObjectValueFrom(ctx, viewObjectType(t, schemaObj).AttributeTypes(), resourceview.ViewResourceModel{
				Metalake:       types.StringNull(),
				Catalog:        types.StringNull(),
				Schema:         types.StringNull(),
				Name:           types.StringNull(),
				Comment:        types.StringNull(),
				DefaultCatalog: types.StringNull(),
				DefaultSchema:  types.StringNull(),
				Properties:     types.MapNull(types.StringType),
				Audit:          types.ObjectNull(models.AuditAttrTypes),
			})
			raw, err := nullObj.ToTerraformValue(ctx)
			if err != nil {
				t.Fatalf("failed to build null state: %v", err)
			}

			resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: schemaObj, Raw: raw}}
			r.ImportState(ctx, resource.ImportStateRequest{ID: tt.id}, resp)

			if tt.expectError {
				if !resp.Diagnostics.HasError() {
					t.Fatalf("expected an error for import id %q", tt.id)
				}
				return
			}
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected error for import id %q: %v", tt.id, resp.Diagnostics)
			}

			for i, attr := range []string{"metalake", "catalog", "schema", "name"} {
				var got types.String
				if diags := resp.State.GetAttribute(ctx, path.Root(attr), &got); diags.HasError() {
					t.Fatalf("failed to read %s: %v", attr, diags)
				}
				if got.ValueString() != tt.wantParts[i] {
					t.Errorf("%s = %q, want %q", attr, got.ValueString(), tt.wantParts[i])
				}
			}

			var id types.String
			if diags := resp.State.GetAttribute(ctx, path.Root("id"), &id); diags.HasError() {
				t.Fatalf("failed to read id: %v", diags)
			}
			if id.ValueString() != tt.id {
				t.Errorf("id = %q, want %q", id.ValueString(), tt.id)
			}
		})
	}
}

func TestViewAuditModelConversion(t *testing.T) {
	ctx := context.Background()

	nullObj, diags := models.AuditToObjectValue(ctx, nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !nullObj.IsNull() {
		t.Error("nil audit must convert to a null object")
	}

	audit := testViewAudit()
	obj, diags := models.AuditToObjectValue(ctx, audit)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	attrs := obj.Attributes()
	if attrs["creator"].(types.String).ValueString() != "gravitino" {
		t.Errorf("creator = %v", attrs["creator"])
	}
	if attrs["create_time"].(types.String).ValueString() != "2024-01-01T00:00:00Z" {
		t.Errorf("create_time = %v", attrs["create_time"])
	}
}

func assertJSONEqual(t *testing.T, want string, got []byte) {
	t.Helper()

	if len(got) == 0 {
		t.Fatal("no request body was captured")
	}

	var wantMap, gotMap interface{}
	if err := json.Unmarshal([]byte(want), &wantMap); err != nil {
		t.Fatalf("invalid expectation: %v", err)
	}
	if err := json.Unmarshal(got, &gotMap); err != nil {
		t.Fatalf("invalid request body %s: %v", string(got), err)
	}
	if !reflect.DeepEqual(wantMap, gotMap) {
		t.Errorf("request body mismatch\n got: %s\nwant: %s", string(got), want)
	}
}
