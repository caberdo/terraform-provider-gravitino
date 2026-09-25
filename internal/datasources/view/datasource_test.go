package view_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	dsview "github.com/gravitino/terraform-provider-gravitino/internal/datasources/view"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// dsViewAudit is the audit block of the ViewResponse example in views.yaml.
func dsViewAudit() *models.Audit {
	create, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return &models.Audit{
		Creator:          "gravitino",
		CreateTime:       &create,
		LastModifier:     "gravitino",
		LastModifiedTime: &create,
	}
}

// dsViewResponse is the ViewResponse example in views.yaml.
func dsViewResponse(name string) models.ViewResponse {
	return models.ViewResponse{
		Code: 0,
		View: models.View{
			Name:    name,
			Comment: "This is a view",
			Columns: []models.Column{
				{Name: "id", Type: models.DataType{Type: "long"}, Comment: "id column", Nullable: true},
			},
			Representations: []models.ViewRepresentation{
				{Type: "sql", Dialect: "trino", SQL: "SELECT id FROM t"},
			},
			Properties: map[string]string{"key": "value"},
			Audit:      dsViewAudit(),
		},
	}
}

const dsNoSuchViewBody = `{
	"code": 1003,
	"type": "NoSuchViewException",
	"message": "Failed to operate view(s) [test_view] operation [LOAD] under schema [test_schema], reason [NoSuchViewException]",
	"stack": ["org.apache.gravitino.exceptions.NoSuchViewException: View test_view does not exist", "..."]
}`

func TestViewDataSourceMetadata(t *testing.T) {
	d := dsview.NewViewDataSource()
	var req datasource.MetadataRequest
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), req, &resp)
	if resp.TypeName != "gravitino_view" {
		t.Errorf("Expected type name gravitino_view, got %s", resp.TypeName)
	}
}

func TestViewsDataSourceMetadata(t *testing.T) {
	d := dsview.NewViewsDataSource()
	var req datasource.MetadataRequest
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), req, &resp)
	if resp.TypeName != "gravitino_views" {
		t.Errorf("Expected type name gravitino_views, got %s", resp.TypeName)
	}
}

func TestViewDataSourceSchema(t *testing.T) {
	d := dsview.NewViewDataSource()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)

	schemaObj := resp.Schema
	for _, name := range []string{"metalake", "catalog", "schema", "name", "comment", "column", "representation", "default_catalog", "default_schema", "properties", "audit"} {
		if _, ok := schemaObj.Attributes[name]; !ok {
			t.Errorf("schema is missing attribute %q", name)
		}
	}
	if _, ok := schemaObj.Attributes["view_def"]; ok {
		t.Error("view_def must not exist: Gravitino v1.3.0 has no viewDef field")
	}
	if _, ok := schemaObj.Attributes["representation"].(dsschema.ListNestedAttribute); !ok {
		t.Errorf("representation is not a list nested attribute: %T", schemaObj.Attributes["representation"])
	}
}

func TestViewDataSourceRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/metalakes/ml/catalogs/cat/schemas/sch/views/view1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_ = json.NewEncoder(w).Encode(dsViewResponse("view1"))
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := dsview.NewViewDataSource()
	d.(*dsview.ViewDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema
	attrTypes := schemaObj.Type().(types.ObjectType).AttributeTypes()

	configModel := dsview.ViewDataSourceModel{
		Metalake:       types.StringValue("ml"),
		Catalog:        types.StringValue("cat"),
		Schema:         types.StringValue("sch"),
		Name:           types.StringValue("view1"),
		Comment:        types.StringNull(),
		DefaultCatalog: types.StringNull(),
		DefaultSchema:  types.StringNull(),
		Properties:     types.MapNull(types.StringType),
		Audit:          types.ObjectNull(models.AuditAttrTypes),
	}

	configObj, diags := types.ObjectValueFrom(ctx, attrTypes, configModel)
	if diags.HasError() {
		t.Fatalf("failed to build config: %v", diags)
	}
	tfVal, err := configObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert config: %v", err)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: tfVal}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var state dsview.ViewDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}

	if state.Comment.ValueString() != "This is a view" {
		t.Errorf("comment = %q, want 'This is a view'", state.Comment.ValueString())
	}
	if len(state.Columns) != 1 {
		t.Fatalf("columns = %d, want 1", len(state.Columns))
	}
	if state.Columns[0].Name.ValueString() != "id" || state.Columns[0].Type.ValueString() != "long" {
		t.Errorf("column = %#v", state.Columns[0])
	}
	if len(state.Representations) != 1 {
		t.Fatalf("representations = %d, want 1", len(state.Representations))
	}
	if state.Representations[0].Dialect.ValueString() != "trino" || state.Representations[0].SQL.ValueString() != "SELECT id FROM t" {
		t.Errorf("representation = %#v", state.Representations[0])
	}
	if state.Properties.IsNull() {
		t.Error("properties must be populated")
	}
	if state.Audit.IsNull() {
		t.Error("audit must be populated")
	}
}

// TestViewDataSourceReadNotFound proves a real Gravitino 404 surfaces as an
// error diagnostic instead of a state with empty attributes.
func TestViewDataSourceReadNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(dsNoSuchViewBody))
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := dsview.NewViewDataSource()
	d.(*dsview.ViewDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	configModel := dsview.ViewDataSourceModel{
		Metalake:   types.StringValue("ml"),
		Catalog:    types.StringValue("cat"),
		Schema:     types.StringValue("sch"),
		Name:       types.StringValue("gone"),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(models.AuditAttrTypes),
	}
	configObj, _ := types.ObjectValueFrom(ctx, schemaObj.Type().(types.ObjectType).AttributeTypes(), configModel)
	tfVal, _ := configObj.ToTerraformValue(ctx)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: tfVal}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a 404 to produce an error diagnostic")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if detail == "" || detail == "no message" {
		t.Errorf("error detail must carry the Gravitino message, got %q", detail)
	}
}

// TestViewsDataSourceReadSkipsDeletedViews covers views.yaml having no
// `details` query parameter: the data source lists identifiers and then loads
// every view individually, skipping views dropped in between.
func TestViewsDataSourceReadSkipsDeletedViews(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case "/api/metalakes/ml/catalogs/cat/schemas/sch/views":
			_ = json.NewEncoder(w).Encode(models.IdentifiersResponse{
				Code: 0,
				Identifiers: []models.NameIdentifier{
					{Namespace: []string{"ml", "cat", "sch"}, Name: "view1"},
					{Namespace: []string{"ml", "cat", "sch"}, Name: "gone"},
				},
			})
		case "/api/metalakes/ml/catalogs/cat/schemas/sch/views/view1":
			_ = json.NewEncoder(w).Encode(dsViewResponse("view1"))
		case "/api/metalakes/ml/catalogs/cat/schemas/sch/views/gone":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(dsNoSuchViewBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := dsview.NewViewsDataSource()
	d.(*dsview.ViewsDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	configModel := dsview.ViewsDataSourceModel{
		Metalake: types.StringValue("ml"),
		Catalog:  types.StringValue("cat"),
		Schema:   types.StringValue("sch"),
	}
	configObj, diags := types.ObjectValueFrom(ctx, schemaObj.Type().(types.ObjectType).AttributeTypes(), configModel)
	if diags.HasError() {
		t.Fatalf("failed to build config: %v", diags)
	}
	tfVal, err := configObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert config: %v", err)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: tfVal}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var state dsview.ViewsDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}

	if len(state.Views) != 1 {
		t.Fatalf("views = %d, want 1 (the deleted view must be skipped)", len(state.Views))
	}
	entry := state.Views[0]
	if entry.Name.ValueString() != "view1" {
		t.Errorf("name = %q, want view1", entry.Name.ValueString())
	}
	if len(entry.Representations) != 1 || entry.Representations[0].Type.ValueString() != "sql" {
		t.Errorf("representations = %#v", entry.Representations)
	}
	if len(entry.Columns) != 1 || entry.Columns[0].Comment.ValueString() != "id column" {
		t.Errorf("columns = %#v", entry.Columns)
	}
	if entry.Properties.IsNull() {
		t.Error("properties must be populated")
	}
}
