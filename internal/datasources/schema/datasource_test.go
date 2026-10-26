package schema_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	datasourceschema "github.com/gravitino/terraform-provider-gravitino/internal/datasources/schema"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/models/modeltest"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// schemaResponse is the verbatim `SchemaResponse` example of schemas.yaml
// (components/examples/SchemaResponse).
const schemaResponse = `{
  "code": 0,
  "schema": {
    "name": "my_hive_schema",
    "comment": "This is my Hive schema",
    "properties": {
      "key1": "value1",
      "key2": "value2",
      "location": "hdfs://0.0.0.0:9000/user/hive/warehouse"
    },
    "audit": {
      "creator": "gravitino",
      "createTime": "2023-12-08T08:37:43.531Z"
    }
  }
}`

// schemaListResponse is the verbatim `SchemaListResponse` example of
// schemas.yaml.
const schemaListResponse = `{
  "code": 0,
  "identifiers": [
    {
      "namespace": [
        "my_metalake",
        "my_hive_catalog"
      ],
      "name": "my_hive_schema"
    }
  ]
}`

// noSuchSchemaError is the verbatim `NoSuchSchemaException` example of
// schemas.yaml, returned with HTTP status 404.
const noSuchSchemaError = `{
  "code": 1003,
  "type": "NoSuchSchemaException",
  "message": "Failed to operate schema(s) [my_hive_schema1] operation [LOAD] under catalog [my_hive_catalog], reason [NoSuchSchemaException]",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchSchemaException: Hive schema (database) does not exist: my_hive_schema1 in Hive Metastore",
    "..."
  ]
}`

func TestSchemaDataSourceMetadata(t *testing.T) {
	d := datasourceschema.NewSchemaDataSource()
	var req datasource.MetadataRequest
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), req, &resp)
	if resp.TypeName != "gravitino_schema" {
		t.Errorf("Expected type name gravitino_schema, got %s", resp.TypeName)
	}
}

func TestSchemasDataSourceMetadata(t *testing.T) {
	d := datasourceschema.NewSchemasDataSource()
	var req datasource.MetadataRequest
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), req, &resp)
	if resp.TypeName != "gravitino_schemas" {
		t.Errorf("Expected type name gravitino_schemas, got %s", resp.TypeName)
	}
}

func dataSourceSchema(t *testing.T, d datasource.DataSource) schema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestSchemaDataSource_Schema(t *testing.T) {
	s := dataSourceSchema(t, datasourceschema.NewSchemaDataSource())

	for _, name := range []string{"metalake", "catalog", "name", "comment", "properties", "audit"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Fatalf("attribute %q is missing", name)
		}
	}
	for _, name := range []string{"metalake", "catalog", "name"} {
		if !s.Attributes[name].IsRequired() {
			t.Errorf("attribute %q must be required", name)
		}
	}
	for _, name := range []string{"comment", "properties", "audit"} {
		if !s.Attributes[name].IsComputed() {
			t.Errorf("attribute %q must be computed", name)
		}
	}
}

func TestSchemasDataSource_Schema(t *testing.T) {
	s := dataSourceSchema(t, datasourceschema.NewSchemasDataSource())

	for _, name := range []string{"metalake", "catalog", "schemas"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Fatalf("attribute %q is missing", name)
		}
	}
	if !s.Attributes["schemas"].IsComputed() {
		t.Error("schemas must be computed")
	}
	nested, ok := s.Attributes["schemas"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("schemas is %T, want a list nested attribute", s.Attributes["schemas"])
	}
	for _, name := range []string{"name", "comment", "properties", "audit"} {
		if _, ok := nested.NestedObject.Attributes[name]; !ok {
			t.Errorf("nested attribute %q is missing", name)
		}
	}
}

func TestSchemaDataSources_Configure(t *testing.T) {
	constructors := map[string]func() datasource.DataSource{
		"gravitino_schema":  datasourceschema.NewSchemaDataSource,
		"gravitino_schemas": datasourceschema.NewSchemasDataSource,
	}

	for name, constructor := range constructors {
		t.Run(name+"/nil provider data", func(t *testing.T) {
			d := constructor()
			configured := d.(datasource.DataSourceWithConfigure)
			resp := &datasource.ConfigureResponse{}
			configured.Configure(context.Background(), datasource.ConfigureRequest{}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("nil provider data must configure without diagnostics: %v", resp.Diagnostics)
			}
		})

		t.Run(name+"/unexpected provider data", func(t *testing.T) {
			d := constructor()
			configured := d.(datasource.DataSourceWithConfigure)
			resp := &datasource.ConfigureResponse{}
			configured.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "not a client"}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected a diagnostic for unexpected provider data")
			}
			if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Unexpected provider data type") {
				t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
			}
		})
	}
}

func configureSchemaDataSource(t *testing.T, d datasource.DataSource, c *client.Client) {
	t.Helper()
	configured := d.(datasource.DataSourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	configured.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("configure diagnostics: %v", resp.Diagnostics)
	}
}

func schemaConfigValue(t *testing.T, ctx context.Context, s schema.Schema, model interface{}) tftypes.Value {
	t.Helper()
	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("failed to build config object: %v", diags)
	}
	value, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert config: %v", err)
	}
	return value
}

func newSchemaDatasourceServer(t *testing.T, handler http.HandlerFunc) *client.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	return c
}

func readSchemaDataSource(t *testing.T, d datasource.DataSource, c *client.Client, config datasourceschema.SchemaDataSourceModel) *datasource.ReadResponse {
	t.Helper()
	configureSchemaDataSource(t, d, c)

	ctx := context.Background()
	s := dataSourceSchema(t, d)
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: schemaConfigValue(t, ctx, s, config)}}, resp)
	return resp
}

func readSchemasDataSource(t *testing.T, d datasource.DataSource, c *client.Client, config datasourceschema.SchemasDataSourceModel) *datasource.ReadResponse {
	t.Helper()
	configureSchemaDataSource(t, d, c)

	ctx := context.Background()
	s := dataSourceSchema(t, d)
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: schemaConfigValue(t, ctx, s, config)}}, resp)
	return resp
}

func getSchemaConfig() datasourceschema.SchemaDataSourceModel {
	return datasourceschema.SchemaDataSourceModel{
		Metalake:   types.StringValue("my_metalake"),
		Catalog:    types.StringValue("my_hive_catalog"),
		Name:       types.StringValue("my_hive_schema"),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(models.AuditAttrTypes),
	}
}

func TestSchemaDataSource_Read(t *testing.T) {
	var requestPath string
	c := newSchemaDatasourceServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, schemaResponse)
	})

	ctx := context.Background()
	resp := readSchemaDataSource(t, datasourceschema.NewSchemaDataSource(), c, getSchemaConfig())
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if want := "/api/metalakes/my_metalake/catalogs/my_hive_catalog/schemas/my_hive_schema"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}

	var state datasourceschema.SchemaDataSourceModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("failed to read state: %v", d)
	}
	if got := state.Comment.ValueString(); got != "This is my Hive schema" {
		t.Errorf("comment = %q", got)
	}
	if got := state.Properties.Elements()["key1"]; !got.Equal(types.StringValue("value1")) {
		t.Errorf("properties[key1] = %v", got)
	}
	if state.Audit.IsNull() {
		t.Fatal("audit must be set from the response")
	}
	if got := state.Audit.Attributes()["creator"].(types.String).ValueString(); got != "gravitino" {
		t.Errorf("audit creator = %q", got)
	}
}

// TestSchemaDataSource_ReadWithoutProperties asserts an absent properties object
// becomes a null map rather than an empty one.
func TestSchemaDataSource_ReadWithoutProperties(t *testing.T) {
	c := newSchemaDatasourceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{"code":0,"schema":{"name":"my_hive_schema","comment":"","audit":{"creator":"gravitino"}}}`)
	})

	ctx := context.Background()
	resp := readSchemaDataSource(t, datasourceschema.NewSchemaDataSource(), c, getSchemaConfig())
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state datasourceschema.SchemaDataSourceModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("failed to read state: %v", d)
	}
	if !state.Properties.IsNull() {
		t.Errorf("properties = %v, want null", state.Properties)
	}
}

func TestSchemaDataSource_ReadNotFound(t *testing.T) {
	c := newSchemaDatasourceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchSchemaError)
	})

	resp := readSchemaDataSource(t, datasourceschema.NewSchemaDataSource(), c, getSchemaConfig())
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a missing schema")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Schema not found" {
		t.Fatalf("unexpected summary: %s", got)
	}
}

func TestSchemaDataSource_ReadServerError(t *testing.T) {
	c := newSchemaDatasourceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalServerError","message":"boom","stack":["..."]}`)
	})

	resp := readSchemaDataSource(t, datasourceschema.NewSchemaDataSource(), c, getSchemaConfig())
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed reading schema "my_hive_schema"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func getSchemasConfig(t *testing.T) datasourceschema.SchemasDataSourceModel {
	t.Helper()
	s := dataSourceSchema(t, datasourceschema.NewSchemasDataSource())
	nested := s.Attributes["schemas"].(schema.ListNestedAttribute)
	itemTypes := make(map[string]attr.Type, len(nested.NestedObject.Attributes))
	for name, attribute := range nested.NestedObject.Attributes {
		itemTypes[name] = attribute.GetType()
	}
	return datasourceschema.SchemasDataSourceModel{
		Metalake: types.StringValue("my_metalake"),
		Catalog:  types.StringValue("my_hive_catalog"),
		Schemas:  types.ListNull(types.ObjectType{AttrTypes: itemTypes}),
	}
}

func TestSchemasDataSource_Read(t *testing.T) {
	var paths []string
	c := newSchemaDatasourceServer(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case "/api/metalakes/my_metalake/catalogs/my_hive_catalog/schemas":
			fmt.Fprint(w, schemaListResponse)
		case "/api/metalakes/my_metalake/catalogs/my_hive_catalog/schemas/my_hive_schema":
			fmt.Fprint(w, schemaResponse)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, noSuchSchemaError)
		}
	})

	ctx := context.Background()
	resp := readSchemasDataSource(t, datasourceschema.NewSchemasDataSource(), c, getSchemasConfig(t))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if len(paths) != 2 {
		t.Fatalf("expected a list request and one detail request, got %v", paths)
	}

	var state datasourceschema.SchemasDataSourceModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("failed to read state: %v", d)
	}
	elements := state.Schemas.Elements()
	if len(elements) != 1 {
		t.Fatalf("expected one schema, got %d", len(elements))
	}
	attrs := elements[0].(types.Object).Attributes()
	if got := attrs["name"].(types.String).ValueString(); got != "my_hive_schema" {
		t.Errorf("name = %q", got)
	}
	if got := attrs["comment"].(types.String).ValueString(); got != "This is my Hive schema" {
		t.Errorf("comment = %q", got)
	}
	if got := attrs["properties"].(types.Map).Elements()["key2"]; !got.Equal(types.StringValue("value2")) {
		t.Errorf("properties[key2] = %v", got)
	}
	if attrs["audit"].(types.Object).IsNull() {
		t.Error("audit must be set from the response")
	}
}

// TestSchemasDataSource_ReadEmptyList asserts an empty identifier list becomes an
// empty list value, not a null one.
func TestSchemasDataSource_ReadEmptyList(t *testing.T) {
	c := newSchemaDatasourceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{"code":0,"identifiers":[]}`)
	})

	ctx := context.Background()
	resp := readSchemasDataSource(t, datasourceschema.NewSchemasDataSource(), c, getSchemasConfig(t))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state datasourceschema.SchemasDataSourceModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("failed to read state: %v", d)
	}
	if state.Schemas.IsNull() {
		t.Fatal("an empty result must not be a null list")
	}
	if len(state.Schemas.Elements()) != 0 {
		t.Fatalf("expected no schemas, got %d", len(state.Schemas.Elements()))
	}
}

func TestSchemasDataSource_ReadNotFound(t *testing.T) {
	c := newSchemaDatasourceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchSchemaError)
	})

	resp := readSchemasDataSource(t, datasourceschema.NewSchemasDataSource(), c, getSchemasConfig(t))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the metalake or catalog does not exist")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Metalake or catalog not found" {
		t.Fatalf("unexpected summary: %s", got)
	}
}

func TestSchemasDataSource_ReadServerError(t *testing.T) {
	c := newSchemaDatasourceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalServerError","message":"boom","stack":["..."]}`)
	})

	resp := readSchemasDataSource(t, datasourceschema.NewSchemasDataSource(), c, getSchemasConfig(t))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed listing schemas "my_hive_catalog"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

// TestSchemaDataSourceModelsMatchSchema keeps the model in lockstep with the
// gravitino_schema schema.
func TestSchemaDataSourceModelsMatchSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	datasourceschema.NewSchemaDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	modeltest.AssertSchemaMatches(t, datasourceschema.SchemaDataSourceModel{}, resp.Schema.Attributes, resp.Schema.Blocks)
}

// TestSchemasDataSourceModelsMatchSchema keeps the model in lockstep with the
// gravitino_schemas schema.
func TestSchemasDataSourceModelsMatchSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	datasourceschema.NewSchemasDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	modeltest.AssertSchemaMatches(t, datasourceschema.SchemasDataSourceModel{}, resp.Schema.Attributes, resp.Schema.Blocks)
}
