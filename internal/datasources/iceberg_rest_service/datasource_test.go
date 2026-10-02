package iceberg_rest_service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/iceberg_rest_service"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// icebergAvailableExample and icebergUnavailableExample are the verbatim examples
// of the Gravitino v1.3.1 system spec (docs/open-api/system.yaml,
// components/examples/IcebergRESTServiceAvailable and
// IcebergRESTServiceUnavailable).
const (
	icebergAvailableExample   = `{"code":0,"uri":"http://gravitino-host:9001/iceberg"}`
	icebergUnavailableExample = `{"code":0,"uri":null}`
)

var icebergAttrTypes = map[string]attr.Type{
	"metalake": types.StringType,
	"uri":      types.StringType,
}

func newIcebergSchema(t *testing.T, d datasource.DataSource) schema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func readIceberg(t *testing.T, d datasource.DataSource, metalake types.String) (*datasource.ReadResponse, ds.IcebergRESTServiceDataSourceModel) {
	t.Helper()

	ctx := context.Background()
	schemaObj := newIcebergSchema(t, d)

	configObj, diags := types.ObjectValueFrom(ctx, icebergAttrTypes, ds.IcebergRESTServiceDataSourceModel{
		Metalake: metalake,
		URI:      types.StringNull(),
	})
	if diags.HasError() {
		t.Fatalf("failed to create config object: %v", diags)
	}
	tfVal, err := configObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: tfVal}}, resp)

	var state ds.IcebergRESTServiceDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			t.Fatalf("failed to read state: %v", resp.Diagnostics)
		}
	}
	return resp, state
}

func logIcebergDiagnostics(t *testing.T, resp *datasource.ReadResponse) {
	t.Helper()
	for _, diag := range resp.Diagnostics.Errors() {
		t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
	}
}

// icebergServer serves GET /api/version and the v1.3.1 iceberg-rest endpoint, and
// records the metalake query parameter the data source sent.
func icebergServer(t *testing.T, body string, gotMetalake *string, calls *int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
			_, _ = w.Write([]byte(`{"code":0,"version":{"version":"1.3.1","compileDate":"01/01/2026","gitCommit":"abc"}}`))
		case "/api/system/iceberg-rest":
			if calls != nil {
				atomic.AddInt64(calls, 1)
			}
			if gotMetalake != nil {
				*gotMetalake = r.URL.Query().Get("metalake")
			}
			w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
			_, _ = w.Write([]byte(body))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
}

func TestIcebergRESTServiceDataSource_Metadata(t *testing.T) {
	d := ds.New()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_iceberg_rest_service" {
		t.Fatalf("expected gravitino_iceberg_rest_service, got %s", resp.TypeName)
	}
}

func TestIcebergRESTServiceDataSource_Schema(t *testing.T) {
	s := newIcebergSchema(t, ds.New())

	metalakeAttr, ok := s.Attributes["metalake"]
	if !ok {
		t.Fatal("missing attribute metalake")
	}
	if !metalakeAttr.IsOptional() || metalakeAttr.IsRequired() || metalakeAttr.IsComputed() {
		t.Error("metalake must be optional and not computed")
	}

	uriAttr, ok := s.Attributes["uri"]
	if !ok {
		t.Fatal("missing attribute uri")
	}
	if !uriAttr.IsComputed() {
		t.Error("uri must be computed")
	}

	if len(s.Attributes) != 2 {
		t.Errorf("expected exactly two attributes, got %d", len(s.Attributes))
	}
}

func TestIcebergRESTServiceDataSource_ReadAvailableSpecExample(t *testing.T) {
	var gotMetalake string
	server := icebergServer(t, icebergAvailableExample, &gotMetalake, nil)
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	d := ds.New()
	d.(*ds.IcebergRESTServiceDataSource).SetClient(c)

	resp, state := readIceberg(t, d, types.StringValue("my_metalake"))
	if resp.Diagnostics.HasError() {
		logIcebergDiagnostics(t, resp)
		t.Fatal("unexpected diagnostics errors")
	}
	if gotMetalake != "my_metalake" {
		t.Errorf("metalake query = %q, want my_metalake", gotMetalake)
	}
	if state.URI.IsNull() {
		t.Fatal("uri must be set for the IcebergRESTServiceAvailable example")
	}
	if state.URI.ValueString() != "http://gravitino-host:9001/iceberg" {
		t.Errorf("uri = %q", state.URI.ValueString())
	}
}

func TestIcebergRESTServiceDataSource_ReadUnavailableSpecExample(t *testing.T) {
	server := icebergServer(t, icebergUnavailableExample, nil, nil)
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	d := ds.New()
	d.(*ds.IcebergRESTServiceDataSource).SetClient(c)

	resp, state := readIceberg(t, d, types.StringNull())
	if resp.Diagnostics.HasError() {
		logIcebergDiagnostics(t, resp)
		t.Fatal("unexpected diagnostics errors")
	}
	if !state.URI.IsNull() {
		t.Errorf("uri = %q, want null for the IcebergRESTServiceUnavailable example", state.URI.ValueString())
	}
	if !state.Metalake.IsNull() {
		t.Errorf("metalake must stay null when omitted, got %q", state.Metalake.ValueString())
	}
}

func TestIcebergRESTServiceDataSource_ReadNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":1002,"type":"NotFoundException","message":"no such endpoint"}`))
	}))
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	d := ds.New()
	d.(*ds.IcebergRESTServiceDataSource).SetClient(c)

	resp, _ := readIceberg(t, d, types.StringNull())
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for HTTP 404")
	}
	logIcebergDiagnostics(t, resp)

	var detail strings.Builder
	for _, diag := range resp.Diagnostics.Errors() {
		detail.WriteString(diag.Summary())
		detail.WriteString(" ")
		detail.WriteString(diag.Detail())
		detail.WriteString("\n")
	}
	if !strings.Contains(detail.String(), "requires Gravitino >= 1.3.1") {
		t.Errorf("diagnostics must name the required version, got:\n%s", detail.String())
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state must not be written when the API call failed")
	}
}

// TestIcebergRESTServiceDataSource_VersionGate is the 1.3.0 compatibility gate:
// the data source must fail before touching an endpoint the server does not have.
func TestIcebergRESTServiceDataSource_VersionGate(t *testing.T) {
	var icebergCalls int64
	var gotMetalake string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
			_, _ = w.Write([]byte(`{"code":0,"version":{"version":"1.3.0","compileDate":"01/01/2026","gitCommit":"abc"}}`))
		case "/api/system/iceberg-rest":
			atomic.AddInt64(&icebergCalls, 1)
			gotMetalake = r.URL.Query().Get("metalake")
			_, _ = w.Write([]byte(icebergAvailableExample))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	d := ds.New()
	d.(*ds.IcebergRESTServiceDataSource).SetClient(c)

	resp, _ := readIceberg(t, d, types.StringValue("my_metalake"))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic on a Gravitino 1.3.0 server")
	}
	logIcebergDiagnostics(t, resp)

	var detail strings.Builder
	for _, diag := range resp.Diagnostics.Errors() {
		detail.WriteString(diag.Summary())
		detail.WriteString(" ")
		detail.WriteString(diag.Detail())
		detail.WriteString("\n")
	}
	if !strings.Contains(detail.String(), "requires Gravitino >= 1.3.1") {
		t.Errorf("diagnostics must name the required version, got:\n%s", detail.String())
	}
	if !strings.Contains(detail.String(), "1.3.0") {
		t.Errorf("diagnostics must report the detected server version, got:\n%s", detail.String())
	}
	if got := atomic.LoadInt64(&icebergCalls); got != 0 {
		t.Errorf("GET /api/system/iceberg-rest called %d times; the gate must run first (metalake=%q)", got, gotMetalake)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state must not be written when the server is too old")
	}
}

// TestIcebergRESTServiceResponseDTOMatchesSpec guards the JSON keys of the DTO.
func TestIcebergRESTServiceResponseDTOMatchesSpec(t *testing.T) {
	var available models.IcebergRESTServiceResponse
	if err := json.Unmarshal([]byte(icebergAvailableExample), &available); err != nil {
		t.Fatalf("failed to decode the available example: %v", err)
	}
	if available.Code != 0 || available.URI == nil || *available.URI != "http://gravitino-host:9001/iceberg" {
		t.Errorf("decoded available example = %+v", available)
	}

	var unavailable models.IcebergRESTServiceResponse
	if err := json.Unmarshal([]byte(icebergUnavailableExample), &unavailable); err != nil {
		t.Fatalf("failed to decode the unavailable example: %v", err)
	}
	if unavailable.Code != 0 || unavailable.URI != nil {
		t.Errorf("decoded unavailable example = %+v, want uri null", unavailable)
	}
}
