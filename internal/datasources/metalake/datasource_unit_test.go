package metalake_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/metalake"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// configureDataSource wires the client through the public Configure hook, as
// the provider does.
func configureDataSource(t *testing.T, ctx context.Context, d datasource.DataSource, c *client.Client) {
	t.Helper()
	cfg, ok := d.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatalf("%T does not accept provider data", d)
	}
	resp := &datasource.ConfigureResponse{}
	cfg.Configure(ctx, datasource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed to configure data source: %v", resp.Diagnostics)
	}
}

func TestMetalakeDataSource_Read(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.URL.Path != "/api/metalakes/my_metalake" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		// Verbatim MetalakeResponse example of metalakes.yaml.
		fmt.Fprint(w, `{"code":0,"metalake":{"name":"my_metalake","comment":"This is my metalake","properties":{"key1":"value1","key2":"value2","gravitino.identifier":"gravitino.v1.uid2062071866014250017"},"audit":{"creator":"gravitino","createTime":"2023-12-06T14:21:24.982Z"}}}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewMetalakeDataSource()
	configureDataSource(t, ctx, d, c)

	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	attrTypes := map[string]attr.Type{
		"name":       types.StringType,
		"comment":    types.StringType,
		"properties": types.MapType{ElemType: types.StringType},
		"audit":      types.ObjectType{AttrTypes: models.AuditAttrTypes},
	}
	configObj, diags := types.ObjectValueFrom(ctx, attrTypes, ds.MetalakeDataSourceModel{
		Name:       types.StringValue("my_metalake"),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(models.AuditAttrTypes),
	})
	if diags.HasError() {
		t.Fatalf("failed to build config: %v", diags)
	}
	raw, err := configObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert config: %v", err)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state ds.MetalakeDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.Comment.ValueString() != "This is my metalake" {
		t.Fatalf("unexpected comment: %s", state.Comment.ValueString())
	}
	// A data source mirrors the API, including server-owned properties.
	if got := state.Properties.Elements()["gravitino.identifier"]; got == nil {
		t.Fatalf("expected the server properties to be exposed, got %v", state.Properties)
	}
	if state.Audit.IsNull() {
		t.Fatal("expected audit to be populated")
	}
}

func TestMetalakeDataSource_ReadNotFound(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"code":1003,"type":"NoSuchMetalakeException","message":"Failed to operate metalake(s) [test] operation [LOAD], reason [NoSuchMetalakeException]","stack":["org.apache.gravitino.exceptions.NoSuchMetalakeException: Metalake test does not exist","..."]}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewMetalakeDataSource()
	configureDataSource(t, ctx, d, c)

	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	configObj, _ := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"name":       types.StringType,
		"comment":    types.StringType,
		"properties": types.MapType{ElemType: types.StringType},
		"audit":      types.ObjectType{AttrTypes: models.AuditAttrTypes},
	}, ds.MetalakeDataSourceModel{
		Name:       types.StringValue("nonexistent"),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(models.AuditAttrTypes),
	})
	raw, _ := configObj.ToTerraformValue(ctx)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: raw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a missing metalake")
	}
	if summary := resp.Diagnostics.Errors()[0].Summary(); summary != "Metalake not found" {
		t.Fatalf("unexpected summary: %s", summary)
	}
}

func TestMetalakeDataSource_ReadServerErrorUsesGravitinoError(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalError","message":"boom","stack":["..."]}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewMetalakeDataSource()
	configureDataSource(t, ctx, d, c)

	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	configObj, _ := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"name":       types.StringType,
		"comment":    types.StringType,
		"properties": types.MapType{ElemType: types.StringType},
		"audit":      types.ObjectType{AttrTypes: models.AuditAttrTypes},
	}, ds.MetalakeDataSourceModel{
		Name:       types.StringValue("my_metalake"),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(models.AuditAttrTypes),
	})
	raw, _ := configObj.ToTerraformValue(ctx)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: raw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != `Failed reading metalake "my_metalake"` {
		t.Fatalf("unexpected summary: %s", got)
	}
}

func TestMetalakesDataSource_Read(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.URL.Path != "/api/metalakes" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(models.MetalakeListResponse{
			Code: 0,
			Metalakes: []models.Metalake{
				{Name: "ml1", Comment: "first", Properties: map[string]string{"k": "v"}},
				{Name: "ml2", Comment: "second"},
			},
		})
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewMetalakesDataSource()
	configureDataSource(t, ctx, d, c)

	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	raw := tftypes.NewValue(schemaObj.Type().TerraformType(ctx), nil)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state ds.MetalakesDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if len(state.Metalakes) != 2 {
		t.Fatalf("expected 2 metalakes, got %d", len(state.Metalakes))
	}
	if state.Metalakes[0].Name.ValueString() != "ml1" {
		t.Fatalf("unexpected first metalake: %s", state.Metalakes[0].Name.ValueString())
	}
	if state.Metalakes[0].Properties.Elements()["k"].(types.String).ValueString() != "v" {
		t.Fatalf("unexpected properties: %v", state.Metalakes[0].Properties)
	}
}

func TestMetalakesDataSource_ReadServerError(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalError","message":"boom","stack":["..."]}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewMetalakesDataSource()
	configureDataSource(t, ctx, d, c)

	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	raw := tftypes.NewValue(schemaObj.Type().TerraformType(ctx), nil)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: raw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != `Failed listing metalakes "metalakes"` {
		t.Fatalf("unexpected summary: %s", got)
	}
}
