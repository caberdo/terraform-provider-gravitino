package partition

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPartitionResource_Configure(t *testing.T) {
	r := NewPartitionResource().(*PartitionResource)

	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("nil provider data must configure without diagnostics: %v", resp.Diagnostics)
	}

	resp = &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for unexpected provider data")
	}

	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool { return false })
	resp = &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a *client.Client must configure: %v", resp.Diagnostics)
	}
	if r.client != c {
		t.Fatal("Configure did not store the client")
	}
}

func TestPartitionResource_ModifyPlanWithNullPlan(t *testing.T) {
	r := NewPartitionResource().(*PartitionResource)
	schemaObj := resourceSchema(t, r)
	ctx := context.Background()

	nullPlan, err := types.ObjectNull(schemaObj.Type().(types.ObjectType).AttributeTypes()).ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("building null plan: %v", err)
	}

	resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: schemaObj, Raw: nullPlan}}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan: tfsdk.Plan{Schema: schemaObj, Raw: nullPlan},
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a null plan (destroy) must be ignored, got %v", resp.Diagnostics)
	}
}

func TestPartitionResource_ModifyPlanLeavesNonIdentityName(t *testing.T) {
	r := NewPartitionResource().(*PartitionResource)
	schemaObj := resourceSchema(t, r)

	plan := rangeCreatePlan(t)

	resp := &resource.ModifyPlanResponse{Plan: planValue(t, schemaObj, plan)}
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{
		Plan: planValue(t, schemaObj, plan),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var planned PartitionResourceModel
	if diags := resp.Plan.Get(context.Background(), &planned); diags.HasError() {
		t.Fatalf("reading plan: %v", diags)
	}
	if planned.Name.ValueString() != "r1" {
		t.Errorf("name = %q, want the configured name for a range partition", planned.Name.ValueString())
	}
}

func createPartition(t *testing.T, r *PartitionResource, plan PartitionResourceModel) *resource.CreateResponse {
	t.Helper()
	schemaObj := resourceSchema(t, r)
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Create(context.Background(), resource.CreateRequest{Plan: planValue(t, schemaObj, plan)}, resp)
	return resp
}

func identityCreatePlan(t *testing.T) PartitionResourceModel {
	t.Helper()
	plan := identityPlan()
	plan.FieldNames = fieldNamesValue(t, [][]string{{"dt"}})
	plan.Values = valuesValue(t, literalObject(t, "date", "2023-01-02"))
	return plan
}

func TestPartitionResource_CreateServerError(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(t, w, map[string]interface{}{"code": 1100, "type": "InternalServerError", "message": "boom", "stack": []string{"..."}})
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.SetClient(c)

	plan := identityCreatePlan(t)
	plan.Name = types.StringValue("p1")

	resp := createPartition(t, r, plan)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed adding partition "p1"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

// TestPartitionResource_CreateWithoutReturnedPartition covers a server that
// accepts the request but returns no partition.
func TestPartitionResource_CreateWithoutReturnedPartition(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		writeJSON(t, w, map[string]interface{}{"code": 0, "partitions": []interface{}{}})
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.SetClient(c)

	resp := createPartition(t, r, identityCreatePlan(t))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when no partition is returned")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Partition not returned") {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func rangeCreatePlan(t *testing.T) PartitionResourceModel {
	t.Helper()
	plan := identityPlan()
	plan.Type = types.StringValue(models.PartitionTypeRange)
	plan.Name = types.StringValue("r1")
	plan.FieldNames = types.ListNull(types.ListType{ElemType: types.StringType})
	plan.Values = types.ListNull(types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes})
	plan.Upper = literalObject(t, "date", "2024-01-01")
	plan.Lower = literalObject(t, "date", "2023-01-01")
	return plan
}

func listCreatePlan(t *testing.T) PartitionResourceModel {
	t.Helper()
	plan := identityPlan()
	plan.Type = types.StringValue(models.PartitionTypeList)
	plan.Name = types.StringValue("l1")
	plan.FieldNames = types.ListNull(types.ListType{ElemType: types.StringType})
	plan.Values = types.ListNull(types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes})
	list, diags := types.ListValue(
		types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes},
		[]attr.Value{literalObject(t, "string", "a")},
	)
	if diags.HasError() {
		t.Fatalf("building list: %v", diags)
	}
	lists, diags := types.ListValue(
		types.ListType{ElemType: types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes}},
		[]attr.Value{list},
	)
	if diags.HasError() {
		t.Fatalf("building lists: %v", diags)
	}
	plan.Lists = lists
	return plan
}

// TestPartitionResource_CreateListPartition asserts a list partition sends its
// value lists.
func TestPartitionResource_CreateListPartition(t *testing.T) {
	var body map[string]interface{}
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		body = req.Body
		writeJSON(t, w, map[string]interface{}{
			"code":       0,
			"partitions": []interface{}{map[string]interface{}{"type": "list", "name": "l1"}},
		})
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.SetClient(c)

	resp := createPartition(t, r, listCreatePlan(t))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %v", resp.Diagnostics)
	}
	partitions, ok := body["partitions"].([]interface{})
	if !ok || len(partitions) != 1 {
		t.Fatalf("unexpected payload: %v", body)
	}
	sent := partitions[0].(map[string]interface{})
	if _, ok := sent["lists"]; !ok {
		t.Errorf("the request must carry lists, got %v", sent)
	}
	if _, ok := sent["name"]; !ok {
		t.Errorf("the request must carry the name of a list partition, got %v", sent)
	}
}

// TestPartitionResource_ValidatePartitionSpecRejectsMismatchedFields covers the
// branches that reject fields of another partition type.
func TestPartitionResource_ValidatePartitionSpecRejectsMismatchedFields(t *testing.T) {
	cases := map[string]func(*testing.T, *PartitionResourceModel){
		"identity with upper": func(t *testing.T, m *PartitionResourceModel) {
			m.Upper = literalObject(t, "date", "2024-01-01")
		},
		"identity with lists": func(t *testing.T, m *PartitionResourceModel) {
			m.Lists = listCreatePlan(t).Lists
		},
		"range without name": func(t *testing.T, m *PartitionResourceModel) {
			m.Name = types.StringNull()
		},
		"range without bounds": func(t *testing.T, m *PartitionResourceModel) {
			m.Upper = types.ObjectNull(models.PartitionLiteralAttrTypes)
		},
		"range with field names": func(t *testing.T, m *PartitionResourceModel) {
			m.FieldNames = fieldNamesValue(t, [][]string{{"dt"}})
		},
		"range with lists": func(t *testing.T, m *PartitionResourceModel) {
			m.Lists = listCreatePlan(t).Lists
		},
		"list without name": func(t *testing.T, m *PartitionResourceModel) {
			m.Name = types.StringNull()
		},
		"list without lists": func(t *testing.T, m *PartitionResourceModel) {
			m.Lists = types.ListNull(types.ListType{ElemType: types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes}})
		},
		"list with values": func(t *testing.T, m *PartitionResourceModel) {
			m.Values = valuesValue(t, literalObject(t, "date", "2023-01-02"))
		},
		"list with bounds": func(t *testing.T, m *PartitionResourceModel) {
			m.Upper = literalObject(t, "date", "2024-01-01")
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c, recorded := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool { return true })
			r := NewPartitionResource().(*PartitionResource)
			r.client = c

			plan := identityCreatePlan(t)
			if strings.HasPrefix(name, "range") {
				plan = rangeCreatePlan(t)
			}
			if strings.HasPrefix(name, "list") {
				plan = listCreatePlan(t)
			}
			mutate(t, &plan)

			resp := createPartition(t, r, plan)
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected an invalid partition spec to be rejected")
			}
			if len(*recorded) != 0 {
				t.Errorf("no request may be sent, got %v", *recorded)
			}
		})
	}
}

// TestPartitionResource_ReadRangePartition covers the state mapping of a range
// partition, including the non-nil bounds.
func TestPartitionResource_ReadRangePartition(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"partition": map[string]interface{}{
				"type":  "range",
				"name":  "r1",
				"upper": map[string]interface{}{"type": "literal", "dataType": "date", "value": "2024-01-01"},
				"lower": map[string]interface{}{"type": "literal", "dataType": "date", "value": "2023-01-01"},
				"properties": map[string]interface{}{
					"totalSize": "2",
				},
			},
		})
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	state := rangeCreatePlan(t)
	state.ID = types.StringValue("probe_ml.ptic_ice.ptsch.pt_part.r1")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Read(context.Background(), resource.ReadRequest{State: stateValue(t, schemaObj, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected read diagnostics: %v", resp.Diagnostics)
	}

	var refreshed PartitionResourceModel
	if diags := resp.State.Get(context.Background(), &refreshed); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if refreshed.Upper.IsNull() || refreshed.Lower.IsNull() {
		t.Fatalf("bounds must be refreshed, got upper=%v lower=%v", refreshed.Upper, refreshed.Lower)
	}
	if got := refreshed.Upper.Attributes()["value"].(types.String).ValueString(); got != "2024-01-01" {
		t.Errorf("upper value = %q", got)
	}
}

func TestPartitionResource_ReadServerError(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(t, w, map[string]interface{}{"code": 1100, "type": "InternalServerError", "message": "boom", "stack": []string{"..."}})
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	state := identityCreatePlan(t)
	state.ID = types.StringValue("probe_ml.ptic_ice.ptsch.pt_part.p1")
	state.Name = types.StringValue("p1")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Read(context.Background(), resource.ReadRequest{State: stateValue(t, schemaObj, state)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed reading partition "p1"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func TestPartitionResource_DeleteServerError(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(t, w, map[string]interface{}{"code": 1100, "type": "InternalServerError", "message": "boom", "stack": []string{"..."}})
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.SetClient(c)
	schemaObj := resourceSchema(t, r)

	state := identityCreatePlan(t)
	state.ID = types.StringValue("probe_ml.ptic_ice.ptsch.pt_part.p1")
	state.Name = types.StringValue("p1")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: schemaObj}}
	r.Delete(context.Background(), resource.DeleteRequest{State: stateValue(t, schemaObj, state)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed dropping partition "p1"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

// TestPartitionResource_CreateStoresServerPropertiesWhenUnmanaged asserts that a
// partition with no managed properties stores what the catalog reports.
func TestPartitionResource_CreateStoresServerPropertiesWhenUnmanaged(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"partitions": []interface{}{
				map[string]interface{}{"type": "identity", "name": "p1", "properties": map[string]interface{}{"totalSize": "2"}},
			},
		})
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.SetClient(c)

	resp := createPartition(t, r, identityCreatePlan(t))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %v", resp.Diagnostics)
	}

	var state PartitionResourceModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	stored := make(map[string]string)
	if diags := state.Properties.ElementsAs(context.Background(), &stored, false); diags.HasError() {
		t.Fatalf("reading properties: %v", diags)
	}
	if stored["totalSize"] != "2" {
		t.Errorf("properties = %#v, want the catalog properties", stored)
	}
}

// TestPartitionResource_CreateIgnoresUnmanagedPropertiesOfAManagedKey asserts a
// managed key keeps its configured value when the catalog reports a different
// one for it.
func TestPartitionResource_CreateKeepsConfiguredValueForMissingKey(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		writeJSON(t, w, map[string]interface{}{
			"code":       0,
			"partitions": []interface{}{map[string]interface{}{"type": "identity", "name": "p1", "properties": map[string]interface{}{"other": "x"}}},
		})
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.SetClient(c)

	plan := identityCreatePlan(t)
	properties, diags := types.MapValueFrom(context.Background(), types.StringType, map[string]string{"managed": "value"})
	if diags.HasError() {
		t.Fatalf("building properties: %v", diags)
	}
	plan.Properties = properties

	resp := createPartition(t, r, plan)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %v", resp.Diagnostics)
	}

	var state PartitionResourceModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	stored := make(map[string]string)
	if diags := state.Properties.ElementsAs(context.Background(), &stored, false); diags.HasError() {
		t.Fatalf("reading properties: %v", diags)
	}
	if len(stored) != 1 || stored["managed"] != "value" {
		t.Errorf("properties = %#v, want only the managed key with its configured value", stored)
	}
}

// TestPartitionResource_CreateWithEmptyManagedProperties asserts an explicitly
// empty properties map is treated as managed and stays empty.
func TestPartitionResource_CreateWithEmptyManagedProperties(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		writeJSON(t, w, map[string]interface{}{
			"code":       0,
			"partitions": []interface{}{map[string]interface{}{"type": "identity", "name": "p1", "properties": map[string]interface{}{"totalSize": "2"}}},
		})
		return true
	})

	r := NewPartitionResource().(*PartitionResource)
	r.SetClient(c)

	plan := identityCreatePlan(t)
	properties, diags := types.MapValueFrom(context.Background(), types.StringType, map[string]string{})
	if diags.HasError() {
		t.Fatalf("building properties: %v", diags)
	}
	plan.Properties = properties

	resp := createPartition(t, r, plan)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %v", resp.Diagnostics)
	}

	var state PartitionResourceModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if state.Properties.IsNull() {
		t.Fatal("properties = null, want the empty managed map")
	}
	if len(state.Properties.Elements()) != 0 {
		t.Errorf("properties = %v, want the empty managed map", state.Properties.Elements())
	}
}
