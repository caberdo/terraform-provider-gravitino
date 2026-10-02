package statistics_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/statistics"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestStatisticsDataSource_Schema(t *testing.T) {
	d := ds.New()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_statistics" {
		t.Fatalf("expected gravitino_statistics, got %s", resp.TypeName)
	}
}

func TestStatisticsDataSource_Read(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/api/metalakes/test_metalake/objects/CATALOG/test_catalog/statistics"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}

		resp := models.StatisticsResponse{
			Code: 0,
			Statistics: []models.Statistics{
				{
					Name:       "numRows",
					Type:       "long",
					Value:      "1000",
					Properties: map[string]string{"unit": "rows"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.New()
	d.(*ds.StatisticsDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	statItemObjType := types.ObjectType{AttrTypes: ds.StatisticItemAttrTypes}
	statsListType := types.ListType{ElemType: statItemObjType}

	attrTypes := map[string]attr.Type{
		"metalake":      types.StringType,
		"resource_type": types.StringType,
		"resource":      types.StringType,
		"statistics":    statsListType,
	}

	configModel := ds.StatisticsDataSourceModel{
		Metalake:     types.StringValue("test_metalake"),
		ResourceType: types.StringValue("CATALOG"),
		Resource:     types.StringValue("test_catalog"),
		Statistics:   types.ListNull(statItemObjType),
	}

	configObj, diags := types.ObjectValueFrom(ctx, attrTypes, configModel)
	if diags.HasError() {
		t.Fatalf("failed to create config object: %v", diags)
	}

	tfVal, err := configObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}

	req := datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaObj, Raw: tfVal},
	}
	resp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaObj},
	}

	d.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}
}

func TestPartitionStatisticsDataSource_Schema(t *testing.T) {
	d := ds.NewPartition()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_partition_statistics" {
		t.Fatalf("expected gravitino_partition_statistics, got %s", resp.TypeName)
	}
}

var statisticsAttrTypes = map[string]attr.Type{
	"metalake":      types.StringType,
	"resource_type": types.StringType,
	"resource":      types.StringType,
	"statistics":    types.ListType{ElemType: types.ObjectType{AttrTypes: ds.StatisticItemAttrTypes}},
}

// readStatistics runs Read with the given config and returns the resulting state model.
func readStatistics(t *testing.T, d datasource.DataSource, model ds.StatisticsDataSourceModel) (*datasource.ReadResponse, ds.StatisticsDataSourceModel) {
	t.Helper()

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	schemaObj := schemaResp.Schema

	model.Statistics = types.ListNull(types.ObjectType{AttrTypes: ds.StatisticItemAttrTypes})
	configObj, diags := types.ObjectValueFrom(ctx, statisticsAttrTypes, model)
	if diags.HasError() {
		t.Fatalf("failed to create config object: %v", diags)
	}
	tfVal, err := configObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: tfVal}}, resp)

	var state ds.StatisticsDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			t.Fatalf("failed to read state: %v", resp.Diagnostics)
		}
	}
	return resp, state
}

// TestStatisticsDataSource_ResourceTypeEnum pins the resource_type validator to
// the v1.3.1 metadataObjectType enum: VIEW and FUNCTION are accepted (the union
// of both supported server versions), TAG/POLICY are not.
func TestStatisticsDataSource_ResourceTypeEnum(t *testing.T) {
	schemaResp := &datasource.SchemaResponse{}
	ds.New().Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}

	resourceType, ok := schemaResp.Schema.Attributes["resource_type"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("resource_type must be a StringAttribute, got %T", schemaResp.Schema.Attributes["resource_type"])
	}
	validators := resourceType.StringValidators()
	if len(validators) == 0 {
		t.Fatal("resource_type must have an enum validator")
	}

	ctx := context.Background()
	for _, valid := range []string{"METALAKE", "CATALOG", "SCHEMA", "TABLE", "VIEW", "COLUMN", "FILESET", "TOPIC", "MODEL", "FUNCTION", "ROLE"} {
		vResp := &validator.StringResponse{}
		validators[0].ValidateString(ctx, validator.StringRequest{
			ConfigValue: types.StringValue(valid),
			Path:        path.Root("resource_type"),
		}, vResp)
		if vResp.Diagnostics.HasError() {
			t.Errorf("resource_type %q must be accepted: %v", valid, vResp.Diagnostics)
		}
	}
	for _, invalid := range []string{"TAG", "POLICY", "view"} {
		vResp := &validator.StringResponse{}
		validators[0].ValidateString(ctx, validator.StringRequest{
			ConfigValue: types.StringValue(invalid),
			Path:        path.Root("resource_type"),
		}, vResp)
		if !vResp.Diagnostics.HasError() {
			t.Errorf("resource_type %q must be rejected", invalid)
		}
	}
}

// TestStatisticsDataSource_VersionRestrictedObjectTypes proves that VIEW and
// FUNCTION (added to the metadataObjectType parameter by Gravitino 1.3.1) are
// rejected with an explicit diagnostic on an older server and read normally
// from 1.3.1 on. The statistics endpoint must not be called when the gate
// fails.
func TestStatisticsDataSource_VersionRestrictedObjectTypes(t *testing.T) {
	for _, resourceType := range []string{"VIEW", "FUNCTION"} {
		for _, tc := range []struct {
			name    string
			version string
			wantErr bool
		}{
			{"rejected on 1.3.0", "1.3.0", true},
			{"accepted on 1.3.1", "1.3.1", false},
		} {
			t.Run(resourceType+"/"+tc.name, func(t *testing.T) {
				var paths []string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					paths = append(paths, r.URL.Path)
					w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
					if r.URL.Path == "/api/version" {
						_, _ = fmt.Fprintf(w, `{"code":0,"version":{"version":%q,"compileDate":"","gitCommit":""}}`, tc.version)
						return
					}
					_, _ = w.Write([]byte(`{"code":0,"statistics":[{"name":"numRows","type":"long","value":"42","properties":{}}]}`))
				}))
				defer server.Close()

				c, _ := client.New(server.URL, nil)
				d := ds.New()
				d.(*ds.StatisticsDataSource).SetClient(c)

				resp, state := readStatistics(t, d, ds.StatisticsDataSourceModel{
					Metalake:     types.StringValue("test_metalake"),
					ResourceType: types.StringValue(resourceType),
					Resource:     types.StringValue("test_catalog.test_schema.object"),
				})

				if tc.wantErr {
					if !resp.Diagnostics.HasError() {
						t.Fatal("expected an error diagnostic on a 1.3.0 server")
					}
					for _, diag := range resp.Diagnostics.Errors() {
						t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
					}
					if !resp.State.Raw.IsNull() {
						t.Error("state must not be written when the version check failed")
					}

					var detail strings.Builder
					for _, diag := range resp.Diagnostics.Errors() {
						detail.WriteString(diag.Detail())
						detail.WriteString("\n")
					}
					for _, want := range []string{resourceType, "Gravitino 1.3.1", tc.version} {
						if !strings.Contains(detail.String(), want) {
							t.Errorf("diagnostics must mention %q, got:\n%s", want, detail.String())
						}
					}
					if len(paths) != 1 || paths[0] != "/api/version" {
						t.Errorf("the statistics endpoint must not be called when the gate rejects the server, got %v", paths)
					}
					return
				}

				if resp.Diagnostics.HasError() {
					for _, diag := range resp.Diagnostics.Errors() {
						t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
					}
					t.Fatal("unexpected diagnostics errors")
				}
				wantPath := "/api/metalakes/test_metalake/objects/" + resourceType + "/test_catalog.test_schema.object/statistics"
				if len(paths) != 2 || paths[0] != "/api/version" || paths[1] != wantPath {
					t.Fatalf("expected [/api/version %s], got %v", wantPath, paths)
				}
				if n := len(state.Statistics.Elements()); n != 1 {
					t.Fatalf("expected 1 statistic, got %d", n)
				}
			})
		}
	}
}

func TestPartitionStatisticsDataSource_Read(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/api/metalakes/test_metalake/objects/TABLE/test_catalog.test_schema.test_table/statistics/partitions"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}

		resp := models.PartitionStatisticsResponse{
			Code: 0,
			Statistics: []models.PartitionStatistics{
				{
					PartitionName: "dt=2024-01-01",
					Statistics: []models.Statistics{
						{
							Name:       "numRows",
							Type:       "long",
							Value:      "500",
							Properties: map[string]string{"unit": "rows"},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewPartition()
	d.(*ds.PartitionStatisticsDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	psItemObjType := types.ObjectType{AttrTypes: ds.PartitionStatisticItemAttrTypes}
	psListType := types.ListType{ElemType: psItemObjType}

	attrTypes := map[string]attr.Type{
		"metalake":             types.StringType,
		"catalog":              types.StringType,
		"schema":               types.StringType,
		"table":                types.StringType,
		"partition_statistics": psListType,
	}

	configModel := ds.PartitionStatisticsDataSourceModel{
		Metalake:            types.StringValue("test_metalake"),
		Catalog:             types.StringValue("test_catalog"),
		Schema:              types.StringValue("test_schema"),
		Table:               types.StringValue("test_table"),
		PartitionStatistics: types.ListNull(psItemObjType),
	}

	configObj, diags := types.ObjectValueFrom(ctx, attrTypes, configModel)
	if diags.HasError() {
		t.Fatalf("failed to create config object: %v", diags)
	}

	tfVal, err := configObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}

	req := datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaObj, Raw: tfVal},
	}
	resp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaObj},
	}

	d.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}
}
