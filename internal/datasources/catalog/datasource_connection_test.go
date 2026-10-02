package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/catalog"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Spec-faithful payloads: every JSON literal below is copied from the
// `examples:` section of catalogs.yaml (v1.3.0 and v1.3.1).
const (
	// TestConnectionSuccess of POST /metalakes/{metalake}/catalogs/testConnection.
	connectionTestSuccessExample = `{"code":0}`

	// ConnectionFailedException of catalogs.yaml.
	connectionTestFailedExample = `{"code":1007,"type":"ConnectionFailedException","message":"Failed to run getAllDatabases in Hive Metastore: Failed to connect to Hive Metastore","stack":["org.apache.gravitino.exceptions.ConnectionFailedException: Failed to run getAllDatabases in Hive Metastore: Failed to connect to Hive Metastore","..."]}`

	// NoSuchCatalogException value of the 1.3.1 testExistingCatalogConnection example.
	connectionTestNoSuchCatalogExample = `{"code":1003,"type":"NoSuchCatalogException","message":"Catalog my_metalake.my_catalog does not exist"}`

	// CatalogCreate example, the request body of the proposed-config endpoint.
	proposedCatalogExample = `{"name":"my_hive_catalog","type":"relational","provider":"hive","comment":"This is my hive catalog","properties":{"metastore.uris":"thrift://127.0.0.1:9083","key1":"value1"}}`
)

var connectionTestAttrTypes = map[string]attr.Type{
	"metalake":         types.StringType,
	"catalog":          types.StringType,
	"name":             types.StringType,
	"type":             types.StringType,
	"catalog_provider": types.StringType,
	"comment":          types.StringType,
	"properties":       types.MapType{ElemType: types.StringType},
	"success":          types.BoolType,
	"message":          types.StringType,
}

// connectionTestRead runs the data source Read and returns the response plus the
// state it wrote (only when no diagnostic error was raised).
func connectionTestRead(t *testing.T, d datasource.DataSource, model ds.CatalogConnectionTestDataSourceModel) (*datasource.ReadResponse, ds.CatalogConnectionTestDataSourceModel) {
	t.Helper()
	ctx := context.Background()

	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)

	configObj, diags := types.ObjectValueFrom(ctx, connectionTestAttrTypes, model)
	if diags.HasError() {
		t.Fatalf("failed to create config object: %v", diags)
	}
	raw, err := configObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert config to terraform value: %v", err)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)

	var state ds.CatalogConnectionTestDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
	}
	return resp, state
}

func newProposedConfigModel() ds.CatalogConnectionTestDataSourceModel {
	return ds.CatalogConnectionTestDataSourceModel{
		Metalake: types.StringValue("test_metalake"),
		Name:     types.StringValue("my_hive_catalog"),
		Type:     types.StringValue("relational"),
		Provider: types.StringValue("hive"),
		Comment:  types.StringValue("This is my hive catalog"),
		Properties: types.MapValueMust(types.StringType, map[string]attr.Value{
			"metastore.uris": types.StringValue("thrift://127.0.0.1:9083"),
			"key1":           types.StringValue("value1"),
		}),
	}
}

func TestCatalogConnectionTestDataSource_Schema(t *testing.T) {
	d := ds.NewConnectionTestDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_catalog_connection_test" {
		t.Fatalf("expected gravitino_catalog_connection_test, got %s", resp.TypeName)
	}

	schemaResp := &datasource.SchemaResponse{}
	d.Schema(context.TODO(), datasource.SchemaRequest{}, schemaResp)
	for _, name := range []string{"metalake", "catalog", "name", "type", "catalog_provider", "comment", "properties", "success", "message"} {
		if _, ok := schemaResp.Schema.Attributes[name]; !ok {
			t.Fatalf("expected attribute %q in the schema", name)
		}
	}
	if !schemaResp.Schema.Attributes["success"].IsComputed() {
		t.Fatal("success must be computed")
	}
	if !schemaResp.Schema.Attributes["message"].IsComputed() {
		t.Fatal("message must be computed")
	}
}

func TestCatalogConnectionTestDataSource_ProposedConfigSuccess(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/metalakes/test_metalake/catalogs/testConnection" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, connectionTestSuccessExample)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewConnectionTestDataSource()
	d.(*ds.CatalogConnectionTestDataSource).SetClient(c)

	resp, state := connectionTestRead(t, d, newProposedConfigModel())
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	// The request body must be the catalog create body of catalogs.yaml, not a
	// hand-rolled approximation.
	var want map[string]interface{}
	if err := json.Unmarshal([]byte(proposedCatalogExample), &want); err != nil {
		t.Fatalf("invalid example: %v", err)
	}
	if !reflect.DeepEqual(gotBody, want) {
		t.Fatalf("request body = %v, want %v", gotBody, want)
	}

	if !state.Success.ValueBool() {
		t.Fatalf("expected success, got %v", state.Success)
	}
	if state.Message.ValueString() != "" {
		t.Fatalf("expected an empty message on success, got %q", state.Message.ValueString())
	}
}

func TestCatalogConnectionTestDataSource_ProposedConfigFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		// Expected test failures are an HTTP 200 with an application error code.
		fmt.Fprint(w, connectionTestFailedExample)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewConnectionTestDataSource()
	d.(*ds.CatalogConnectionTestDataSource).SetClient(c)

	resp, state := connectionTestRead(t, d, newProposedConfigModel())
	if resp.Diagnostics.HasError() {
		t.Fatalf("a completed failed test must not raise a diagnostic: %v", resp.Diagnostics)
	}
	if state.Success.ValueBool() {
		t.Fatal("expected success = false")
	}
	want := "Failed to run getAllDatabases in Hive Metastore: Failed to connect to Hive Metastore"
	if got := state.Message.ValueString(); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestCatalogConnectionTestDataSource_ExistingCatalogSuccess(t *testing.T) {
	var tested atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case "/api/version":
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.1","compileDate":"2026-01-01","gitCommit":"abc"}}`)
		case "/api/metalakes/test_metalake/catalogs/my_hive_catalog/testConnection":
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			// The optional updates body must be absent, not a literal "null".
			if r.ContentLength != 0 {
				t.Errorf("expected no request body, got %d bytes", r.ContentLength)
			}
			tested.Store(true)
			fmt.Fprint(w, connectionTestSuccessExample)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewConnectionTestDataSource()
	d.(*ds.CatalogConnectionTestDataSource).SetClient(c)

	resp, state := connectionTestRead(t, d, ds.CatalogConnectionTestDataSourceModel{
		Metalake:   types.StringValue("test_metalake"),
		Catalog:    types.StringValue("my_hive_catalog"),
		Properties: types.MapNull(types.StringType),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !tested.Load() {
		t.Fatal("the existing catalog connection test was never called")
	}
	if !state.Success.ValueBool() {
		t.Fatalf("expected success, got %v", state.Success)
	}
}

func TestCatalogConnectionTestDataSource_ExistingCatalogFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case "/api/version":
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.4.0","compileDate":"2026-01-01","gitCommit":"abc"}}`)
		case "/api/metalakes/my_metalake/catalogs/my_catalog/testConnection":
			fmt.Fprint(w, connectionTestNoSuchCatalogExample)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewConnectionTestDataSource()
	d.(*ds.CatalogConnectionTestDataSource).SetClient(c)

	resp, state := connectionTestRead(t, d, ds.CatalogConnectionTestDataSourceModel{
		Metalake:   types.StringValue("my_metalake"),
		Catalog:    types.StringValue("my_catalog"),
		Properties: types.MapNull(types.StringType),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if state.Success.ValueBool() {
		t.Fatal("expected success = false")
	}
	if got, want := state.Message.ValueString(), "Catalog my_metalake.my_catalog does not exist"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestCatalogConnectionTestDataSource_ExistingCatalogVersionGate(t *testing.T) {
	var tested atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case "/api/version":
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.0","compileDate":"2026-01-01","gitCommit":"abc"}}`)
		default:
			tested.Store(true)
			fmt.Fprint(w, connectionTestSuccessExample)
		}
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewConnectionTestDataSource()
	d.(*ds.CatalogConnectionTestDataSource).SetClient(c)

	resp, _ := connectionTestRead(t, d, ds.CatalogConnectionTestDataSourceModel{
		Metalake:   types.StringValue("test_metalake"),
		Catalog:    types.StringValue("my_hive_catalog"),
		Properties: types.MapNull(types.StringType),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the version gate to reject Gravitino 1.3.0")
	}
	summary := resp.Diagnostics.Errors()[0].Summary()
	if summary != "Unsupported Gravitino version" {
		t.Fatalf("unexpected summary: %s", summary)
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "1.3.1") {
		t.Fatalf("the diagnostic must name the required version, got: %s", resp.Diagnostics.Errors()[0].Detail())
	}
	if tested.Load() {
		t.Fatal("the connection test must not be sent to a server that does not support it")
	}
}

// TestCatalogConnectionTestDataSource_UnparsableVersionRejected: a version the
// provider cannot read counts as unsupported (models.ServerVersionAtLeast), so
// the request is not sent to an endpoint that may not exist.
func TestCatalogConnectionTestDataSource_UnparsableVersionRejected(t *testing.T) {
	var tested atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case "/api/version":
			fmt.Fprint(w, `{"code":0,"version":{"version":"dev","compileDate":"","gitCommit":""}}`)
		default:
			tested.Store(true)
			fmt.Fprint(w, connectionTestSuccessExample)
		}
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewConnectionTestDataSource()
	d.(*ds.CatalogConnectionTestDataSource).SetClient(c)

	resp, _ := connectionTestRead(t, d, ds.CatalogConnectionTestDataSourceModel{
		Metalake:   types.StringValue("test_metalake"),
		Catalog:    types.StringValue("my_hive_catalog"),
		Properties: types.MapNull(types.StringType),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an unparsable server version to be rejected")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Unsupported Gravitino version" {
		t.Fatalf("unexpected summary: %s", got)
	}
	if tested.Load() {
		t.Fatal("the connection test must not be sent when the server version is unknown")
	}
}

func TestCatalogConnectionTestDataSource_ServerErrorUsesGravitinoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalError","message":"boom","stack":["..."]}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewConnectionTestDataSource()
	d.(*ds.CatalogConnectionTestDataSource).SetClient(c)

	resp, _ := connectionTestRead(t, d, newProposedConfigModel())
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != `Failed testing the connection of catalog "my_hive_catalog"` {
		t.Fatalf("unexpected summary: %s", got)
	}
}

func TestCatalogConnectionTestDataSource_VariantValidator(t *testing.T) {
	properties := types.MapValueMust(types.StringType, map[string]attr.Value{
		"metastore.uris": types.StringValue("thrift://127.0.0.1:9083"),
	})

	tests := []struct {
		name    string
		model   ds.CatalogConnectionTestDataSourceModel
		wantErr string
	}{
		{
			name: "existing catalog only",
			model: ds.CatalogConnectionTestDataSourceModel{
				Metalake: types.StringValue("m"), Catalog: types.StringValue("c"), Properties: types.MapNull(types.StringType),
			},
		},
		{
			name: "proposed config only",
			model: ds.CatalogConnectionTestDataSourceModel{
				Metalake: types.StringValue("m"), Name: types.StringValue("n"), Type: types.StringValue("relational"),
				Properties: types.MapNull(types.StringType),
			},
		},
		{
			name: "proposed config with optional attributes",
			model: ds.CatalogConnectionTestDataSourceModel{
				Metalake:   types.StringValue("m"),
				Name:       types.StringValue("n"),
				Type:       types.StringValue("relational"),
				Provider:   types.StringValue("hive"),
				Comment:    types.StringValue("c"),
				Properties: properties,
			},
		},
		{
			name: "empty catalog string with a proposed config",
			model: ds.CatalogConnectionTestDataSourceModel{
				Metalake: types.StringValue("m"), Catalog: types.StringValue(""),
				Name: types.StringValue("n"), Type: types.StringValue("relational"), Properties: types.MapNull(types.StringType),
			},
		},
		{
			name: "neither variant",
			model: ds.CatalogConnectionTestDataSourceModel{
				Metalake: types.StringValue("m"), Properties: types.MapNull(types.StringType),
			},
			wantErr: "Missing catalog connection test configuration",
		},
		{
			name: "name without type",
			model: ds.CatalogConnectionTestDataSourceModel{
				Metalake: types.StringValue("m"), Name: types.StringValue("n"), Properties: types.MapNull(types.StringType),
			},
			wantErr: "Missing catalog connection test configuration",
		},
		{
			name: "type without name",
			model: ds.CatalogConnectionTestDataSourceModel{
				Metalake: types.StringValue("m"), Type: types.StringValue("relational"), Properties: types.MapNull(types.StringType),
			},
			wantErr: "Missing catalog connection test configuration",
		},
		{
			name: "existing catalog combined with a proposed config",
			model: ds.CatalogConnectionTestDataSourceModel{
				Metalake: types.StringValue("m"), Catalog: types.StringValue("c"),
				Name: types.StringValue("n"), Type: types.StringValue("relational"), Properties: types.MapNull(types.StringType),
			},
			wantErr: "Conflicting catalog connection test configuration",
		},
		{
			name: "existing catalog combined with properties",
			model: ds.CatalogConnectionTestDataSourceModel{
				Metalake: types.StringValue("m"), Catalog: types.StringValue("c"), Properties: properties,
			},
			wantErr: "Conflicting catalog connection test configuration",
		},
		{
			name: "existing catalog combined with a provider",
			model: ds.CatalogConnectionTestDataSourceModel{
				Metalake: types.StringValue("m"), Catalog: types.StringValue("c"),
				Provider: types.StringValue("hive"), Properties: types.MapNull(types.StringType),
			},
			wantErr: "Conflicting catalog connection test configuration",
		},
	}

	d := ds.NewConnectionTestDataSource()
	validators := d.(datasource.DataSourceWithConfigValidators).ConfigValidators(context.Background())
	if len(validators) != 1 {
		t.Fatalf("expected one config validator, got %d", len(validators))
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			schemaResp := &datasource.SchemaResponse{}
			d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)

			configObj, diags := types.ObjectValueFrom(ctx, connectionTestAttrTypes, tt.model)
			if diags.HasError() {
				t.Fatalf("failed to create config object: %v", diags)
			}
			raw, err := configObj.ToTerraformValue(ctx)
			if err != nil {
				t.Fatalf("failed to convert config to terraform value: %v", err)
			}

			resp := &datasource.ValidateConfigResponse{}
			validators[0].ValidateDataSource(ctx,
				datasource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)

			if tt.wantErr == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
				}
				return
			}
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected a diagnostic error")
			}
			if got := resp.Diagnostics.Errors()[0].Summary(); got != tt.wantErr {
				t.Fatalf("summary = %q, want %q", got, tt.wantErr)
			}
		})
	}
}
