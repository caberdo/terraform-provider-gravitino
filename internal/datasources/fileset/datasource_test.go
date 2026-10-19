package fileset_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/fileset"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestFilesetsDataSource_Schema(t *testing.T) {
	d := ds.NewListDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_filesets" {
		t.Fatalf("expected gravitino_filesets, got %s", resp.TypeName)
	}
}

func TestFilesetsDataSource_Read(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/metalakes/test_metalake/catalogs/test_catalog/schemas/test_schema/filesets" {
			resp := models.IdentifiersResponse{
				Code: 0,
				Identifiers: []models.NameIdentifier{
					{Namespace: []string{"test_metalake", "test_catalog", "test_schema"}, Name: "fileset1"},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/api/metalakes/test_metalake/catalogs/test_catalog/schemas/test_schema/filesets/fileset1" {
			resp := models.FilesetResponse{
				Code: 0,
				Fileset: models.Fileset{
					Name:       "fileset1",
					Type:       "managed",
					Comment:    "test fileset",
					Properties: map[string]string{"key": "value"},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewListDataSource()
	d.(*ds.FilesetsDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	fsItemObjType := types.ObjectType{AttrTypes: ds.FilesetItemAttrTypes}
	filesetsListType := types.ListType{ElemType: fsItemObjType}

	configModel := ds.FilesetsDataSourceModel{
		Metalake: types.StringValue("test_metalake"),
		Catalog:  types.StringValue("test_catalog"),
		Schema:   types.StringValue("test_schema"),
		Filesets: types.ListNull(fsItemObjType),
	}

	attrTypes := map[string]attr.Type{
		"metalake": types.StringType,
		"catalog":  types.StringType,
		"schema":   types.StringType,
		"filesets": filesetsListType,
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

func TestFilesetDataSource_Schema(t *testing.T) {
	d := ds.NewGetDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_fileset" {
		t.Fatalf("expected gravitino_fileset, got %s", resp.TypeName)
	}
}

func TestFilesetDataSource_Read(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/api/metalakes/test_metalake/catalogs/test_catalog/schemas/test_schema/filesets/test_fileset"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}

		resp := models.FilesetResponse{
			Code: 0,
			Fileset: models.Fileset{
				Name:            "test_fileset",
				Type:            "managed",
				Comment:         "a test fileset",
				StorageLocation: "s3://bucket/path",
				Properties:      map[string]string{"env": "test"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewGetDataSource()
	d.(*ds.FilesetDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	attrTypes := map[string]attr.Type{
		"metalake":         types.StringType,
		"catalog":          types.StringType,
		"schema":           types.StringType,
		"name":             types.StringType,
		"comment":          types.StringType,
		"type":             types.StringType,
		"storage_location": types.StringType,
		"properties":       types.MapType{ElemType: types.StringType},
		"audit":            types.ObjectType{AttrTypes: ds.AuditAttrTypes},
	}

	configModel := ds.FilesetDataSourceModel{
		Metalake:   types.StringValue("test_metalake"),
		Catalog:    types.StringValue("test_catalog"),
		Schema:     types.StringValue("test_schema"),
		Name:       types.StringValue("test_fileset"),
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

func configureFilesetDataSource(t *testing.T, d datasource.DataSource, c *client.Client) {
	t.Helper()
	configured := d.(datasource.DataSourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	configured.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("configure diagnostics: %v", resp.Diagnostics)
	}
}

func newFilesetClient(t *testing.T, handler http.HandlerFunc) *client.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	return c
}

func filesetGetConfig(s schema.Schema) ds.FilesetDataSourceModel {
	return ds.FilesetDataSourceModel{
		Metalake:        types.StringValue("test_metalake"),
		Catalog:         types.StringValue("test_catalog"),
		Schema:          types.StringValue("test_schema"),
		Name:            types.StringValue("test_fileset"),
		Comment:         types.StringNull(),
		Type:            types.StringNull(),
		StorageLocation: types.StringNull(),
		Properties:      types.MapNull(types.StringType),
		Audit:           types.ObjectNull(ds.AuditAttrTypes),
	}
}

func readFilesetGet(t *testing.T, d datasource.DataSource, c *client.Client, config ds.FilesetDataSourceModel) *datasource.ReadResponse {
	t.Helper()
	configureFilesetDataSource(t, d, c)

	ctx := context.Background()
	s := filesetSchema(t, d)
	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), config)
	if diags.HasError() {
		t.Fatalf("failed to build config object: %v", diags)
	}
	raw, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert config: %v", err)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: raw}}, resp)
	return resp
}

func filesetSchema(t *testing.T, d datasource.DataSource) schema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestFilesetDataSources_Configure(t *testing.T) {
	constructors := map[string]func() datasource.DataSource{
		"gravitino_fileset":  ds.NewGetDataSource,
		"gravitino_filesets": ds.NewListDataSource,
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
		})
	}
}

func TestFilesetDataSource_ReadNotFound(t *testing.T) {
	c := newFilesetClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 1003, "type": "NoSuchFilesetException", "message": "Fileset does not exist", "stack": []string{"..."}})
	})

	d := ds.NewGetDataSource()
	resp := readFilesetGet(t, d, c, filesetGetConfig(filesetSchema(t, d)))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a missing fileset")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Fileset not found" {
		t.Fatalf("unexpected summary: %s", got)
	}
}

func TestFilesetDataSource_ReadServerError(t *testing.T) {
	c := newFilesetClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 1100, "type": "InternalServerError", "message": "boom", "stack": []string{"..."}})
	})

	d := ds.NewGetDataSource()
	resp := readFilesetGet(t, d, c, filesetGetConfig(filesetSchema(t, d)))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed reading fileset "test_fileset"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func filesetListConfig() ds.FilesetsDataSourceModel {
	return ds.FilesetsDataSourceModel{
		Metalake: types.StringValue("test_metalake"),
		Catalog:  types.StringValue("test_catalog"),
		Schema:   types.StringValue("test_schema"),
		Filesets: types.ListNull(types.ObjectType{AttrTypes: ds.FilesetItemAttrTypes}),
	}
}

func readFilesetList(t *testing.T, d datasource.DataSource, c *client.Client) *datasource.ReadResponse {
	t.Helper()
	configureFilesetDataSource(t, d, c)

	ctx := context.Background()
	s := filesetSchema(t, d)
	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), filesetListConfig())
	if diags.HasError() {
		t.Fatalf("failed to build config object: %v", diags)
	}
	raw, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert config: %v", err)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: raw}}, resp)
	return resp
}

// TestFilesetsDataSource_ReadDecodesDetails asserts the per-fileset detail
// requests are decoded into the nested list.
func TestFilesetsDataSource_ReadDecodesDetails(t *testing.T) {
	c := newFilesetClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case "/api/metalakes/test_metalake/catalogs/test_catalog/schemas/test_schema/filesets":
			// FilesetListResponse example of filesets.yaml.
			json.NewEncoder(w).Encode(models.IdentifiersResponse{Code: 0, Identifiers: []models.NameIdentifier{
				{Namespace: []string{"test_metalake", "test_catalog", "test_schema"}, Name: "fileset1"},
			}})
		case "/api/metalakes/test_metalake/catalogs/test_catalog/schemas/test_schema/filesets/fileset1":
			// FilesetResponse example of filesets.yaml.
			json.NewEncoder(w).Encode(models.FilesetResponse{Code: 0, Fileset: models.Fileset{
				Name:            "fileset1",
				Type:            "managed",
				Comment:         "This is a comment",
				StorageLocation: "hdfs://host/user/s_fileset/schema/fileset1",
				Properties:      map[string]string{"key1": "value1", "key2": "value2"},
				Audit:           &models.Audit{Creator: "gravitino"},
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	d := ds.NewListDataSource()
	resp := readFilesetList(t, d, c)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	ctx := context.Background()
	var state ds.FilesetsDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	elements := state.Filesets.Elements()
	if len(elements) != 1 {
		t.Fatalf("expected one fileset, got %d", len(elements))
	}
	attrs := elements[0].(types.Object).Attributes()
	if got := attrs["type"].(types.String).ValueString(); got != "managed" {
		t.Errorf("type = %q", got)
	}
	if got := attrs["storage_location"].(types.String).ValueString(); got != "hdfs://host/user/s_fileset/schema/fileset1" {
		t.Errorf("storage_location = %q", got)
	}
	if got := attrs["properties"].(types.Map).Elements()["key1"]; !got.Equal(types.StringValue("value1")) {
		t.Errorf("properties[key1] = %v", got)
	}
	if attrs["audit"].(types.Object).IsNull() {
		t.Error("audit must be set from the response")
	}
}

// TestFilesetsDataSource_ReadEmpty asserts an empty identifier list becomes an
// empty list, not a null one.
func TestFilesetsDataSource_ReadEmpty(t *testing.T) {
	c := newFilesetClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		json.NewEncoder(w).Encode(models.IdentifiersResponse{Code: 0, Identifiers: []models.NameIdentifier{}})
	})

	d := ds.NewListDataSource()
	resp := readFilesetList(t, d, c)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	ctx := context.Background()
	var state ds.FilesetsDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.Filesets.IsNull() {
		t.Fatal("an empty result must not be a null list")
	}
	if len(state.Filesets.Elements()) != 0 {
		t.Fatalf("expected no filesets, got %d", len(state.Filesets.Elements()))
	}
}

func TestFilesetsDataSource_ReadNotFound(t *testing.T) {
	c := newFilesetClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 1003, "type": "NoSuchMetalakeException", "message": "Metalake does not exist", "stack": []string{"..."}})
	})

	d := ds.NewListDataSource()
	resp := readFilesetList(t, d, c)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the metalake, catalog or schema is missing")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Metalake, catalog or schema not found" {
		t.Fatalf("unexpected summary: %s", got)
	}
}

func TestFilesetsDataSource_ReadServerError(t *testing.T) {
	c := newFilesetClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 1100, "type": "InternalServerError", "message": "boom", "stack": []string{"..."}})
	})

	d := ds.NewListDataSource()
	resp := readFilesetList(t, d, c)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed listing filesets "test_schema"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}
