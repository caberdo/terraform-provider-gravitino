package partition

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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	testMetalake = "probe_ml"
	testCatalog  = "ptic_ice"
	testSchema   = "ptsch"
	testTable    = "pt_part"
	testPath     = "/api/metalakes/probe_ml/catalogs/ptic_ice/schemas/ptsch/tables/pt_part/partitions"
)

type recordedRequest struct {
	Method   string
	Path     string
	RawQuery string
}

func newMockServer(t *testing.T, handler func(req recordedRequest, w http.ResponseWriter) bool) (*client.Client, *[]recordedRequest) {
	t.Helper()

	recorded := &[]recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := recordedRequest{Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery}
		*recorded = append(*recorded, req)

		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if !handler(req, w) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":1003,"type":"NoSuchMetalakeException","message":"Metalake does not exist: probe_ml","stack":["org.apache.gravitino.exceptions.NoSuchMetalakeException: x"]}`))
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

func partitionSchema(t *testing.T, d datasource.DataSource) datasource.SchemaResponse {
	t.Helper()

	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return *resp
}

func TestPartitionDataSource_Metadata(t *testing.T) {
	d := NewPartitionDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "gravitino"}, resp)
	if resp.TypeName != "gravitino_partition" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

func TestPartitionsDataSource_Metadata(t *testing.T) {
	d := NewPartitionsDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "gravitino"}, resp)
	if resp.TypeName != "gravitino_partitions" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

func TestPartitionDataSource_Schema(t *testing.T) {
	schemaResp := partitionSchema(t, NewPartitionDataSource())

	for _, name := range []string{"metalake", "catalog", "schema", "table", "name", "type", "field_names", "values", "upper", "lower", "lists", "properties"} {
		attribute, ok := schemaResp.Schema.Attributes[name]
		if !ok {
			t.Fatalf("attribute %q is missing", name)
		}
		switch name {
		case "metalake", "catalog", "schema", "table", "name":
			if !attribute.IsRequired() {
				t.Errorf("attribute %q must be required", name)
			}
		default:
			if !attribute.IsComputed() {
				t.Errorf("attribute %q must be computed", name)
			}
		}
	}
}

// TestPartitionDataSource_ReadUsesTheSpecExample asserts the partition of the
// HivePartitionResponse example of partitions.yaml is mapped into the data
// source, using the spec field names.
func TestPartitionDataSource_ReadUsesTheSpecExample(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method != http.MethodGet || req.Path != testPath+"/p1" {
			return false
		}
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"partition": map[string]interface{}{
				"type": "identity",
				"name": "p1",
				"fieldNames": []interface{}{
					[]interface{}{"col1"},
				},
				"values": []interface{}{
					map[string]interface{}{"type": "literal", "dataType": "string", "value": "v1"},
				},
				"properties": map[string]interface{}{"totalSize": "2"},
			},
		})
		return true
	})

	d := NewPartitionDataSource().(*PartitionDataSource)
	d.client = c

	schemaObj := partitionSchema(t, d).Schema
	config := PartitionDataSourceModel{
		Metalake:   types.StringValue(testMetalake),
		Catalog:    types.StringValue(testCatalog),
		Schema:     types.StringValue(testSchema),
		Table:      types.StringValue(testTable),
		Name:       types.StringValue("p1"),
		Type:       types.StringNull(),
		FieldNames: types.ListNull(types.ListType{ElemType: types.StringType}),
		Values:     types.ListNull(types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes}),
		Upper:      types.ObjectNull(models.PartitionLiteralAttrTypes),
		Lower:      types.ObjectNull(models.PartitionLiteralAttrTypes),
		Lists:      types.ListNull(types.ListType{ElemType: types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes}}),
		Properties: types.MapNull(types.StringType),
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

	var state PartitionDataSourceModel
	if d := resp.State.Get(context.Background(), &state); d.HasError() {
		t.Fatalf("reading state: %v", d)
	}

	if state.Type.ValueString() != models.PartitionTypeIdentity {
		t.Errorf("type = %q", state.Type.ValueString())
	}
	if state.Name.ValueString() != "p1" {
		t.Errorf("name = %q", state.Name.ValueString())
	}
	if len(state.FieldNames.Elements()) != 1 {
		t.Errorf("field_names = %v", state.FieldNames.Elements())
	}
	if len(state.Values.Elements()) != 1 {
		t.Errorf("values = %v", state.Values.Elements())
	}
	if state.Properties.IsNull() || len(state.Properties.Elements()) != 1 {
		t.Errorf("properties = %v", state.Properties)
	}
	if !state.Upper.IsNull() {
		t.Errorf("upper = %v, want null for an identity partition", state.Upper)
	}
}

// TestPartitionsDataSource_ReadRequestsDetails asserts the list data source uses
// the details query parameter of partitions.yaml, which returns the partitions
// with their details in a single request.
func TestPartitionsDataSource_ReadRequestsDetails(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method != http.MethodGet || req.Path != testPath {
			return false
		}
		if req.RawQuery != "details=true" {
			return false
		}
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"partitions": []interface{}{
				map[string]interface{}{
					"type":       "identity",
					"name":       "p1",
					"fieldNames": []interface{}{[]interface{}{"col1"}},
					"values":     []interface{}{map[string]interface{}{"type": "literal", "dataType": "string", "value": "v1"}},
					"properties": map[string]interface{}{},
				},
				map[string]interface{}{
					"type":       "range",
					"name":       "r1",
					"upper":      map[string]interface{}{"type": "literal", "dataType": "date", "value": "2024-01-01"},
					"lower":      map[string]interface{}{"type": "literal", "dataType": "date", "value": "2023-01-01"},
					"properties": map[string]interface{}{},
				},
			},
		})
		return true
	})

	d := NewPartitionsDataSource().(*PartitionsDataSource)
	d.client = c

	schemaObj := partitionSchema(t, d).Schema
	config := PartitionsDataSourceModel{
		Metalake:   types.StringValue(testMetalake),
		Catalog:    types.StringValue(testCatalog),
		Schema:     types.StringValue(testSchema),
		Table:      types.StringValue(testTable),
		Partitions: types.ListNull(types.ObjectType{AttrTypes: models.PartitionSpecAttrTypes}),
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
		t.Fatalf("the details parameter must be used instead of one request per partition, got %v", *recorded)
	}
	if (*recorded)[0].RawQuery != "details=true" {
		t.Errorf("query = %q, want details=true", (*recorded)[0].RawQuery)
	}

	var state PartitionsDataSourceModel
	if d := resp.State.Get(context.Background(), &state); d.HasError() {
		t.Fatalf("reading state: %v", d)
	}

	elements := state.Partitions.Elements()
	if len(elements) != 2 {
		t.Fatalf("partitions = %v", elements)
	}

	first, ok := elements[0].(types.Object)
	if !ok {
		t.Fatalf("partition element is %T", elements[0])
	}
	attributes := first.Attributes()
	if got := attributes["name"].(types.String).ValueString(); got != "p1" {
		t.Errorf("name = %q", got)
	}
	if got := attributes["type"].(types.String).ValueString(); got != models.PartitionTypeIdentity {
		t.Errorf("type = %q", got)
	}

	second, _ := elements[1].(types.Object)
	if got := second.Attributes()["upper"].(types.Object); got.IsNull() {
		t.Error("the upper bound of the range partition must be reported")
	}
}

func TestPartitionDataSource_ReadNotFoundIsAnError(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		return false
	})

	d := NewPartitionDataSource().(*PartitionDataSource)
	d.client = c

	schemaObj := partitionSchema(t, d).Schema
	config := PartitionDataSourceModel{
		Metalake:   types.StringValue(testMetalake),
		Catalog:    types.StringValue(testCatalog),
		Schema:     types.StringValue(testSchema),
		Table:      types.StringValue(testTable),
		Name:       types.StringValue("p1"),
		FieldNames: types.ListNull(types.ListType{ElemType: types.StringType}),
		Values:     types.ListNull(types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes}),
		Upper:      types.ObjectNull(models.PartitionLiteralAttrTypes),
		Lower:      types.ObjectNull(models.PartitionLiteralAttrTypes),
		Lists:      types.ListNull(types.ListType{ElemType: types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes}}),
		Properties: types.MapNull(types.StringType),
	}

	obj, _ := types.ObjectValueFrom(context.Background(), schemaObj.Type().(types.ObjectType).AttributeTypes(), config)
	value, _ := obj.ToTerraformValue(context.Background())

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(context.Background(), datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaObj, Raw: value},
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a missing partition")
	}
	found := false
	for _, diag := range resp.Diagnostics.Errors() {
		if strings.Contains(diag.Detail(), "NoSuchMetalakeException") {
			found = true
		}
	}
	if !found {
		t.Errorf("the Gravitino error type must be reported, got %v", resp.Diagnostics.Errors())
	}
}
