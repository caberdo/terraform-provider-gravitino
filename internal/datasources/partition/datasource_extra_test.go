package partition

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPartitionDataSources_Configure(t *testing.T) {
	constructors := map[string]func() datasource.DataSource{
		"gravitino_partition":  NewPartitionDataSource,
		"gravitino_partitions": NewPartitionsDataSource,
	}

	for name, constructor := range constructors {
		t.Run(name, func(t *testing.T) {
			d := constructor()
			configured := d.(datasource.DataSourceWithConfigure)

			resp := &datasource.ConfigureResponse{}
			configured.Configure(context.Background(), datasource.ConfigureRequest{}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("nil provider data must configure without diagnostics: %v", resp.Diagnostics)
			}

			resp = &datasource.ConfigureResponse{}
			configured.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected a diagnostic for unexpected provider data")
			}

			c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool { return false })
			resp = &datasource.ConfigureResponse{}
			configured.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: c}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("a *client.Client must configure: %v", resp.Diagnostics)
			}
		})
	}
}

func partitionGetConfig() PartitionDataSourceModel {
	return PartitionDataSourceModel{
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
}

func readPartitionGet(t *testing.T, d *PartitionDataSource, config PartitionDataSourceModel) *datasource.ReadResponse {
	t.Helper()
	ctx := context.Background()
	s := partitionSchema(t, d).Schema
	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), config)
	if diags.HasError() {
		t.Fatalf("building config: %v", diags)
	}
	value, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("converting config: %v", err)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: value}}, resp)
	return resp
}

// TestPartitionDataSource_ReadRangePartition covers the range bounds and the
// empty properties of a range partition.
func TestPartitionDataSource_ReadRangePartition(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method != http.MethodGet || req.Path != testPath+"/p1" {
			return false
		}
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"partition": map[string]interface{}{
				"type":       "range",
				"name":       "r1",
				"upper":      map[string]interface{}{"type": "literal", "dataType": "date", "value": "2024-01-01"},
				"lower":      map[string]interface{}{"type": "literal", "dataType": "date", "value": "2023-01-01"},
				"properties": map[string]interface{}{},
			},
		})
		return true
	})

	d := NewPartitionDataSource().(*PartitionDataSource)
	d.SetClient(c)

	resp := readPartitionGet(t, d, partitionGetConfig())
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected read diagnostics: %v", resp.Diagnostics)
	}

	ctx := context.Background()
	var state PartitionDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if state.Upper.IsNull() || state.Lower.IsNull() {
		t.Fatalf("range bounds must be reported, got upper=%v lower=%v", state.Upper, state.Lower)
	}
	if got := state.Upper.Attributes()["value"].(types.String).ValueString(); got != "2024-01-01" {
		t.Errorf("upper value = %q", got)
	}
	if !state.Properties.IsNull() {
		t.Errorf("properties = %v, want null", state.Properties)
	}
	if !state.Lists.IsNull() {
		t.Errorf("lists = %v, want null for a range partition", state.Lists)
	}
}

// TestPartitionDataSource_ReadListPartition covers the value lists of a list
// partition.
func TestPartitionDataSource_ReadListPartition(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.Method != http.MethodGet || req.Path != testPath+"/p1" {
			return false
		}
		writeJSON(t, w, map[string]interface{}{
			"code": 0,
			"partition": map[string]interface{}{
				"type": "list",
				"name": "l1",
				"lists": []interface{}{
					[]interface{}{map[string]interface{}{"type": "literal", "dataType": "string", "value": "a"}},
				},
				"properties": map[string]interface{}{"totalSize": "2"},
			},
		})
		return true
	})

	d := NewPartitionDataSource().(*PartitionDataSource)
	d.SetClient(c)

	resp := readPartitionGet(t, d, partitionGetConfig())
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected read diagnostics: %v", resp.Diagnostics)
	}

	ctx := context.Background()
	var state PartitionDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading state: %v", diags)
	}
	if state.Lists.IsNull() || len(state.Lists.Elements()) != 1 {
		t.Fatalf("lists = %v", state.Lists)
	}
	if !state.Upper.IsNull() || !state.Lower.IsNull() {
		t.Fatalf("bound attributes must stay null for a list partition: upper=%v lower=%v", state.Upper, state.Lower)
	}
}

func TestPartitionDataSource_ReadServerError(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(t, w, map[string]interface{}{"code": 1100, "type": "InternalServerError", "message": "boom", "stack": []string{"..."}})
		return true
	})

	d := NewPartitionDataSource().(*PartitionDataSource)
	d.SetClient(c)

	resp := readPartitionGet(t, d, partitionGetConfig())
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed reading partition "p1"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func partitionsListConfig() PartitionsDataSourceModel {
	return PartitionsDataSourceModel{
		Metalake:   types.StringValue(testMetalake),
		Catalog:    types.StringValue(testCatalog),
		Schema:     types.StringValue(testSchema),
		Table:      types.StringValue(testTable),
		Partitions: types.ListNull(types.ObjectType{AttrTypes: models.PartitionSpecAttrTypes}),
	}
}

func readPartitionsList(t *testing.T, d *PartitionsDataSource) (*datasource.ReadResponse, PartitionsDataSourceModel) {
	t.Helper()
	ctx := context.Background()
	s := partitionSchema(t, d).Schema
	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), partitionsListConfig())
	if diags.HasError() {
		t.Fatalf("building config: %v", diags)
	}
	value, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("converting config: %v", err)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: value}}, resp)

	var state PartitionsDataSourceModel
	if !resp.Diagnostics.HasError() {
		if diags := resp.State.Get(ctx, &state); diags.HasError() {
			t.Fatalf("reading state: %v", diags)
		}
	}
	return resp, state
}

// TestPartitionsDataSource_ReadEmpty asserts an empty partition list is not
// reported as a null list.
func TestPartitionsDataSource_ReadEmpty(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		if req.RawQuery != "details=true" {
			return false
		}
		writeJSON(t, w, map[string]interface{}{"code": 0, "partitions": []interface{}{}})
		return true
	})

	d := NewPartitionsDataSource().(*PartitionsDataSource)
	d.SetClient(c)

	resp, state := readPartitionsList(t, d)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected read diagnostics: %v", resp.Diagnostics)
	}
	if state.Partitions.IsNull() {
		t.Fatal("an empty result must not be a null list")
	}
	if len(state.Partitions.Elements()) != 0 {
		t.Fatalf("expected no partitions, got %d", len(state.Partitions.Elements()))
	}
}

func TestPartitionsDataSource_ReadServerError(t *testing.T) {
	c, _ := newMockServer(t, func(req recordedRequest, w http.ResponseWriter) bool {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(t, w, map[string]interface{}{"code": 1100, "type": "InternalServerError", "message": "boom", "stack": []string{"..."}})
		return true
	})

	d := NewPartitionsDataSource().(*PartitionsDataSource)
	d.SetClient(c)

	resp, _ := readPartitionsList(t, d)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed listing partitions "pt_part"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}
