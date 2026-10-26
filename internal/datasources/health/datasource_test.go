package health_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/health"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/models/modeltest"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func healthCheckAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":    types.StringType,
		"status":  types.StringType,
		"details": types.MapType{ElemType: types.StringType},
	}
}

func TestHealthDataSource_ModelMatchesSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	ds.NewHealthDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	modeltest.AssertSchemaMatches(t, ds.HealthDataSourceModel{}, resp.Schema.Attributes, resp.Schema.Blocks)
	assertHealthCheckModelMatchesSchema(t, resp.Schema)
}

func TestLivenessDataSource_ModelMatchesSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	ds.NewLivenessDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	modeltest.AssertSchemaMatches(t, ds.LivenessDataSourceModel{}, resp.Schema.Attributes, resp.Schema.Blocks)
	assertHealthCheckModelMatchesSchema(t, resp.Schema)
}

func TestReadinessDataSource_ModelMatchesSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	ds.NewReadinessDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	modeltest.AssertSchemaMatches(t, ds.ReadinessDataSourceModel{}, resp.Schema.Attributes, resp.Schema.Blocks)
	assertHealthCheckModelMatchesSchema(t, resp.Schema)
}

// assertHealthCheckModelMatchesSchema guards the checks block entry model shared
// by the health, liveness and readiness data sources.
func assertHealthCheckModelMatchesSchema(t *testing.T, s schema.Schema) {
	t.Helper()
	checks, ok := s.Blocks["checks"].(schema.ListNestedBlock)
	if !ok {
		t.Fatalf("checks block = %T, want schema.ListNestedBlock", s.Blocks["checks"])
	}
	modeltest.AssertAttributesMatch(t, ds.HealthCheckModel{}, modeltest.Keys(checks.NestedObject.Attributes))
}

func setupHealthServer(t *testing.T, expectedPath string, status string, checks []models.HealthCheck) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		resp := models.HealthResponse{
			Code:   0,
			Status: status,
			Checks: checks,
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestHealthDataSource_Schema(t *testing.T) {
	d := ds.NewHealthDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_health" {
		t.Fatalf("expected gravitino_health, got %s", resp.TypeName)
	}
}

func TestHealthDataSource_Read(t *testing.T) {
	checks := []models.HealthCheck{
		{
			Name:    "database",
			Status:  "healthy",
			Details: map[string]string{"latency": "5ms"},
		},
		{
			Name:   "disk",
			Status: "healthy",
		},
	}

	server := setupHealthServer(t, "/api/health", "healthy", checks)
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewHealthDataSource()
	d.(*ds.HealthDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	checkObjType := types.ObjectType{AttrTypes: healthCheckAttrTypes()}
	checksListType := types.ListType{ElemType: checkObjType}

	attrTypes := map[string]attr.Type{
		"status": types.StringType,
		"checks": checksListType,
	}

	configModel := ds.HealthDataSourceModel{
		Status: types.StringNull(),
		Checks: nil,
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

func TestLivenessDataSource_Schema(t *testing.T) {
	d := ds.NewLivenessDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_liveness" {
		t.Fatalf("expected gravitino_liveness, got %s", resp.TypeName)
	}
}

func TestLivenessDataSource_Read(t *testing.T) {
	checks := []models.HealthCheck{
		{
			Name:   "liveness",
			Status: "alive",
		},
	}

	server := setupHealthServer(t, "/api/health/live", "alive", checks)
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewLivenessDataSource()
	d.(*ds.LivenessDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	checkObjType := types.ObjectType{AttrTypes: healthCheckAttrTypes()}
	checksListType := types.ListType{ElemType: checkObjType}

	attrTypes := map[string]attr.Type{
		"status": types.StringType,
		"checks": checksListType,
	}

	configModel := ds.LivenessDataSourceModel{
		Status: types.StringNull(),
		Checks: nil,
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

func TestReadinessDataSource_Schema(t *testing.T) {
	d := ds.NewReadinessDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_readiness" {
		t.Fatalf("expected gravitino_readiness, got %s", resp.TypeName)
	}
}

func TestReadinessDataSource_Read(t *testing.T) {
	checks := []models.HealthCheck{
		{
			Name:   "database",
			Status: "ready",
			Details: map[string]string{
				"host": "localhost",
				"port": "5432",
			},
		},
		{
			Name:   "cache",
			Status: "ready",
		},
	}

	server := setupHealthServer(t, "/api/health/ready", "ready", checks)
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewReadinessDataSource()
	d.(*ds.ReadinessDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	checkObjType := types.ObjectType{AttrTypes: healthCheckAttrTypes()}
	checksListType := types.ListType{ElemType: checkObjType}

	attrTypes := map[string]attr.Type{
		"status": types.StringType,
		"checks": checksListType,
	}

	configModel := ds.ReadinessDataSourceModel{
		Status: types.StringNull(),
		Checks: nil,
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
