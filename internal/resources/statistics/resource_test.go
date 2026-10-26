package statistics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	testMetalake = "ml"
	testObject   = "sales.orders"

	testStatsPath = "/api/metalakes/ml/objects/TABLE/sales.orders/statistics"
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
			_, _ = w.Write([]byte(`{"code":1003,"type":"NoSuchMetadataObjectException","message":"Metadata object does not exist","stack":["org.apache.gravitino.exceptions.NoSuchMetadataObjectException: Metadata object does not exist"]}`))
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

func mapValue(t *testing.T, statistics map[string]string) types.Map {
	t.Helper()

	value, diags := types.MapValueFrom(context.Background(), types.StringType, statistics)
	if diags.HasError() {
		t.Fatalf("building map value: %v", diags)
	}
	return value
}

func statisticsPlan(t *testing.T, schemaObj schema.Schema, model StatisticsResourceModel) tfsdk.Plan {
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

func statisticsState(t *testing.T, schemaObj schema.Schema, model StatisticsResourceModel) tfsdk.State {
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

func newStatisticsModel(t *testing.T, statistics map[string]string) StatisticsResourceModel {
	t.Helper()

	return StatisticsResourceModel{
		ID:             types.StringUnknown(),
		Metalake:       types.StringValue(testMetalake),
		ObjectType:     types.StringValue("TABLE"),
		ObjectFullName: types.StringValue(testObject),
		Statistics:     mapValue(t, statistics),
	}
}

func statisticsModelFrom(t *testing.T, state tfsdk.State) StatisticsResourceModel {
	t.Helper()

	var model StatisticsResourceModel
	if diags := state.Get(context.Background(), &model); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	return model
}

func statisticsMap(t *testing.T, value types.Map) map[string]string {
	t.Helper()

	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("statistics = %v, want a known map", value)
	}
	result := make(map[string]string)
	if diags := value.ElementsAs(context.Background(), &result, false); diags.HasError() {
		t.Fatalf("decoding statistics: %v", diags)
	}
	return result
}

func TestStatisticsResource_Schema(t *testing.T) {
	r := New()
	metadata := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, metadata)
	if metadata.TypeName != "gravitino_statistics" {
		t.Fatalf("type name = %q", metadata.TypeName)
	}

	schemaObj := resourceSchema(t, r)
	for _, name := range []string{"id", "metalake", "object_type", "object_full_name", "statistics"} {
		if _, ok := schemaObj.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	statistics, ok := schemaObj.Attributes["statistics"].(schema.MapAttribute)
	if !ok {
		t.Fatalf("statistics = %T, want schema.MapAttribute", schemaObj.Attributes["statistics"])
	}
	if !statistics.Required {
		t.Error("statistics must be required")
	}
	if !statistics.ElementType.Equal(types.StringType) {
		t.Errorf("statistics element type = %v", statistics.ElementType)
	}

	validators := statistics.MapValidators()
	if len(validators) == 0 {
		t.Fatal("statistics must carry the statistic name validator")
	}
}

func TestStatisticsResource_CreateSendsUpdates(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		switch {
		case req.Method == http.MethodGet && req.Path == testStatsPath:
			_, _ = w.Write([]byte(`{"code":0,"statistics":[]}`))
			return true
		case req.Method == http.MethodPut && req.Path == testStatsPath:
			writeJSON(t, w, map[string]interface{}{"code": 0})
			return true
		}
		return false
	})

	r := New().(*StatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: statisticsPlan(t, schemaObj, newStatisticsModel(t, map[string]string{"custom-a": "1", "custom-b": "2"})),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %v", resp.Diagnostics)
	}

	put := findRequest(t, *recorded, http.MethodPut, testStatsPath)
	want := map[string]interface{}{
		"updates": map[string]interface{}{"custom-a": "1", "custom-b": "2"},
	}
	if !reflect.DeepEqual(want, put.Body) {
		wantJSON, _ := json.Marshal(want)
		gotJSON, _ := json.Marshal(put.Body)
		t.Errorf("update payload mismatch\nwant: %s\ngot:  %s", wantJSON, gotJSON)
	}

	state := statisticsModelFrom(t, resp.State)
	if got := state.ID.ValueString(); got != "ml.TABLE.sales.orders" {
		t.Errorf("id = %q", got)
	}
	if got := statisticsMap(t, state.Statistics); !reflect.DeepEqual(got, map[string]string{"custom-a": "1", "custom-b": "2"}) {
		t.Errorf("statistics = %v", got)
	}
}

func TestStatisticsResource_CreateDropsUnmanagedStatistics(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		switch {
		case req.Method == http.MethodGet && req.Path == testStatsPath:
			_, _ = w.Write([]byte(`{"code":0,"statistics":[
				{"name":"custom-old","value":"old","reserved":false,"modifiable":true},
				{"name":"row_count","value":"10","reserved":true,"modifiable":false}
			]}`))
			return true
		case req.Method == http.MethodPut && req.Path == testStatsPath:
			writeJSON(t, w, map[string]interface{}{"code": 0})
			return true
		case req.Method == http.MethodPost && req.Path == testStatsPath:
			writeJSON(t, w, map[string]interface{}{"code": 0, "dropped": true})
			return true
		}
		return false
	})

	r := New().(*StatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: statisticsPlan(t, schemaObj, newStatisticsModel(t, map[string]string{"custom-new": "new"})),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %v", resp.Diagnostics)
	}

	post := findRequest(t, *recorded, http.MethodPost, testStatsPath)
	want := map[string]interface{}{"names": []interface{}{"custom-old"}}
	if !reflect.DeepEqual(want, post.Body) {
		t.Errorf("drop payload = %v, want %v", post.Body, want)
	}

	state := statisticsModelFrom(t, resp.State)
	if got := statisticsMap(t, state.Statistics); !reflect.DeepEqual(got, map[string]string{"custom-new": "new"}) {
		t.Errorf("statistics = %v", got)
	}
}

// statListResponse is the StatisticListResponse example of statistics.yaml,
// with a numeric custom value to pin the string rendering.
const statListResponse = `{"code":0,"statistics":[
	{"name":"row_count","value":"1000","reserved":true,"modifiable":false,"audit":{"creator":"user1","createTime":"2023-10-01T12:00:00Z"}},
	{"name":"custom-k1","value":"v1","reserved":false,"modifiable":true,"audit":{"creator":"user1","createTime":"2023-10-01T12:00:00Z"}},
	{"name":"custom-k2","value":42,"reserved":false,"modifiable":true,"audit":{"creator":"user1","createTime":"2023-10-01T12:00:00Z"}}
]}`

func TestStatisticsResource_ReadKeepsOnlyModifiableStatistics(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method == http.MethodGet && req.Path == testStatsPath {
			_, _ = w.Write([]byte(statListResponse))
			return true
		}
		return false
	})

	r := New().(*StatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	stateModel := newStatisticsModel(t, nil)
	stateModel.ID = types.StringValue("ml.TABLE.sales.orders")

	resp := &resource.ReadResponse{State: statisticsState(t, schemaObj, stateModel)}
	r.Read(context.Background(), resource.ReadRequest{
		State: statisticsState(t, schemaObj, stateModel),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected read diagnostics: %v", resp.Diagnostics)
	}

	state := statisticsModelFrom(t, resp.State)
	got := statisticsMap(t, state.Statistics)
	want := map[string]string{"custom-k1": "v1", "custom-k2": "42"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("statistics = %v, want %v (reserved statistics must be ignored)", got, want)
	}
}

func TestStatisticsResource_ReadNotFoundRemovesResource(t *testing.T) {
	c, _ := newMockServer(t, func(recordedRequest, http.ResponseWriter) bool { return false })

	r := New().(*StatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	stateModel := newStatisticsModel(t, map[string]string{"custom-a": "1"})
	stateModel.ID = types.StringValue("ml.TABLE.sales.orders")

	resp := &resource.ReadResponse{State: statisticsState(t, schemaObj, stateModel)}
	r.Read(context.Background(), resource.ReadRequest{
		State: statisticsState(t, schemaObj, stateModel),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected read diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a 404 must remove the resource from state")
	}
}

func TestStatisticsResource_UpdateSendsOnlyTheDifference(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		switch {
		case req.Method == http.MethodGet && req.Path == testStatsPath:
			_, _ = w.Write([]byte(`{"code":0,"statistics":[
				{"name":"custom-a","value":"1","reserved":false,"modifiable":true},
				{"name":"custom-b","value":"2","reserved":false,"modifiable":true}
			]}`))
			return true
		case req.Method == http.MethodPut && req.Path == testStatsPath:
			writeJSON(t, w, map[string]interface{}{"code": 0})
			return true
		case req.Method == http.MethodPost && req.Path == testStatsPath:
			writeJSON(t, w, map[string]interface{}{"code": 0, "dropped": true})
			return true
		}
		return false
	})

	r := New().(*StatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	stateModel := newStatisticsModel(t, map[string]string{"custom-a": "1", "custom-b": "2"})
	stateModel.ID = types.StringValue("ml.TABLE.sales.orders")
	planModel := newStatisticsModel(t, map[string]string{"custom-a": "1", "custom-c": "3"})
	planModel.ID = types.StringValue("ml.TABLE.sales.orders")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  statisticsPlan(t, schemaObj, planModel),
		State: statisticsState(t, schemaObj, stateModel),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected update diagnostics: %v", resp.Diagnostics)
	}

	put := findRequest(t, *recorded, http.MethodPut, testStatsPath)
	if want := map[string]interface{}{"updates": map[string]interface{}{"custom-c": "3"}}; !reflect.DeepEqual(want, put.Body) {
		t.Errorf("update payload = %v, want %v", put.Body, want)
	}

	post := findRequest(t, *recorded, http.MethodPost, testStatsPath)
	if want := map[string]interface{}{"names": []interface{}{"custom-b"}}; !reflect.DeepEqual(want, post.Body) {
		t.Errorf("drop payload = %v, want %v", post.Body, want)
	}
}

func TestStatisticsResource_DeleteDropsEveryManagedStatistic(t *testing.T) {
	c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method == http.MethodPost && req.Path == testStatsPath {
			writeJSON(t, w, map[string]interface{}{"code": 0, "dropped": true})
			return true
		}
		return false
	})

	r := New().(*StatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	stateModel := newStatisticsModel(t, map[string]string{"custom-b": "2", "custom-a": "1"})
	stateModel.ID = types.StringValue("ml.TABLE.sales.orders")

	resp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: statisticsState(t, schemaObj, stateModel),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected delete diagnostics: %v", resp.Diagnostics)
	}

	post := findRequest(t, *recorded, http.MethodPost, testStatsPath)
	want := map[string]interface{}{"names": []interface{}{"custom-a", "custom-b"}}
	if !reflect.DeepEqual(want, post.Body) {
		t.Errorf("drop payload = %v, want %v", post.Body, want)
	}
}

func TestStatisticsResource_DeleteToleratesNotFound(t *testing.T) {
	c, _ := newMockServer(t, func(recordedRequest, http.ResponseWriter) bool { return false })

	r := New().(*StatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	stateModel := newStatisticsModel(t, map[string]string{"custom-a": "1"})
	stateModel.ID = types.StringValue("ml.TABLE.sales.orders")

	resp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: statisticsState(t, schemaObj, stateModel),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 during delete must be success, got %v", resp.Diagnostics)
	}
}

func TestStatisticsResource_UnmodifiableStatisticDiagnostic(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		switch {
		case req.Method == http.MethodGet && req.Path == testStatsPath:
			_, _ = w.Write([]byte(`{"code":0,"statistics":[]}`))
			return true
		case req.Method == http.MethodPut && req.Path == testStatsPath:
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":1006,"type":"UnmodifiableStatisticException","message":"Statistic is unmodifiable","stack":["org.apache.gravitino.exceptions.UnmodifiableStatisticException: Statistic is unmodifiable"]}`))
			return true
		}
		return false
	})

	r := New().(*StatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: statisticsPlan(t, schemaObj, newStatisticsModel(t, map[string]string{"custom-a": "1"})),
	}, resp)

	summary, detail := diagnostics(t, resp.Diagnostics.Errors())
	if !strings.Contains(summary, "unmodifiable") {
		t.Errorf("summary = %q, want it to name the unmodifiable statistic", summary)
	}
	for _, want := range []string{"catalog", "409", "UnmodifiableStatisticException"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail = %q, want it to mention %q", detail, want)
		}
	}
}

func TestStatisticsResource_NotImplementedDiagnostic(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		switch {
		case req.Method == http.MethodGet && req.Path == testStatsPath:
			_, _ = w.Write([]byte(`{"code":0,"statistics":[]}`))
			return true
		case req.Method == http.MethodPut && req.Path == testStatsPath:
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte(`{"code":1007,"type":"UnsupportedOperationException","message":"Statistics are not supported","stack":["org.apache.gravitino.exceptions.UnsupportedOperationException: Statistics are not supported"]}`))
			return true
		}
		return false
	})

	r := New().(*StatisticsResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: statisticsPlan(t, schemaObj, newStatisticsModel(t, map[string]string{"custom-a": "1"})),
	}, resp)

	summary, detail := diagnostics(t, resp.Diagnostics.Errors())
	if !strings.Contains(summary, "not supported") {
		t.Errorf("summary = %q, want it to name the unsupported operation", summary)
	}
	for _, want := range []string{"catalog", "501", "lakehouse"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail = %q, want it to mention %q", detail, want)
		}
	}
}

func TestStatisticsResource_ImportState(t *testing.T) {
	r := New().(*StatisticsResource)
	schemaObj := resourceSchema(t, r)
	_ = schemaObj

	resp := &resource.ImportStateResponse{State: nullState(t, schemaObj)}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "ml.TABLE.sales.orders"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected import diagnostics: %v", resp.Diagnostics)
	}

	checkImportedAttribute(t, resp, "metalake", "ml")
	checkImportedAttribute(t, resp, "object_type", "TABLE")
	checkImportedAttribute(t, resp, "object_full_name", "sales.orders")
	checkImportedAttribute(t, resp, "id", "ml.TABLE.sales.orders")
}

func TestStatisticsResource_ImportStateInvalid(t *testing.T) {
	r := New().(*StatisticsResource)
	schemaObj := resourceSchema(t, r)

	for _, id := range []string{"ml.TABLE", "ml.NOT_A_TYPE.sales.orders"} {
		resp := &resource.ImportStateResponse{State: nullState(t, schemaObj)}
		r.ImportState(context.Background(), resource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("import ID %q must be rejected", id)
		}
	}
}

func findRequest(t *testing.T, recorded []recordedRequest, method, path string) recordedRequest {
	t.Helper()

	for _, req := range recorded {
		if req.Method == method && req.Path == path {
			return req
		}
	}
	t.Fatalf("no %s %s request recorded; got %+v", method, path, recorded)
	return recordedRequest{}
}

func diagnostics(t *testing.T, errors diag.Diagnostics) (string, string) {
	t.Helper()

	if !errors.HasError() {
		t.Fatal("expected an error diagnostic")
	}

	var summary, detail strings.Builder
	for _, d := range errors {
		summary.WriteString(d.Summary())
		summary.WriteString("\n")
		detail.WriteString(d.Detail())
		detail.WriteString("\n")
	}
	return summary.String(), detail.String()
}

func checkImportedAttribute(t *testing.T, resp *resource.ImportStateResponse, name, want string) {
	t.Helper()

	var value types.String
	if diags := resp.State.GetAttribute(context.Background(), path.Root(name), &value); diags.HasError() {
		t.Fatalf("reading %s: %v", name, diags)
	}
	if value.ValueString() != want {
		t.Errorf("%s = %q, want %q", name, value.ValueString(), want)
	}
}
