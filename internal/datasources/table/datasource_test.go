package table

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
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

type recordedRequest struct {
	Method string
	Path   string
}

func newMockServer(t *testing.T, handler func(req recordedRequest, w http.ResponseWriter) bool) (*client.Client, *[]recordedRequest) {
	t.Helper()

	recorded := &[]recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := recordedRequest{Method: r.Method, Path: r.URL.Path}
		*recorded = append(*recorded, req)

		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if !handler(req, w) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":1003,"type":"NoSuchTableException","message":"Failed to operate table(s) [test_table] operation [LOAD] under schema [test_schema], reason [NoSuchTableException]","stack":["org.apache.gravitino.exceptions.NoSuchTableException: Hive table does not exist"]}`))
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

func dataSourceSchema(t *testing.T, d datasource.DataSource) datasource.SchemaResponse {
	t.Helper()

	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return *resp
}

func TestTableDataSource_Metadata(t *testing.T) {
	d := NewTableDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "gravitino"}, resp)
	if resp.TypeName != "gravitino_table" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

func TestTablesDataSource_Metadata(t *testing.T) {
	d := NewTablesDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "gravitino"}, resp)
	if resp.TypeName != "gravitino_tables" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

func TestTableDataSource_Schema(t *testing.T) {
	schemaResp := dataSourceSchema(t, NewTableDataSource())

	for _, name := range []string{"metalake", "catalog", "schema", "name"} {
		attribute, ok := schemaResp.Schema.Attributes[name]
		if !ok {
			t.Fatalf("attribute %q is missing", name)
		}
		if !attribute.IsRequired() {
			t.Errorf("attribute %q must be required", name)
		}
	}

	for _, name := range []string{"comment", "properties"} {
		attribute, ok := schemaResp.Schema.Attributes[name]
		if !ok {
			t.Fatalf("attribute %q is missing", name)
		}
		if !attribute.IsComputed() {
			t.Errorf("attribute %q must be computed", name)
		}
	}

	if _, ok := schemaResp.Schema.Attributes["audit"]; !ok {
		t.Error("attribute \"audit\" is missing")
	}

	for _, name := range []string{"column", "sort_order", "distribution", "partitioning", "index"} {
		if _, ok := schemaResp.Schema.Blocks[name]; !ok {
			t.Errorf("block %q is missing", name)
		}
	}

	columnBlock, ok := schemaResp.Schema.Blocks["column"].(schema.ListNestedBlock)
	if !ok {
		t.Fatalf("the column block is %T", schemaResp.Schema.Blocks["column"])
	}
	if _, ok := columnBlock.NestedObject.Attributes["length"]; ok {
		t.Error("the invented length attribute must be gone: the length is part of the primitive type string")
	}
	for _, name := range []string{"name", "type", "comment", "nullable", "auto_increment", "default_value"} {
		if _, ok := columnBlock.NestedObject.Attributes[name]; !ok {
			t.Errorf("column attribute %q is missing", name)
		}
	}
}

// TestTableDataSource_ReadUsesTheSpecExample asserts the table of the
// TableResponse example of tables.yaml is mapped into the data source with the
// spec field names, including the pre-assigned nested types, the sort orders,
// the distribution, the partitioning and the indexes.
func TestTableDataSource_ReadUsesTheSpecExample(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method != http.MethodGet || req.Path != testPath+"/"+testTable {
			return false
		}
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"table": map[string]interface{}{
				"name":    testTable,
				"comment": "This is my Hive table",
				"columns": []interface{}{
					map[string]interface{}{
						"name": "id", "type": "integer", "comment": "id column comment",
						"nullable": true, "autoIncrement": false,
					},
					map[string]interface{}{
						"name": "info", "nullable": true, "autoIncrement": false,
						"type": map[string]interface{}{
							"type": "struct",
							"fields": []interface{}{
								map[string]interface{}{"name": "position", "type": "string", "nullable": true},
								map[string]interface{}{
									"name": "contact", "nullable": true,
									"type": map[string]interface{}{"type": "list", "elementType": "integer", "containsNull": false},
								},
							},
						},
					},
				},
				"properties": map[string]interface{}{"location": "hdfs://0.0.0.0:9000/user/hive/warehouse/my_hive_table"},
				"audit": map[string]interface{}{
					"creator":    "gravitino",
					"createTime": "2023-12-08T11:07:46.938Z",
				},
				"distribution": map[string]interface{}{
					"strategy": "hash",
					"number":   32,
					"funcArgs": []interface{}{map[string]interface{}{"type": "field", "fieldName": []interface{}{"id"}}},
				},
				"sortOrders": []interface{}{
					map[string]interface{}{
						"sortTerm":     map[string]interface{}{"type": "field", "fieldName": []interface{}{"age"}},
						"direction":    "asc",
						"nullOrdering": "nulls_first",
					},
				},
				"partitioning": []interface{}{
					map[string]interface{}{"strategy": "identity", "fieldName": []interface{}{"dt"}},
					map[string]interface{}{"strategy": "bucket", "numBuckets": 16, "fieldNames": []interface{}{[]interface{}{"id"}}},
				},
				"indexes": []interface{}{
					map[string]interface{}{
						"indexType": "primary_key", "name": "PRIMARY",
						"fieldNames": []interface{}{[]interface{}{"id"}},
					},
				},
			},
		})
		return true
	})

	d := NewTableDataSource().(*tableDataSource)
	d.client = c

	schemaObj := dataSourceSchema(t, d).Schema
	config := models.TableDataSourceModel{
		Metalake:   types.StringValue(testMetalake),
		Catalog:    types.StringValue(testCatalog),
		Schema:     types.StringValue(testSchema),
		Name:       types.StringValue(testTable),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(models.AuditAttrTypes),
	}

	obj, diags := types.ObjectValueFrom(context.Background(), schemaObj.Type().(types.ObjectType).AttributeTypes(), config)
	if diags.HasError() {
		t.Fatalf("building config: %v", diags)
	}
	value, err := obj.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("converting config: %v", err)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(context.Background(), datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaObj, Raw: value},
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected read diagnostics: %v", resp.Diagnostics)
	}
	if len(*recorded) != 1 {
		t.Fatalf("expected one request, got %v", *recorded)
	}

	var state models.TableDataSourceModel
	if d := resp.State.Get(context.Background(), &state); d.HasError() {
		t.Fatalf("reading state: %v", d)
	}

	if state.Comment.ValueString() != "This is my Hive table" {
		t.Errorf("comment = %q", state.Comment.ValueString())
	}
	if len(state.Columns) != 2 {
		t.Fatalf("columns = %v", state.Columns)
	}
	if state.Columns[0].Type.ValueString() != "integer" {
		t.Errorf("column type = %q, want the bare primitive name", state.Columns[0].Type.ValueString())
	}
	structType := state.Columns[1].Type.ValueString()
	if !strings.HasPrefix(structType, `{"fields":`) || !strings.Contains(structType, `"type":"struct"`) {
		t.Errorf("structured column type = %q", structType)
	}
	if state.Columns[1].DefaultValue.IsNull() == false {
		t.Errorf("default_value = %v, want null when the column has no default", state.Columns[1].DefaultValue)
	}
	if len(state.SortOrders) != 1 || state.SortOrders[0].NullOrdering.ValueString() != "nulls_first" {
		t.Errorf("sort orders = %v", state.SortOrders)
	}
	if state.Distribution == nil || state.Distribution.Number.ValueInt64() != 32 {
		t.Fatalf("distribution = %v", state.Distribution)
	}
	if args := state.Distribution.FuncArgs.Elements(); len(args) != 1 || args[0].(types.String).ValueString() != "id" {
		t.Errorf("distribution func_args = %v", args)
	}
	if len(state.Partitioning) != 2 {
		t.Fatalf("partitioning = %v", state.Partitioning)
	}
	if state.Partitioning[1].NumBuckets.ValueInt64() != 16 {
		t.Errorf("num_buckets = %v", state.Partitioning[1].NumBuckets)
	}
	if len(state.Indexes) != 1 || state.Indexes[0].IndexType.ValueString() != "primary_key" {
		t.Errorf("indexes = %v", state.Indexes)
	}
	if state.Audit.IsNull() {
		t.Errorf("audit = %v, want the audit reported by Gravitino", state.Audit)
	}
}

// TestTableDataSource_NormalisesServerValues asserts the values Gravitino
// reports but tables.yaml does not define are normalised.
func TestTableDataSource_NormalisesServerValues(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
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
						"properties": map[string]interface{}{},
					},
				},
			},
		})
		return true
	})

	d := NewTableDataSource().(*tableDataSource)
	d.client = c

	schemaObj := dataSourceSchema(t, d).Schema
	config := models.TableDataSourceModel{
		Metalake:   types.StringValue(testMetalake),
		Catalog:    types.StringValue(testCatalog),
		Schema:     types.StringValue(testSchema),
		Name:       types.StringValue(testTable),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(models.AuditAttrTypes),
	}

	obj, _ := types.ObjectValueFrom(context.Background(), schemaObj.Type().(types.ObjectType).AttributeTypes(), config)
	value, _ := obj.ToTerraformValue(context.Background())

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(context.Background(), datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaObj, Raw: value},
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected read diagnostics: %v", resp.Diagnostics)
	}

	var state models.TableDataSourceModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}

	if !state.Columns[0].DefaultValue.IsNull() {
		t.Errorf("the null literal default must map to no default value, got %v", state.Columns[0].DefaultValue)
	}
	if state.Distribution != nil {
		t.Errorf("a distribution with strategy none must map to no distribution, got %v", state.Distribution)
	}
	if state.Indexes[0].IndexType.ValueString() != "primary_key" {
		t.Errorf("index type = %q, want primary_key", state.Indexes[0].IndexType.ValueString())
	}
}

func TestTableDataSource_ReadNotFoundIsAnError(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		return false
	})

	d := NewTableDataSource().(*tableDataSource)
	d.client = c

	schemaObj := dataSourceSchema(t, d).Schema
	config := models.TableDataSourceModel{
		Metalake:   types.StringValue(testMetalake),
		Catalog:    types.StringValue(testCatalog),
		Schema:     types.StringValue(testSchema),
		Name:       types.StringValue("missing_table"),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(models.AuditAttrTypes),
	}

	obj, _ := types.ObjectValueFrom(context.Background(), schemaObj.Type().(types.ObjectType).AttributeTypes(), config)
	value, _ := obj.ToTerraformValue(context.Background())

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(context.Background(), datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaObj, Raw: value},
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a missing table")
	}

	found := false
	for _, diag := range resp.Diagnostics.Errors() {
		if strings.Contains(diag.Detail(), "NoSuchTableException") {
			found = true
		}
	}
	if !found {
		t.Errorf("the Gravitino error type must be reported, got %v", resp.Diagnostics.Errors())
	}
}

// TestTablesDataSource_Read asserts the table list of tables.yaml
// (EntityListResponse with identifiers) is mapped into a list of names.
func TestTablesDataSource_Read(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method != http.MethodGet || req.Path != testPath {
			return false
		}
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"identifiers": []interface{}{
				map[string]interface{}{"namespace": []interface{}{testMetalake, testCatalog, testSchema}, "name": "t1"},
				map[string]interface{}{"namespace": []interface{}{testMetalake, testCatalog, testSchema}, "name": "t2"},
			},
		})
		return true
	})

	d := NewTablesDataSource().(*tablesDataSource)
	d.client = c

	schemaObj := dataSourceSchema(t, d).Schema
	config := tablesDataSourceModel{
		Metalake: types.StringValue(testMetalake),
		Catalog:  types.StringValue(testCatalog),
		Schema:   types.StringValue(testSchema),
		Tables:   types.ListNull(types.StringType),
	}

	obj, _ := types.ObjectValueFrom(context.Background(), schemaObj.Type().(types.ObjectType).AttributeTypes(), config)
	value, _ := obj.ToTerraformValue(context.Background())

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(context.Background(), datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaObj, Raw: value},
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected read diagnostics: %v", resp.Diagnostics)
	}

	var state tablesDataSourceModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}

	elements := state.Tables.Elements()
	if len(elements) != 2 {
		t.Fatalf("tables = %v", elements)
	}
	if elements[0].(types.String).ValueString() != "t1" {
		t.Errorf("first table = %v", elements[0])
	}
}
