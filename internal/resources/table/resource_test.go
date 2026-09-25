package table

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	testMetalake = "probe_ml"
	testCatalog  = "ptcat"
	testSchema   = "ptsch"
	testTable    = "my_hive_table"
	testPath     = "/api/metalakes/probe_ml/catalogs/ptcat/schemas/ptsch/tables"
)

// recordedRequest captures the requests a resource test made against the mock
// Gravitino API.
type recordedRequest struct {
	Method string
	Path   string
	Body   map[string]interface{}
}

func newMockServer(t *testing.T, handler func(req recordedRequest, w http.ResponseWriter) bool) (*client.Client, *[]recordedRequest) {
	t.Helper()

	recorded := &[]recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := recordedRequest{Method: r.Method, Path: r.URL.Path}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req.Body)
		}
		*recorded = append(*recorded, req)

		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if !handler(req, w) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":1003,"type":"NoSuchTableException","message":"no such table","stack":["org.apache.gravitino.exceptions.NoSuchTableException: x"]}`))
		}
	}))
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}
	return c, recorded
}

func writeJSON(t *testing.T, w http.ResponseWriter, value interface{}) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encoding response: %v", err)
	}
}

func tableSchema(t *testing.T, r resource.Resource) schema.Schema {
	t.Helper()

	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func planValue(t *testing.T, schemaObj schema.Schema, model models.TableResourceModel) tfsdk.Plan {
	t.Helper()

	obj, diags := types.ObjectValueFrom(context.Background(), schemaObj.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("building plan object: %v", diags)
	}
	value, err := obj.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("converting plan: %v", err)
	}
	return tfsdk.Plan{Schema: schemaObj, Raw: value}
}

func stateValue(t *testing.T, schemaObj schema.Schema, model models.TableResourceModel) tfsdk.State {
	t.Helper()

	obj, diags := types.ObjectValueFrom(context.Background(), schemaObj.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("building state object: %v", diags)
	}
	value, err := obj.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("converting state: %v", err)
	}
	return tfsdk.State{Schema: schemaObj, Raw: value}
}

func nullState(t *testing.T, schemaObj schema.Schema) tfsdk.State {
	t.Helper()

	obj := types.ObjectNull(schemaObj.Type().(types.ObjectType).AttributeTypes())
	value, err := obj.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("converting null state: %v", err)
	}
	return tfsdk.State{Schema: schemaObj, Raw: value}
}

func modelFromState(t *testing.T, state tfsdk.State, model *models.TableResourceModel) {
	t.Helper()

	diags := state.Get(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
}

// hivePlan builds the plan of the Hive table of the HiveTableCreate example of
// tables.yaml.
func hivePlan() models.TableResourceModel {
	return models.TableResourceModel{
		Metalake: types.StringValue(testMetalake),
		Catalog:  types.StringValue(testCatalog),
		Schema:   types.StringValue(testSchema),
		Name:     types.StringValue(testTable),
		Comment:  types.StringValue("This is my Hive table"),
		Properties: mustMap(map[string]string{
			"format": "ORC",
		}),
		ID:    types.StringUnknown(),
		Audit: types.ObjectUnknown(models.AuditAttrTypes),
		Columns: []models.ColumnTFSDK{
			{
				Name:          types.StringValue("id"),
				Type:          types.StringValue("integer"),
				Comment:       types.StringValue("id column comment"),
				Nullable:      types.BoolValue(true),
				AutoIncrement: types.BoolValue(false),
				DefaultValue:  types.StringNull(),
			},
			{
				Name:          types.StringValue("name"),
				Type:          types.StringValue("varchar(255)"),
				Comment:       types.StringValue("name column comment"),
				Nullable:      types.BoolValue(true),
				AutoIncrement: types.BoolValue(false),
				DefaultValue:  types.StringValue("default_name"),
			},
			{
				Name:          types.StringValue("dt"),
				Type:          types.StringValue("date"),
				Comment:       types.StringNull(),
				Nullable:      types.BoolValue(true),
				AutoIncrement: types.BoolValue(false),
				DefaultValue:  types.StringNull(),
			},
		},
		SortOrders: []models.SortOrderTFSDK{
			{
				FieldName:    mustList("age"),
				Direction:    types.StringValue("asc"),
				NullOrdering: types.StringNull(),
			},
		},
		Distribution: &models.DistributionTFSDK{
			Strategy: types.StringValue("hash"),
			Number:   types.Int64Value(32),
			FuncArgs: mustList("id"),
		},
		Partitioning: []models.PartitioningTFSDK{
			{
				Strategy:   types.StringValue("identity"),
				FieldName:  mustList("dt"),
				FieldNames: types.ListNull(types.ListType{ElemType: types.StringType}),
				NumBuckets: types.Int64Null(),
				Width:      types.Int64Null(),
				FuncName:   types.StringNull(),
				FuncArgs:   types.ListNull(types.StringType),
			},
		},
		Indexes: []models.IndexTFSDK{
			{
				IndexType:  types.StringValue("primary_key"),
				Name:       types.StringValue("PRIMARY"),
				FieldNames: mustNestedList([][]string{{"id"}}),
			},
		},
	}
}

func mustMap(values map[string]string) types.Map {
	value, diags := types.MapValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		panic(diags)
	}
	return value
}

func mustList(values ...string) types.List {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}
	list, diags := types.ListValue(types.StringType, elements)
	if diags.HasError() {
		panic(diags)
	}
	return list
}

func mustNestedList(values [][]string) types.List {
	outer := make([]attr.Value, 0, len(values))
	for _, inner := range values {
		list := mustList(inner...)
		outer = append(outer, list)
	}
	result, diags := types.ListValue(types.ListType{ElemType: types.StringType}, outer)
	if diags.HasError() {
		panic(diags)
	}
	return result
}

// TestTableResource_CreateSendsSpecPayload asserts that the create request is
// exactly the payload of the tables.yaml TableCreateRequest schema, using the
// HiveTableCreate example as the source of the field names and shapes.
func TestTableResource_CreateSendsSpecPayload(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method != http.MethodPost || req.Path != testPath {
			return false
		}
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"table": map[string]interface{}{
				"name": testTable,
				"audit": map[string]interface{}{
					"creator":    "gravitino",
					"createTime": "2023-12-08T11:07:46.938Z",
				},
			},
		})
		return true
	})

	r := NewTableResource().(*tableResource)
	r.client = c

	schemaObj := tableSchema(t, r)
	plan := hivePlan()

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: planValue(t, schemaObj, plan),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %v", resp.Diagnostics)
	}
	if len(*recorded) != 1 {
		t.Fatalf("expected 1 request, got %d", len(*recorded))
	}

	expected := map[string]interface{}{
		"name":    testTable,
		"comment": "This is my Hive table",
		"properties": map[string]interface{}{
			"format": "ORC",
		},
		"columns": []interface{}{
			map[string]interface{}{
				"name": "id", "type": "integer", "comment": "id column comment",
				"nullable": true, "autoIncrement": false,
			},
			map[string]interface{}{
				"name": "name", "type": "varchar(255)", "comment": "name column comment",
				"nullable": true, "autoIncrement": false,
				"defaultValue": map[string]interface{}{
					"type": "literal", "dataType": "varchar(255)", "value": "default_name",
				},
			},
			map[string]interface{}{
				"name": "dt", "type": "date", "nullable": true, "autoIncrement": false,
			},
		},
		"sortOrders": []interface{}{
			map[string]interface{}{
				"sortTerm":     map[string]interface{}{"type": "field", "fieldName": []interface{}{"age"}},
				"direction":    "asc",
				"nullOrdering": "nulls_first",
			},
		},
		"distribution": map[string]interface{}{
			"strategy": "hash",
			"number":   float64(32),
			"funcArgs": []interface{}{
				map[string]interface{}{"type": "field", "fieldName": []interface{}{"id"}},
			},
		},
		"partitioning": []interface{}{
			map[string]interface{}{
				"strategy":  "identity",
				"fieldName": []interface{}{"dt"},
			},
		},
		"indexes": []interface{}{
			map[string]interface{}{
				"indexType":  "primary_key",
				"name":       "PRIMARY",
				"fieldNames": []interface{}{[]interface{}{"id"}},
			},
		},
	}

	if !reflect.DeepEqual(expected, (*recorded)[0].Body) {
		expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
		actualJSON, _ := json.MarshalIndent((*recorded)[0].Body, "", "  ")
		t.Errorf("create payload mismatch\nwant: %s\ngot:  %s", expectedJSON, actualJSON)
	}

	var state models.TableResourceModel
	modelFromState(t, resp.State, &state)
	if state.ID.ValueString() != "probe_ml.ptcat.ptsch.my_hive_table" {
		t.Errorf("id = %q", state.ID.ValueString())
	}
	if state.Properties.IsNull() || state.Properties.IsUnknown() {
		t.Error("properties must be known after create")
	}
	if state.Audit.IsNull() || state.Audit.IsUnknown() {
		t.Error("audit must be known after create")
	}
}

// TestTableResource_CreateWithStructuredColumnType asserts that a structured
// column type is sent as the JSON object of datatype.yaml while a primitive
// type stays a bare string.
func TestTableResource_CreateWithStructuredColumnType(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method != http.MethodPost || req.Path != testPath {
			return false
		}
		writeJSON(t, w, map[string]interface{}{
			"code":  0,
			"table": map[string]interface{}{"name": testTable},
		})
		return true
	})

	r := NewTableResource().(*tableResource)
	r.client = c

	schemaObj := tableSchema(t, r)
	plan := hivePlan()
	plan.Columns = []models.ColumnTFSDK{
		{
			Name:          types.StringValue("info"),
			Type:          types.StringValue(`{"fields":[{"name":"position","type":"string"}],"type":"struct"}`),
			Comment:       types.StringValue("info column comment"),
			Nullable:      types.BoolValue(true),
			AutoIncrement: types.BoolValue(false),
			DefaultValue:  types.StringNull(),
		},
	}
	plan.SortOrders = nil
	plan.Distribution = nil
	plan.Partitioning = nil
	plan.Indexes = nil
	plan.Properties = types.MapNull(types.StringType)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: planValue(t, schemaObj, plan),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %v", resp.Diagnostics)
	}

	columns, ok := (*recorded)[0].Body["columns"].([]interface{})
	if !ok || len(columns) != 1 {
		t.Fatalf("expected one column, got %v", (*recorded)[0].Body["columns"])
	}

	column, _ := columns[0].(map[string]interface{})
	expected := map[string]interface{}{
		"fields": []interface{}{
			map[string]interface{}{"name": "position", "type": "string"},
		},
		"type": "struct",
	}
	if !reflect.DeepEqual(expected, column["type"]) {
		t.Errorf("structured column type = %#v, want %#v", column["type"], expected)
	}
}

func TestTableResource_ReadNotFoundRemovesResource(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		return false // always the real Gravitino 404 payload
	})

	r := NewTableResource().(*tableResource)
	r.client = c

	schemaObj := tableSchema(t, r)
	state := hivePlan()
	state.ID = types.StringValue("probe_ml.ptcat.ptsch.my_hive_table")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Read(context.Background(), resource.ReadRequest{
		State: stateValue(t, schemaObj, state),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a missing table must not be an error, got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("the table must be removed from state on a 404")
	}
}

func TestTableResource_DeleteToleratesNotFound(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		return false
	})

	r := NewTableResource().(*tableResource)
	r.client = c

	schemaObj := tableSchema(t, r)
	state := hivePlan()

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: stateValue(t, schemaObj, state),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("deleting a table that is already gone must succeed, got %v", resp.Diagnostics)
	}
}

// TestTableResource_UpdateColumnCommentSendsUpdateRequest is the convergence
// test: a column change is applied with the updateColumnComment request of
// tables.yaml, the applied state holds the planned value and no attribute is
// left unknown.
func TestTableResource_UpdateColumnCommentSendsUpdateRequest(t *testing.T) {
	updatedTable := map[string]interface{}{
		"name":    testTable,
		"comment": "This is my Hive table",
		"columns": []interface{}{
			map[string]interface{}{"name": "id", "type": "integer", "nullable": true, "autoIncrement": false},
			map[string]interface{}{
				"name": "name", "type": "varchar(255)", "comment": "name column comment v2",
				"nullable": true, "autoIncrement": false,
			},
		},
		"properties": map[string]interface{}{"format": "ORC"},
	}

	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		switch {
		case req.Method == http.MethodPut && req.Path == testPath+"/"+testTable:
			writeJSON(t, w, map[string]interface{}{"code": 0, "table": updatedTable})
			return true
		case req.Method == http.MethodGet && req.Path == testPath+"/"+testTable:
			writeJSON(t, w, map[string]interface{}{"code": 0, "table": updatedTable})
			return true
		}
		return false
	})

	r := NewTableResource().(*tableResource)
	r.client = c

	schemaObj := tableSchema(t, r)

	state := hivePlan()
	state.Columns = twoColumns("name column comment")

	plan := hivePlan()
	plan.Columns = twoColumns("name column comment v2")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  planValue(t, schemaObj, plan),
		State: stateValue(t, schemaObj, state),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected update diagnostics: %v", resp.Diagnostics)
	}

	if len(*recorded) != 2 {
		t.Fatalf("expected one update and one read, got %d requests", len(*recorded))
	}

	expected := map[string]interface{}{
		"updates": []interface{}{
			map[string]interface{}{
				"@type":      "updateColumnComment",
				"fieldName":  []interface{}{"name"},
				"newComment": "name column comment v2",
			},
		},
	}
	if !reflect.DeepEqual(expected, (*recorded)[0].Body) {
		expectedJSON, _ := json.Marshal(expected)
		actualJSON, _ := json.Marshal((*recorded)[0].Body)
		t.Errorf("update payload mismatch\nwant: %s\ngot:  %s", expectedJSON, actualJSON)
	}

	var applied models.TableResourceModel
	modelFromState(t, resp.State, &applied)
	if applied.Columns[1].Comment.ValueString() != "name column comment v2" {
		t.Errorf("applied column comment = %q", applied.Columns[1].Comment.ValueString())
	}
	if applied.Columns[1].Comment.IsUnknown() {
		t.Error("the applied column comment must not be unknown")
	}
	if applied.ID.ValueString() != "probe_ml.ptcat.ptsch.my_hive_table" {
		t.Errorf("id = %q", applied.ID.ValueString())
	}
}

func twoColumns(nameComment string) []models.ColumnTFSDK {
	return []models.ColumnTFSDK{
		{
			Name:          types.StringValue("id"),
			Type:          types.StringValue("integer"),
			Comment:       types.StringNull(),
			Nullable:      types.BoolValue(true),
			AutoIncrement: types.BoolValue(false),
			DefaultValue:  types.StringNull(),
		},
		{
			Name:          types.StringValue("name"),
			Type:          types.StringValue("varchar(255)"),
			Comment:       types.StringValue(nameComment),
			Nullable:      types.BoolValue(true),
			AutoIncrement: types.BoolValue(false),
			DefaultValue:  types.StringNull(),
		},
	}
}

func TestTableResource_UpdateSendsRenameCommentPropertiesAndType(t *testing.T) {
	table := map[string]interface{}{"name": "my_hive_table_new"}

	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		switch req.Method {
		case http.MethodPut:
			writeJSON(t, w, map[string]interface{}{"code": 0, "table": table})
			return true
		case http.MethodGet:
			writeJSON(t, w, map[string]interface{}{"code": 0, "table": table})
			return true
		}
		return false
	})

	r := NewTableResource().(*tableResource)
	r.client = c

	schemaObj := tableSchema(t, r)

	state := hivePlan()
	state.Columns = twoColumns("name column comment")

	plan := hivePlan()
	plan.Columns = twoColumns("name column comment")
	plan.Name = types.StringValue("my_hive_table_new")
	plan.Comment = types.StringValue("new table comment")
	plan.Properties = mustMap(map[string]string{"format": "PARQUET", "key": "value"})
	plan.Columns[0].Type = types.StringValue("long")
	plan.Columns[0].Nullable = types.BoolValue(false)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  planValue(t, schemaObj, plan),
		State: stateValue(t, schemaObj, state),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected update diagnostics: %v", resp.Diagnostics)
	}

	updates, ok := (*recorded)[0].Body["updates"].([]interface{})
	if !ok {
		t.Fatalf("no updates in %v", (*recorded)[0].Body)
	}

	expected := []interface{}{
		map[string]interface{}{"@type": "rename", "newName": "my_hive_table_new"},
		map[string]interface{}{"@type": "updateComment", "newComment": "new table comment"},
		map[string]interface{}{"@type": "setProperty", "property": "format", "value": "PARQUET"},
		map[string]interface{}{"@type": "setProperty", "property": "key", "value": "value"},
		map[string]interface{}{"@type": "updateColumnType", "fieldName": []interface{}{"id"}, "newType": "long"},
		map[string]interface{}{"@type": "updateColumnNullability", "fieldName": []interface{}{"id"}, "nullable": false},
	}
	if !reflect.DeepEqual(expected, updates) {
		expectedJSON, _ := json.Marshal(expected)
		actualJSON, _ := json.Marshal(updates)
		t.Errorf("update payload mismatch\nwant: %s\ngot:  %s", expectedJSON, actualJSON)
	}
}

// TestTableResource_UpdateRejectsChangedBlock asserts that a block change that
// reached Update is reported instead of being ignored. The planned value is
// unknown during plan, so ModifyPlan cannot turn it into a replacement.
func TestTableResource_UpdateRejectsChangedBlock(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		return true
	})

	r := NewTableResource().(*tableResource)
	r.client = c

	schemaObj := tableSchema(t, r)

	state := hivePlan()
	plan := hivePlan()
	plan.SortOrders = []models.SortOrderTFSDK{
		{
			FieldName:    mustList("id"),
			Direction:    types.StringValue("desc"),
			NullOrdering: types.StringNull(),
		},
	}

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  planValue(t, schemaObj, plan),
		State: stateValue(t, schemaObj, state),
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for the changed sort order")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "sort_order") {
		t.Errorf("unexpected diagnostic: %s", resp.Diagnostics.Errors()[0].Summary())
	}
	if len(*recorded) != 0 {
		t.Error("no request may be sent when the change cannot be applied")
	}
}

func TestTableResource_ModifyPlanRequiresReplace(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(plan *models.TableResourceModel)
		wantPath string
	}{
		{
			name: "sort_order",
			mutate: func(plan *models.TableResourceModel) {
				plan.SortOrders = []models.SortOrderTFSDK{
					{
						FieldName:    mustList("id"),
						Direction:    types.StringValue("desc"),
						NullOrdering: types.StringNull(),
					},
				}
			},
			wantPath: "sort_order",
		},
		{
			name: "distribution",
			mutate: func(plan *models.TableResourceModel) {
				plan.Distribution = &models.DistributionTFSDK{
					Strategy: types.StringValue("hash"),
					Number:   types.Int64Value(8),
					FuncArgs: mustList("id"),
				}
			},
			wantPath: "distribution",
		},
		{
			name: "partitioning",
			mutate: func(plan *models.TableResourceModel) {
				plan.Partitioning = []models.PartitioningTFSDK{
					{
						Strategy:   types.StringValue("identity"),
						FieldName:  mustList("id"),
						FieldNames: types.ListNull(types.ListType{ElemType: types.StringType}),
						NumBuckets: types.Int64Null(),
						Width:      types.Int64Null(),
						FuncName:   types.StringNull(),
						FuncArgs:   types.ListNull(types.StringType),
					},
				}
			},
			wantPath: "partitioning",
		},
		{
			name: "index",
			mutate: func(plan *models.TableResourceModel) {
				plan.Indexes = nil
			},
			wantPath: "index",
		},
		{
			name: "column renamed",
			mutate: func(plan *models.TableResourceModel) {
				plan.Columns = twoColumns("name column comment")
				plan.Columns[1].Name = types.StringValue("surname")
			},
			wantPath: "column",
		},
		{
			name: "column default value removed",
			mutate: func(plan *models.TableResourceModel) {
				plan.Columns = twoColumns("name column comment")
				plan.Columns[1].DefaultValue = types.StringNull()
			},
			wantPath: "column",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewTableResource().(*tableResource)
			schemaObj := tableSchema(t, r)

			state := hivePlan()
			state.Columns = twoColumns("name column comment")
			state.Columns[1].DefaultValue = types.StringValue("default_name")
			state.ID = types.StringValue("probe_ml.ptcat.ptsch.my_hive_table")

			plan := hivePlan()
			plan.Columns = twoColumns("name column comment")
			plan.Columns[1].DefaultValue = types.StringValue("default_name")

			tt.mutate(&plan)

			resp := &resource.ModifyPlanResponse{Plan: planValue(t, schemaObj, plan)}
			r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{
				Plan:  planValue(t, schemaObj, plan),
				State: stateValue(t, schemaObj, state),
			}, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected modify plan diagnostics: %v", resp.Diagnostics)
			}

			found := false
			for _, p := range resp.RequiresReplace {
				if p.String() == tt.wantPath {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %q to require a replacement, got %v", tt.wantPath, resp.RequiresReplace)
			}
		})
	}
}

func TestTableResource_ModifyPlanCanonicalisesColumnType(t *testing.T) {
	r := NewTableResource().(*tableResource)
	schemaObj := tableSchema(t, r)

	plan := hivePlan()
	plan.Columns = twoColumns("c")
	plan.Columns[0].Type = types.StringValue("  integer  ")

	resp := &resource.ModifyPlanResponse{Plan: planValue(t, schemaObj, plan)}
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{
		Plan: planValue(t, schemaObj, plan),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected modify plan diagnostics: %v", resp.Diagnostics)
	}

	var canonical models.TableResourceModel
	diags := resp.Plan.Get(context.Background(), &canonical)
	if diags.HasError() {
		t.Fatalf("reading canonical plan: %v", diags)
	}
	if canonical.Columns[0].Type.ValueString() != "integer" {
		t.Errorf("column type = %q, want %q", canonical.Columns[0].Type.ValueString(), "integer")
	}
}

// TestTableResource_ModifyPlanNormalisesEmptyComment asserts that an empty
// comment is planned as unset, because Gravitino reports an empty comment as an
// absent one.
func TestTableResource_ModifyPlanNormalisesEmptyComment(t *testing.T) {
	r := NewTableResource().(*tableResource)
	schemaObj := tableSchema(t, r)

	plan := hivePlan()
	plan.Comment = types.StringValue("")
	plan.Columns = twoColumns("")
	plan.Columns[0].Comment = types.StringValue("")

	resp := &resource.ModifyPlanResponse{Plan: planValue(t, schemaObj, plan)}
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{
		Plan: planValue(t, schemaObj, plan),
	}, resp)

	var canonical models.TableResourceModel
	diags := resp.Plan.Get(context.Background(), &canonical)
	if diags.HasError() {
		t.Fatalf("reading canonical plan: %v", diags)
	}
	if !canonical.Comment.IsNull() {
		t.Errorf("table comment = %v, want null", canonical.Comment)
	}
	if !canonical.Columns[1].Comment.IsNull() {
		t.Errorf("column comment = %v, want null", canonical.Columns[1].Comment)
	}
}

// TestTableResource_ReadNormalisesServerValues asserts the values Gravitino
// reports but the configuration cannot express are not stored as drift.
func TestTableResource_ReadNormalisesServerValues(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method != http.MethodGet {
			return false
		}
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"table": map[string]interface{}{
				"name": testTable,
				"columns": []interface{}{
					map[string]interface{}{
						"name": "id", "type": "integer", "nullable": true, "autoIncrement": false,
						"defaultValue": map[string]interface{}{"type": "literal", "dataType": "null", "value": "NULL"},
					},
				},
				"distribution": map[string]interface{}{"strategy": "none", "number": 0, "funcArgs": []interface{}{}},
				"indexes": []interface{}{
					map[string]interface{}{
						"indexType": "PRIMARY_KEY", "name": "PRIMARY",
						"fieldNames": []interface{}{[]interface{}{"id"}},
					},
				},
			},
		})
		return true
	})

	r := NewTableResource().(*tableResource)
	r.client = c

	schemaObj := tableSchema(t, r)
	state := hivePlan()
	state.Properties = types.MapNull(types.StringType)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Read(context.Background(), resource.ReadRequest{
		State: stateValue(t, schemaObj, state),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected read diagnostics: %v", resp.Diagnostics)
	}

	var refreshed models.TableResourceModel
	modelFromState(t, resp.State, &refreshed)

	if !refreshed.Columns[0].DefaultValue.IsNull() {
		t.Errorf("the null literal default must map to no default value, got %v", refreshed.Columns[0].DefaultValue)
	}
	if refreshed.Distribution != nil {
		t.Errorf("a distribution with strategy none must map to no distribution, got %v", refreshed.Distribution)
	}
	if got := refreshed.Indexes[0].IndexType.ValueString(); got != "primary_key" {
		t.Errorf("index type = %q, want primary_key", got)
	}
}

func TestTableResource_ImportState(t *testing.T) {
	r := NewTableResource().(resource.ResourceWithImportState)
	schemaObj := tableSchema(t, r)

	resp := &resource.ImportStateResponse{State: nullState(t, schemaObj)}
	r.ImportState(context.Background(), resource.ImportStateRequest{
		ID: "ml.cat.sch.tbl",
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected import diagnostics: %v", resp.Diagnostics)
	}

	var imported models.TableResourceModel
	modelFromState(t, resp.State, &imported)
	if imported.Metalake.ValueString() != "ml" || imported.Name.ValueString() != "tbl" {
		t.Errorf("imported %q / %q", imported.Metalake.ValueString(), imported.Name.ValueString())
	}
}

func TestTableResource_ImportStateInvalid(t *testing.T) {
	r := NewTableResource().(resource.ResourceWithImportState)
	schemaObj := tableSchema(t, r)

	resp := &resource.ImportStateResponse{State: nullState(t, schemaObj)}
	r.ImportState(context.Background(), resource.ImportStateRequest{
		ID: "too.few",
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an invalid import id")
	}
}

func TestTableResource_Metadata(t *testing.T) {
	r := NewTableResource()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "gravitino"}, resp)
	if resp.TypeName != "gravitino_table" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

// TestTableResource_SchemaDocumentsReplacements asserts every block Gravitino
// cannot update in place documents that it forces a replacement.
func TestTableResource_SchemaDocumentsReplacements(t *testing.T) {
	schemaObj := tableSchema(t, NewTableResource())

	for _, name := range []string{"metalake", "catalog", "schema"} {
		if schemaObj.Attributes[name] == nil {
			t.Fatalf("attribute %q is missing", name)
		}
	}

	for _, name := range []string{"column", "sort_order", "distribution", "partitioning", "index"} {
		block, ok := schemaObj.Blocks[name]
		if !ok {
			t.Fatalf("block %q is missing", name)
		}
		if block.GetDescription() == "" || !strings.Contains(block.GetDescription(), "replac") {
			t.Errorf("block %q must document when it forces a replacement, got %q", name, block.GetDescription())
		}
	}
}
