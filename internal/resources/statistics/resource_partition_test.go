package statistics

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const testPartitionStatsPath = "/api/metalakes/ml/objects/TABLE/sales.public.orders/statistics/partitions"

func partitionsValue(t *testing.T, partitions map[string]map[string]string) types.Map {
	t.Helper()

	value, diags := partitionsToMap(context.Background(), partitions)
	if diags.HasError() {
		t.Fatalf("building partitions value: %v", diags)
	}
	return value
}

func newPartitionStatisticsModel(t *testing.T, partitions map[string]map[string]string) PartitionStatisticsResourceModel {
	t.Helper()

	return PartitionStatisticsResourceModel{
		ID:         types.StringUnknown(),
		Metalake:   types.StringValue(testMetalake),
		Catalog:    types.StringValue("sales"),
		Schema:     types.StringValue("public"),
		Table:      types.StringValue("orders"),
		Partitions: partitionsValue(t, partitions),
	}
}

func partitionStatisticsPlan(t *testing.T, schemaObj schema.Schema, model PartitionStatisticsResourceModel) tfsdk.Plan {
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

func partitionStatisticsState(t *testing.T, schemaObj schema.Schema, model PartitionStatisticsResourceModel) tfsdk.State {
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

func partitionStatisticsModelFrom(t *testing.T, state tfsdk.State) PartitionStatisticsResourceModel {
	t.Helper()

	var model PartitionStatisticsResourceModel
	if diags := state.Get(context.Background(), &model); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	return model
}

func partitionsMap(t *testing.T, value types.Map) map[string]map[string]string {
	t.Helper()

	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("partitions = %v, want a known map", value)
	}

	result := make(map[string]map[string]string, len(value.Elements()))
	for partition, element := range value.Elements() {
		statistics, ok := element.(types.Map)
		if !ok {
			t.Fatalf("partition %q = %T, want types.Map", partition, element)
		}
		decoded := make(map[string]string)
		if diags := statistics.ElementsAs(context.Background(), &decoded, false); diags.HasError() {
			t.Fatalf("decoding partition %q: %v", partition, diags)
		}
		result[partition] = decoded
	}
	return result
}

func TestPartitionStatisticsResource_Schema(t *testing.T) {
	r := NewPartition()
	metadata := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, metadata)
	if metadata.TypeName != "gravitino_partition_statistics" {
		t.Fatalf("type name = %q", metadata.TypeName)
	}

	schemaObj := resourceSchema(t, r)
	for _, name := range []string{"id", "metalake", "catalog", "schema", "table", "partitions"} {
		if _, ok := schemaObj.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	partitions, ok := schemaObj.Attributes["partitions"].(schema.MapAttribute)
	if !ok {
		t.Fatalf("partitions = %T, want schema.MapAttribute", schemaObj.Attributes["partitions"])
	}
	elementType, ok := partitions.ElementType.(types.MapType)
	if !ok {
		t.Fatalf("partitions element type = %T, want types.MapType", partitions.ElementType)
	}
	if !elementType.ElemType.Equal(types.StringType) {
		t.Errorf("partitions element type = %v", elementType)
	}
	if len(partitions.MapValidators()) == 0 {
		t.Fatal("partitions must carry the statistic name validator")
	}
}

func TestPartitionStatisticsResource_NameValidator(t *testing.T) {
	v := partitionStatisticsValidator{}
	configValue, diags := partitionsToMap(context.Background(), map[string]map[string]string{
		"dt=2024-01-01": {"custom-ok": "1", "not-custom": "2"},
	})
	if diags.HasError() {
		t.Fatalf("building value: %v", diags)
	}

	resp := &validator.MapResponse{}
	v.ValidateMap(context.Background(), validator.MapRequest{
		ConfigValue: configValue,
		Path:        path.Root("partitions"),
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a statistic name without the custom- prefix must be rejected")
	}
	if got := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(got, "not-custom") {
		t.Errorf("detail = %q, want it to name the rejected statistic", got)
	}

	empty, diags := partitionsToMap(context.Background(), map[string]map[string]string{"p0": {}})
	if diags.HasError() {
		t.Fatalf("building empty value: %v", diags)
	}
	emptyResp := &validator.MapResponse{}
	v.ValidateMap(context.Background(), validator.MapRequest{
		ConfigValue: empty,
		Path:        path.Root("partitions"),
	}, emptyResp)
	if !emptyResp.Diagnostics.HasError() {
		t.Fatal("an empty partition statistics map must be rejected (it would never converge)")
	}
}

func TestPartitionStatisticsResource_CreateSendsUpdates(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		switch {
		case req.Method == http.MethodGet && req.Path == testPartitionStatsPath:
			_, _ = w.Write([]byte(`{"code":0,"partitionStatistics":[]}`))
			return true
		case req.Method == http.MethodPut && req.Path == testPartitionStatsPath:
			writeJSON(t, w, map[string]interface{}{"code": 0})
			return true
		}
		return false
	})

	r := NewPartition().(*PartitionStatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: partitionStatisticsPlan(t, schemaObj, newPartitionStatisticsModel(t, map[string]map[string]string{
			"p0": {"custom-a": "1"},
			"p1": {"custom-b": "2"},
		})),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %v", resp.Diagnostics)
	}

	put := findRequest(t, *recorded, http.MethodPut, testPartitionStatsPath)
	want := map[string]interface{}{
		"updates": []interface{}{
			map[string]interface{}{
				"partitionName": "p0",
				"statistics":    map[string]interface{}{"custom-a": "1"},
			},
			map[string]interface{}{
				"partitionName": "p1",
				"statistics":    map[string]interface{}{"custom-b": "2"},
			},
		},
	}
	if !reflect.DeepEqual(want, put.Body) {
		wantJSON, _ := json.Marshal(want)
		gotJSON, _ := json.Marshal(put.Body)
		t.Errorf("update payload mismatch\nwant: %s\ngot:  %s", wantJSON, gotJSON)
	}

	state := partitionStatisticsModelFrom(t, resp.State)
	if got := state.ID.ValueString(); got != "ml.sales.public.orders" {
		t.Errorf("id = %q", got)
	}
	if got := partitionsMap(t, state.Partitions); !reflect.DeepEqual(got, map[string]map[string]string{
		"p0": {"custom-a": "1"},
		"p1": {"custom-b": "2"},
	}) {
		t.Errorf("partitions = %v", got)
	}
}

const partitionStatListResponse = `{"code":0,"partitionStatistics":[
	{
		"partitionName":"p0",
		"statistics":[
			{"name":"custom-row_count","value":"1000","reserved":false,"modifiable":true,"audit":{"creator":"user1","createTime":"2023-10-01T12:00:00Z"}},
			{"name":"row_count","value":"10","reserved":true,"modifiable":false,"audit":{"creator":"user1","createTime":"2023-10-01T12:00:00Z"}}
		]
	}
]}`

func TestPartitionStatisticsResource_ReadKeepsOnlyModifiableStatistics(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method == http.MethodGet && req.Path == testPartitionStatsPath {
			_, _ = w.Write([]byte(partitionStatListResponse))
			return true
		}
		return false
	})

	r := NewPartition().(*PartitionStatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	stateModel := newPartitionStatisticsModel(t, nil)
	stateModel.ID = types.StringValue("ml.sales.public.orders")

	resp := &resource.ReadResponse{State: partitionStatisticsState(t, schemaObj, stateModel)}
	r.Read(context.Background(), resource.ReadRequest{
		State: partitionStatisticsState(t, schemaObj, stateModel),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected read diagnostics: %v", resp.Diagnostics)
	}

	state := partitionStatisticsModelFrom(t, resp.State)
	if got := partitionsMap(t, state.Partitions); !reflect.DeepEqual(got, map[string]map[string]string{
		"p0": {"custom-row_count": "1000"},
	}) {
		t.Errorf("partitions = %v", got)
	}
}

func TestPartitionStatisticsResource_ReadNotFoundRemovesResource(t *testing.T) {
	c, _ := newMockServer(t, func(recordedRequest, http.ResponseWriter) bool { return false })

	r := NewPartition().(*PartitionStatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	stateModel := newPartitionStatisticsModel(t, map[string]map[string]string{"p0": {"custom-a": "1"}})
	stateModel.ID = types.StringValue("ml.sales.public.orders")

	resp := &resource.ReadResponse{State: partitionStatisticsState(t, schemaObj, stateModel)}
	r.Read(context.Background(), resource.ReadRequest{
		State: partitionStatisticsState(t, schemaObj, stateModel),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected read diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a 404 must remove the resource from state")
	}
}

func TestPartitionStatisticsResource_UpdateDropsRemovedStatistics(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		switch {
		case req.Method == http.MethodGet && req.Path == testPartitionStatsPath:
			_, _ = w.Write([]byte(`{"code":0,"partitionStatistics":[
				{"partitionName":"p0","statistics":[
					{"name":"custom-a","value":"1","reserved":false,"modifiable":true},
					{"name":"custom-b","value":"2","reserved":false,"modifiable":true}
				]},
				{"partitionName":"p1","statistics":[
					{"name":"custom-x","value":"9","reserved":false,"modifiable":true}
				]}
			]}`))
			return true
		case req.Method == http.MethodPut && req.Path == testPartitionStatsPath:
			writeJSON(t, w, map[string]interface{}{"code": 0})
			return true
		case req.Method == http.MethodPost && req.Path == testPartitionStatsPath:
			writeJSON(t, w, map[string]interface{}{"code": 0, "dropped": true})
			return true
		}
		return false
	})

	r := NewPartition().(*PartitionStatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	stateModel := newPartitionStatisticsModel(t, map[string]map[string]string{
		"p0": {"custom-a": "1", "custom-b": "2"},
		"p1": {"custom-x": "9"},
	})
	stateModel.ID = types.StringValue("ml.sales.public.orders")
	planModel := newPartitionStatisticsModel(t, map[string]map[string]string{
		"p0": {"custom-a": "1", "custom-c": "3"},
	})
	planModel.ID = types.StringValue("ml.sales.public.orders")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  partitionStatisticsPlan(t, schemaObj, planModel),
		State: partitionStatisticsState(t, schemaObj, stateModel),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected update diagnostics: %v", resp.Diagnostics)
	}

	put := findRequest(t, *recorded, http.MethodPut, testPartitionStatsPath)
	wantPut := map[string]interface{}{
		"updates": []interface{}{
			map[string]interface{}{
				"partitionName": "p0",
				"statistics":    map[string]interface{}{"custom-c": "3"},
			},
		},
	}
	if !reflect.DeepEqual(wantPut, put.Body) {
		t.Errorf("update payload = %v, want %v", put.Body, wantPut)
	}

	post := findRequest(t, *recorded, http.MethodPost, testPartitionStatsPath)
	wantPost := map[string]interface{}{
		"drops": []interface{}{
			map[string]interface{}{"partitionName": "p0", "statisticNames": []interface{}{"custom-b"}},
			map[string]interface{}{"partitionName": "p1", "statisticNames": []interface{}{"custom-x"}},
		},
	}
	if !reflect.DeepEqual(wantPost, post.Body) {
		t.Errorf("drop payload = %v, want %v", post.Body, wantPost)
	}

	state := partitionStatisticsModelFrom(t, resp.State)
	if got := partitionsMap(t, state.Partitions); !reflect.DeepEqual(got, map[string]map[string]string{
		"p0": {"custom-a": "1", "custom-c": "3"},
	}) {
		t.Errorf("partitions = %v", got)
	}
}

func TestPartitionStatisticsResource_DeleteDropsEveryManagedStatistic(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method == http.MethodPost && req.Path == testPartitionStatsPath {
			writeJSON(t, w, map[string]interface{}{"code": 0, "dropped": true})
			return true
		}
		return false
	})

	r := NewPartition().(*PartitionStatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	stateModel := newPartitionStatisticsModel(t, map[string]map[string]string{
		"p0": {"custom-b": "2", "custom-a": "1"},
	})
	stateModel.ID = types.StringValue("ml.sales.public.orders")

	resp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: partitionStatisticsState(t, schemaObj, stateModel),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected delete diagnostics: %v", resp.Diagnostics)
	}

	post := findRequest(t, *recorded, http.MethodPost, testPartitionStatsPath)
	want := map[string]interface{}{
		"drops": []interface{}{
			map[string]interface{}{"partitionName": "p0", "statisticNames": []interface{}{"custom-a", "custom-b"}},
		},
	}
	if !reflect.DeepEqual(want, post.Body) {
		t.Errorf("drop payload = %v, want %v", post.Body, want)
	}
}

func TestPartitionStatisticsResource_DeleteToleratesNotFound(t *testing.T) {
	c, _ := newMockServer(t, func(recordedRequest, http.ResponseWriter) bool { return false })

	r := NewPartition().(*PartitionStatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	stateModel := newPartitionStatisticsModel(t, map[string]map[string]string{"p0": {"custom-a": "1"}})
	stateModel.ID = types.StringValue("ml.sales.public.orders")

	resp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: partitionStatisticsState(t, schemaObj, stateModel),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 during delete must be success, got %v", resp.Diagnostics)
	}
}

func TestPartitionStatisticsResource_UnmodifiableStatisticDiagnostic(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		switch {
		case req.Method == http.MethodGet && req.Path == testPartitionStatsPath:
			_, _ = w.Write([]byte(`{"code":0,"partitionStatistics":[]}`))
			return true
		case req.Method == http.MethodPut && req.Path == testPartitionStatsPath:
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":1006,"type":"UnmodifiableStatisticException","message":"Statistic is unmodifiable","stack":["org.apache.gravitino.exceptions.UnmodifiableStatisticException: Statistic is unmodifiable"]}`))
			return true
		}
		return false
	})

	r := NewPartition().(*PartitionStatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: partitionStatisticsPlan(t, schemaObj, newPartitionStatisticsModel(t, map[string]map[string]string{
			"p0": {"custom-a": "1"},
		})),
	}, resp)

	summary, detail := diagnostics(t, resp.Diagnostics.Errors())
	if !strings.Contains(summary, "unmodifiable") {
		t.Errorf("summary = %q, want it to name the unmodifiable statistic", summary)
	}
	if !strings.Contains(detail, "catalog") {
		t.Errorf("detail = %q, want it to name the owning catalog", detail)
	}
}

func TestPartitionStatisticsResource_ImportState(t *testing.T) {
	r := NewPartition().(*PartitionStatisticsResource)
	schemaObj := resourceSchema(t, r)

	resp := &resource.ImportStateResponse{State: nullState(t, schemaObj)}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "ml.sales.public.orders"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected import diagnostics: %v", resp.Diagnostics)
	}

	for name, want := range map[string]string{
		"metalake": "ml",
		"catalog":  "sales",
		"schema":   "public",
		"table":    "orders",
		"id":       "ml.sales.public.orders",
	} {
		checkImportedAttribute(t, resp, name, want)
	}
}

func TestPartitionStatisticsResource_ImportStateInvalid(t *testing.T) {
	r := NewPartition().(*PartitionStatisticsResource)
	schemaObj := resourceSchema(t, r)

	for _, id := range []string{"ml.sales.public", "ml..public.orders"} {
		resp := &resource.ImportStateResponse{State: nullState(t, schemaObj)}
		r.ImportState(context.Background(), resource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("import ID %q must be rejected", id)
		}
	}
}
