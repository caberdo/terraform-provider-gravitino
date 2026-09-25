package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/catalog"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCatalogsDataSource_Schema(t *testing.T) {
	d := ds.NewListDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_catalogs" {
		t.Fatalf("expected gravitino_catalogs, got %s", resp.TypeName)
	}
}

func TestCatalogsDataSource_Read(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/api/metalakes/test_metalake/catalogs"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		if r.URL.Query().Get("details") != "true" {
			t.Errorf("expected details=true, got %s", r.URL.Query().Get("details"))
		}

		resp := models.CatalogInfoListResponse{
			Code: 0,
			Catalogs: []models.Catalog{
				{
					Name:       "catalog1",
					Type:       "relational",
					Provider:   "hive",
					Comment:    "test catalog",
					Properties: map[string]string{"key": "value"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewListDataSource()
	d.(*ds.CatalogsDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	catItemObjType := types.ObjectType{AttrTypes: ds.CatalogItemAttrTypes}
	catalogsListType := types.ListType{ElemType: catItemObjType}

	configModel := ds.CatalogsDataSourceModel{
		Metalake: types.StringValue("test_metalake"),
		Catalogs: types.ListNull(catItemObjType),
	}

	attrTypes := map[string]attr.Type{
		"metalake": types.StringType,
		"catalogs": catalogsListType,
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

func TestCatalogDataSource_Schema(t *testing.T) {
	d := ds.NewGetDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_catalog" {
		t.Fatalf("expected gravitino_catalog, got %s", resp.TypeName)
	}
}

func TestCatalogDataSource_Read(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/api/metalakes/test_metalake/catalogs/test_catalog"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}

		resp := models.CatalogResponse{
			Code: 0,
			Catalog: models.Catalog{
				Name:       "test_catalog",
				Type:       "relational",
				Provider:   "hive",
				Comment:    "a test catalog",
				Properties: map[string]string{"env": "test"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewGetDataSource()
	d.(*ds.CatalogDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	attrTypes := map[string]attr.Type{
		"metalake":         types.StringType,
		"name":             types.StringType,
		"type":             types.StringType,
		"catalog_provider": types.StringType,
		"comment":          types.StringType,
		"properties":       types.MapType{ElemType: types.StringType},
		"audit":            types.ObjectType{AttrTypes: ds.AuditAttrTypes},
	}

	configModel := ds.CatalogDataSourceModel{
		Metalake:   types.StringValue("test_metalake"),
		Name:       types.StringValue("test_catalog"),
		Audit:      types.ObjectNull(ds.AuditAttrTypes),
		Properties: types.MapNull(types.StringType),
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

func TestCatalogDataSource_ReadNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		// Verbatim NoSuchCatalogException example of catalogs.yaml.
		fmt.Fprint(w, `{"code":1003,"type":"NoSuchCatalogException","message":"Failed to operate catalog(s) [test] operation [LOAD] under metalake [my_test_metalake], reason [NoSuchCatalogException]","stack":["org.apache.gravitino.exceptions.NoSuchCatalogException: Catalog my_test_metalake.test does not exist","..."]}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewGetDataSource()
	d.(*ds.CatalogDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)

	configObj, _ := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"metalake":         types.StringType,
		"name":             types.StringType,
		"type":             types.StringType,
		"catalog_provider": types.StringType,
		"comment":          types.StringType,
		"properties":       types.MapType{ElemType: types.StringType},
		"audit":            types.ObjectType{AttrTypes: ds.AuditAttrTypes},
	}, ds.CatalogDataSourceModel{
		Metalake:   types.StringValue("my_test_metalake"),
		Name:       types.StringValue("missing"),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(ds.AuditAttrTypes),
	})
	raw, _ := configObj.ToTerraformValue(ctx)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a missing catalog")
	}
	if summary := resp.Diagnostics.Errors()[0].Summary(); summary != "Catalog not found" {
		t.Fatalf("unexpected summary: %s", summary)
	}
}

// TestCatalogDataSource_ReadExposesServerProperties: a data source mirrors the
// API, including server-owned keys such as "in-use".
func TestCatalogDataSource_ReadExposesServerProperties(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{"code":0,"catalog":{"name":"my_hive_catalog","type":"relational","provider":"hive","comment":"c","properties":{"in-use":"true","key1":"value1"}}}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewGetDataSource()
	d.(*ds.CatalogDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)

	configObj, _ := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"metalake":         types.StringType,
		"name":             types.StringType,
		"type":             types.StringType,
		"catalog_provider": types.StringType,
		"comment":          types.StringType,
		"properties":       types.MapType{ElemType: types.StringType},
		"audit":            types.ObjectType{AttrTypes: ds.AuditAttrTypes},
	}, ds.CatalogDataSourceModel{
		Metalake:   types.StringValue("my_test_metalake"),
		Name:       types.StringValue("my_hive_catalog"),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(ds.AuditAttrTypes),
	})
	raw, _ := configObj.ToTerraformValue(ctx)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state ds.CatalogDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if got := state.Properties.Elements()["in-use"].(types.String).ValueString(); got != "true" {
		t.Fatalf("expected server properties to be exposed, got %v", state.Properties)
	}
}

func TestCatalogsDataSource_ReadServerErrorUsesGravitinoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalError","message":"boom","stack":["..."]}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewListDataSource()
	d.(*ds.CatalogsDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)

	catItemObjType := types.ObjectType{AttrTypes: ds.CatalogItemAttrTypes}
	configObj, _ := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"metalake": types.StringType,
		"catalogs": types.ListType{ElemType: catItemObjType},
	}, ds.CatalogsDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Catalogs: types.ListNull(catItemObjType),
	})
	raw, _ := configObj.ToTerraformValue(ctx)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != `Failed listing catalogs "my_test_metalake"` {
		t.Fatalf("unexpected summary: %s", got)
	}
}
