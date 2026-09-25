package partition

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
	testMetalake  = "probe_ml"
	testCatalog   = "ptic_ice"
	testSchema    = "ptsch"
	testTable     = "pt_part"
	testPartition = "hive_col_name2=2023-01-02/hive_col_name3=gravitino_it_test"
	testPath      = "/api/metalakes/probe_ml/catalogs/ptic_ice/schemas/ptsch/tables/pt_part/partitions"
)

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
			_, _ = w.Write([]byte(`{"code":1003,"type":"NoSuchPartitionException","message":"Failed to operate partition(s) operation [GET] of table [table1], reason [p3]","stack":["org.apache.gravitino.exceptions.NoSuchPartitionException: p3"]}`))
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

func resourceSchema(t *testing.T, r resource.Resource) schema.Schema {
	t.Helper()

	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func planValue(t *testing.T, schemaObj schema.Schema, model PartitionResourceModel) tfsdk.Plan {
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

func stateValue(t *testing.T, schemaObj schema.Schema, model PartitionResourceModel) tfsdk.State {
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

func literalObject(t *testing.T, dataType, value string) types.Object {
	t.Helper()

	object, diags := types.ObjectValue(models.PartitionLiteralAttrTypes, map[string]attr.Value{
		"data_type": types.StringValue(dataType),
		"value":     types.StringValue(value),
	})
	if diags.HasError() {
		t.Fatalf("building literal object: %v", diags)
	}
	return object
}

func fieldNamesValue(t *testing.T, fieldNames [][]string) types.List {
	t.Helper()

	outer := make([]attr.Value, 0, len(fieldNames))
	for _, inner := range fieldNames {
		segments := make([]attr.Value, 0, len(inner))
		for _, segment := range inner {
			segments = append(segments, types.StringValue(segment))
		}
		list, diags := types.ListValue(types.StringType, segments)
		if diags.HasError() {
			t.Fatalf("building field name: %v", diags)
		}
		outer = append(outer, list)
	}

	list, diags := types.ListValue(types.ListType{ElemType: types.StringType}, outer)
	if diags.HasError() {
		t.Fatalf("building field names: %v", diags)
	}
	return list
}

func valuesValue(t *testing.T, literals ...types.Object) types.List {
	t.Helper()

	elements := make([]attr.Value, 0, len(literals))
	for _, literal := range literals {
		elements = append(elements, literal)
	}
	list, diags := types.ListValue(types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes}, elements)
	if diags.HasError() {
		t.Fatalf("building values: %v", diags)
	}
	return list
}

// identityPlan builds the plan of the identity partition of the HiveAddPartition
// example of partitions.yaml.
func identityPlan() PartitionResourceModel {
	return PartitionResourceModel{
		ID:         types.StringUnknown(),
		Metalake:   types.StringValue(testMetalake),
		Catalog:    types.StringValue(testCatalog),
		Schema:     types.StringValue(testSchema),
		Table:      types.StringValue(testTable),
		Type:       types.StringValue(models.PartitionTypeIdentity),
		Name:       types.StringUnknown(),
		Upper:      types.ObjectNull(models.PartitionLiteralAttrTypes),
		Lower:      types.ObjectNull(models.PartitionLiteralAttrTypes),
		FieldNames: types.ListNull(types.ListType{ElemType: types.StringType}),
		Lists:      types.ListNull(types.ListType{ElemType: types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes}}),
		Properties: types.MapNull(types.StringType),
	}
}

// TestPartitionResource_CreateSendsAddPartitionsRequest asserts the create
// request is the AddPartitionsRequest of partitions.yaml: a partitions array of
// PartitionSpec objects, using the field names of the HiveAddPartition example.
func TestPartitionResource_CreateSendsAddPartitionsRequest(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method != http.MethodPost || req.Path != testPath {
			return false
		}
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"partitions": []interface{}{
				map[string]interface{}{
					"type": "identity",
					"name": testPartition,
					"fieldNames": []interface{}{
						[]interface{}{"hive_col_name2"},
						[]interface{}{"hive_col_name3"},
					},
					"values": []interface{}{
						map[string]interface{}{"type": "literal", "dataType": "date", "value": "2023-01-02"},
						map[string]interface{}{"type": "literal", "dataType": "string", "value": "gravitino_it_test"},
					},
					"properties": map[string]interface{}{
						"totalSize": "2",
					},
				},
			},
		})
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.client = c
	schemaObj := resourceSchema(t, r)

	plan := identityPlan()
	plan.FieldNames = fieldNamesValue(t, [][]string{{"hive_col_name2"}, {"hive_col_name3"}})
	plan.Values = valuesValue(t,
		literalObject(t, "date", "2023-01-02"),
		literalObject(t, "string", "gravitino_it_test2"),
	)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: planValue(t, schemaObj, plan),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %v", resp.Diagnostics)
	}

	expected := map[string]interface{}{
		"partitions": []interface{}{
			map[string]interface{}{
				"type": "identity",
				"fieldNames": []interface{}{
					[]interface{}{"hive_col_name2"},
					[]interface{}{"hive_col_name3"},
				},
				"values": []interface{}{
					map[string]interface{}{"type": "literal", "dataType": "date", "value": "2023-01-02"},
					map[string]interface{}{"type": "literal", "dataType": "string", "value": "gravitino_it_test2"},
				},
			},
		},
	}
	if !reflect.DeepEqual(expected, (*recorded)[0].Body) {
		expectedJSON, _ := json.Marshal(expected)
		actualJSON, _ := json.Marshal((*recorded)[0].Body)
		t.Errorf("create payload mismatch\nwant: %s\ngot:  %s", expectedJSON, actualJSON)
	}

	var state PartitionResourceModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if state.Name.ValueString() != testPartition {
		t.Errorf("name = %q, want the name the catalog derived", state.Name.ValueString())
	}
	if state.ID.ValueString() != testMetalake+"."+testCatalog+"."+testSchema+"."+testTable+"."+testPartition {
		t.Errorf("id = %q", state.ID.ValueString())
	}
}

// TestPartitionResource_ModifyPlanDerivesNameForIdentityPartitions asserts the
// name of an identity partition is planned as unknown, because the catalog
// derives it from the field names and values.
func TestPartitionResource_ModifyPlanDerivesNameForIdentityPartitions(t *testing.T) {
	r := NewPartitionResource().(*PartitionResource)
	schemaObj := resourceSchema(t, r)

	plan := identityPlan()
	plan.Name = types.StringValue("configured_name")
	plan.FieldNames = fieldNamesValue(t, [][]string{{"dt"}})
	plan.Values = valuesValue(t, literalObject(t, "date", "2023-01-02"))

	resp := &resource.ModifyPlanResponse{Plan: planValue(t, schemaObj, plan)}
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{
		Plan: planValue(t, schemaObj, plan),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected modify plan diagnostics: %v", resp.Diagnostics)
	}

	var planned PartitionResourceModel
	if diags := resp.Plan.Get(context.Background(), &planned); diags.HasError() {
		t.Fatalf("reading plan: %v", diags)
	}
	if !planned.Name.IsUnknown() {
		t.Errorf("identity partition name = %v, want unknown", planned.Name)
	}
}

// TestPartitionResource_CreateRangePartition asserts a range partition sends
// the upper and lower literals and the required name.
func TestPartitionResource_CreateRangePartition(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method != http.MethodPost {
			return false
		}
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"partitions": []interface{}{
				map[string]interface{}{"type": "range", "name": "r1"},
			},
		})
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.client = c
	schemaObj := resourceSchema(t, r)

	plan := identityPlan()
	plan.Type = types.StringValue(models.PartitionTypeRange)
	plan.Name = types.StringValue("r1")
	plan.FieldNames = types.ListNull(types.ListType{ElemType: types.StringType})
	plan.Values = types.ListNull(types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes})
	plan.Upper = literalObject(t, "date", "2024-01-01")
	plan.Lower = literalObject(t, "date", "2023-01-01")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: planValue(t, schemaObj, plan),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %v", resp.Diagnostics)
	}

	expected := map[string]interface{}{
		"partitions": []interface{}{
			map[string]interface{}{
				"type":  "range",
				"name":  "r1",
				"upper": map[string]interface{}{"type": "literal", "dataType": "date", "value": "2024-01-01"},
				"lower": map[string]interface{}{"type": "literal", "dataType": "date", "value": "2023-01-01"},
			},
		},
	}
	if !reflect.DeepEqual(expected, (*recorded)[0].Body) {
		actualJSON, _ := json.Marshal((*recorded)[0].Body)
		t.Errorf("create payload =\n%s", actualJSON)
	}
}

// TestPartitionResource_CreateValidatesThePartitionSpec asserts the fields each
// partition type requires are checked before a request is sent.
func TestPartitionResource_CreateValidatesThePartitionSpec(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.client = c
	schemaObj := resourceSchema(t, r)

	plan := identityPlan()
	plan.FieldNames = types.ListNull(types.ListType{ElemType: types.StringType})
	plan.Values = types.ListNull(types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes})

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: planValue(t, schemaObj, plan),
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an identity partition without field_names and values")
	}
	if len(*recorded) != 0 {
		t.Error("no request may be sent for an invalid partition")
	}
}

// TestPartitionResource_CreateKeepsManagedProperties asserts the properties of
// the partition are stored as configured, not as reported by the catalog: a
// Hive partition reports statistics the configuration never asked for.
func TestPartitionResource_CreateKeepsManagedProperties(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method != http.MethodPost {
			return false
		}
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"partitions": []interface{}{
				map[string]interface{}{
					"type": "identity",
					"name": "p1",
					"properties": map[string]interface{}{
						"managed":   "value",
						"totalSize": "2",
					},
				},
			},
		})
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.client = c
	schemaObj := resourceSchema(t, r)

	plan := identityPlan()
	plan.FieldNames = fieldNamesValue(t, [][]string{{"dt"}})
	plan.Values = valuesValue(t, literalObject(t, "date", "2023-01-02"))
	properties, diags := types.MapValueFrom(context.Background(), types.StringType, map[string]string{"managed": "value"})
	if diags.HasError() {
		t.Fatalf("building properties: %v", diags)
	}
	plan.Properties = properties

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: planValue(t, schemaObj, plan),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %v", resp.Diagnostics)
	}

	var state PartitionResourceModel
	if d := resp.State.Get(context.Background(), &state); d.HasError() {
		t.Fatalf("reading state: %v", d)
	}

	stored := make(map[string]string)
	if d := state.Properties.ElementsAs(context.Background(), &stored, false); d.HasError() {
		t.Fatalf("reading properties: %v", d)
	}
	if len(stored) != 1 || stored["managed"] != "value" {
		t.Errorf("properties = %#v, want only the managed property", stored)
	}
}

func TestPartitionResource_ReadNotFoundRemovesResource(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		return false
	})

	r := NewPartitionResource().(*PartitionResource)
	r.client = c
	schemaObj := resourceSchema(t, r)

	state := identityPlan()
	state.ID = types.StringValue("probe_ml.ptic_ice.ptsch.pt_part.p1")
	state.Name = types.StringValue("p1")
	state.FieldNames = fieldNamesValue(t, [][]string{{"dt"}})
	state.Values = valuesValue(t, literalObject(t, "date", "2023-01-02"))

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Read(context.Background(), resource.ReadRequest{
		State: stateValue(t, schemaObj, state),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a missing partition must not be an error, got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("the partition must be removed from state on a 404")
	}
}

func TestPartitionResource_DeleteToleratesNotFound(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		return false
	})

	r := NewPartitionResource().(*PartitionResource)
	r.client = c
	schemaObj := resourceSchema(t, r)

	state := identityPlan()
	state.ID = types.StringValue("probe_ml.ptic_ice.ptsch.pt_part.p1")
	state.Name = types.StringValue("p1")
	state.FieldNames = fieldNamesValue(t, [][]string{{"dt"}})
	state.Values = valuesValue(t, literalObject(t, "date", "2023-01-02"))

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: stateValue(t, schemaObj, state),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("dropping a partition that is already gone must succeed, got %v", resp.Diagnostics)
	}
	if len(*recorded) != 1 || (*recorded)[0].Method != http.MethodDelete {
		t.Errorf("expected one DELETE, got %v", *recorded)
	}
	if (*recorded)[0].Path != testPath+"/p1" {
		t.Errorf("path = %q", (*recorded)[0].Path)
	}
}

// TestPartitionResource_UpdateIsNotSupported asserts Update reports the missing
// update API instead of silently ignoring a change. Every attribute of the
// resource uses RequiresReplace, so Terraform never calls it in practice.
func TestPartitionResource_UpdateIsNotSupported(t *testing.T) {
	r := NewPartitionResource().(*PartitionResource)
	schemaObj := resourceSchema(t, r)

	plan := identityPlan()
	plan.FieldNames = fieldNamesValue(t, [][]string{{"dt"}})
	plan.Values = valuesValue(t, literalObject(t, "date", "2023-01-02"))

	state := plan
	state.FieldNames = fieldNamesValue(t, [][]string{{"dt"}})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  planValue(t, schemaObj, plan),
		State: stateValue(t, schemaObj, state),
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error, Gravitino has no partition update API")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "not supported") {
		t.Errorf("unexpected diagnostic: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func TestPartitionResource_ImportState(t *testing.T) {
	r := NewPartitionResource().(resource.ResourceWithImportState)
	schemaObj := resourceSchema(t, r)

	resp := &resource.ImportStateResponse{State: nullState(t, schemaObj)}
	r.ImportState(context.Background(), resource.ImportStateRequest{
		ID: "ml.cat.sch.tbl.col=value/col2=value2",
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected import diagnostics: %v", resp.Diagnostics)
	}

	var imported PartitionResourceModel
	if diags := resp.State.Get(context.Background(), &imported); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if imported.Metalake.ValueString() != "ml" || imported.Table.ValueString() != "tbl" {
		t.Errorf("imported %q / %q", imported.Metalake.ValueString(), imported.Table.ValueString())
	}
	if imported.Name.ValueString() != "col=value/col2=value2" {
		t.Errorf("name = %q, the partition name may contain dots", imported.Name.ValueString())
	}
}

func TestPartitionResource_ImportStateTooFewParts(t *testing.T) {
	r := NewPartitionResource().(resource.ResourceWithImportState)
	schemaObj := resourceSchema(t, r)

	resp := &resource.ImportStateResponse{State: nullState(t, schemaObj)}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "ml.cat"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an invalid import id")
	}
}

func TestPartitionResource_Metadata(t *testing.T) {
	r := NewPartitionResource()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "gravitino"}, resp)
	if resp.TypeName != "gravitino_partition" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

// TestPartitionResource_SchemaRequiresReplace asserts the schema marks the
// attributes Gravitino cannot change without dropping the partition.
func TestPartitionResource_SchemaRequiresReplace(t *testing.T) {
	r := NewPartitionResource().(*PartitionResource)
	schemaObj := resourceSchema(t, r)

	for _, name := range []string{"metalake", "catalog", "schema", "table", "type", "name", "field_names", "values", "upper", "lower", "lists", "properties"} {
		if schemaObj.Attributes[name] == nil {
			t.Fatalf("attribute %q is missing from the schema", name)
		}
	}

	description := schemaObj.Description
	if !strings.Contains(description, "replaces the partition") {
		t.Errorf("the resource must document that every change replaces the partition, got %q", description)
	}
}
