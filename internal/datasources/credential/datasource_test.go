package credential_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/credential"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// credentialResponseExample is the exact `CredentialResponse` example of the
// Gravitino v1.3.0 credentials spec (docs/open-api/credentials.yaml,
// components/examples/CredentialResponse), with the trailing comma of the YAML
// flow mapping removed so it is valid JSON.
const credentialResponseExample = `{
  "code": 0,
  "credentials": [
    {
      "credentialType": "s3-token",
      "expireTimeInMs": 1735891948411,
      "credentialInfo": {
        "s3-access-key-id": "value1",
        "s3-secret-access-key": "value2",
        "s3-session-token": "value3"
      }
    },
    {
      "credentialType": "s3-secret-key",
      "expireTimeInMs": 0,
      "credentialInfo": {
        "s3-access-key-id": "value1",
        "s3-secret-access-key": "value2"
      }
    }
  ]
}`

// noSuchMetalakeExample is the error payload of the spec's 404 example for the
// credentials endpoint (metalakes.yaml components/examples/NoSuchMetalakeException).
const noSuchMetalakeExample = `{
  "code": 1003,
  "type": "NoSuchMetalakeException",
  "message": "Metalake test_metalake does not exist",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchMetalakeException: Metalake test_metalake does not exist"
  ]
}`

const noSuchCatalogExample = `{
  "code": 1003,
  "type": "NoSuchCatalogException",
  "message": "Failed to get credentials under object [], reason [Catalog probe_ml.nope_cat does not exist]",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchCatalogException: Catalog probe_ml.nope_cat does not exist"
  ]
}`

var credentialListType = types.ListType{ElemType: types.ObjectType{AttrTypes: ds.CredentialItemAttrTypes}}

var credentialsAttrTypes = map[string]attr.Type{
	"metalake":      types.StringType,
	"resource_type": types.StringType,
	"resource":      types.StringType,
	"credentials":   credentialListType,
}

func newSchema(t *testing.T, d datasource.DataSource) schema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// readCredentials runs Read with the given config and returns the resulting state model.
func readCredentials(t *testing.T, d datasource.DataSource, configModel ds.CredentialsDataSourceModel) (*datasource.ReadResponse, ds.CredentialsDataSourceModel) {
	t.Helper()

	ctx := context.Background()
	schemaObj := newSchema(t, d)

	configModel.Credentials = types.ListNull(types.ObjectType{AttrTypes: ds.CredentialItemAttrTypes})

	configObj, diags := types.ObjectValueFrom(ctx, credentialsAttrTypes, configModel)
	if diags.HasError() {
		t.Fatalf("failed to create config object: %v", diags)
	}
	tfVal, err := configObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaObj}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaObj, Raw: tfVal}}, resp)

	var state ds.CredentialsDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			t.Fatalf("failed to read state: %v", resp.Diagnostics)
		}
	}
	return resp, state
}

func logDiagnostics(t *testing.T, resp *datasource.ReadResponse) {
	t.Helper()
	for _, diag := range resp.Diagnostics.Errors() {
		t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
	}
}

func TestCredentialsDataSource_Metadata(t *testing.T) {
	d := ds.New()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_credentials" {
		t.Fatalf("expected gravitino_credentials, got %s", resp.TypeName)
	}
}

func TestCredentialsDataSource_Schema(t *testing.T) {
	s := newSchema(t, ds.New())

	for _, name := range []string{"metalake", "resource_type", "resource", "credentials"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Fatalf("missing attribute %s", name)
		}
	}

	// The pre-v1.3.0 single-credential attributes must be gone.
	for _, gone := range []string{"type", "value", "expire_time"} {
		if _, ok := s.Attributes[gone]; ok {
			t.Errorf("attribute %s must no longer exist; the API returns a list under `credentials`", gone)
		}
	}

	credentialsAttr, ok := s.Attributes["credentials"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("credentials must be a ListNestedAttribute, got %T", s.Attributes["credentials"])
	}
	if !credentialsAttr.Computed {
		t.Error("credentials must be computed")
	}

	nested := credentialsAttr.NestedObject.Attributes
	for _, name := range []string{"credential_type", "expire_time_in_ms", "credential_info"} {
		if _, ok := nested[name]; !ok {
			t.Fatalf("missing nested attribute %s", name)
		}
	}
	if got := nested["expire_time_in_ms"].GetType(); got != types.Int64Type {
		t.Errorf("expire_time_in_ms must be Int64, got %s", got)
	}
	infoAttr, ok := nested["credential_info"].(schema.MapAttribute)
	if !ok {
		t.Fatalf("credential_info must be a MapAttribute, got %T", nested["credential_info"])
	}
	if !infoAttr.Sensitive {
		t.Error("credential_info carries secrets and must be sensitive")
	}
	if infoAttr.ElementType != types.StringType {
		t.Errorf("credential_info elements must be strings, got %s", infoAttr.ElementType)
	}

	// The resource_type enum must be exactly the metadataObjectType enum of the spec.
	resourceType, ok := s.Attributes["resource_type"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("resource_type must be a StringAttribute, got %T", s.Attributes["resource_type"])
	}
	validators := resourceType.StringValidators()
	if len(validators) == 0 {
		t.Fatal("resource_type must have an enum validator")
	}
	ctx := context.Background()
	for _, valid := range []string{"METALAKE", "CATALOG", "SCHEMA", "TABLE", "COLUMN", "FILESET", "TOPIC", "MODEL", "ROLE"} {
		vResp := &validator.StringResponse{}
		validators[0].ValidateString(ctx, validator.StringRequest{
			ConfigValue: types.StringValue(valid),
			Path:        path.Root("resource_type"),
		}, vResp)
		if vResp.Diagnostics.HasError() {
			t.Errorf("resource_type %q must be accepted: %v", valid, vResp.Diagnostics)
		}
	}
	// FUNCTION/TAG/POLICY are not part of the credentials endpoint enum.
	for _, invalid := range []string{"FUNCTION", "TAG", "POLICY", "catalog"} {
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

func TestCredentialsDataSource_ReadSpecExample(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(credentialResponseExample))
	}))
	defer server.Close()

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	d := ds.New()
	d.(*ds.CredentialsDataSource).SetClient(c)

	resp, state := readCredentials(t, d, ds.CredentialsDataSourceModel{
		Metalake:     types.StringValue("test_metalake"),
		ResourceType: types.StringValue("CATALOG"),
		Resource:     types.StringValue("test_catalog"),
	})
	if resp.Diagnostics.HasError() {
		logDiagnostics(t, resp)
		t.Fatal("unexpected diagnostics errors")
	}

	if want := "/api/metalakes/test_metalake/objects/CATALOG/test_catalog/credentials"; requestedPath != want {
		t.Fatalf("expected path %s, got %s", want, requestedPath)
	}

	elems := state.Credentials.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(elems))
	}

	first := credentialItem(t, elems, 0)
	assertStringAttr(t, first, "credential_type", "s3-token")
	assertInt64Attr(t, first, "expire_time_in_ms", 1735891948411)
	firstInfo := credentialInfo(t, first)
	assertMapEntry(t, firstInfo, "s3-access-key-id", "value1")
	assertMapEntry(t, firstInfo, "s3-secret-access-key", "value2")
	assertMapEntry(t, firstInfo, "s3-session-token", "value3")
	if len(firstInfo) != 3 {
		t.Errorf("expected 3 credential_info entries, got %d", len(firstInfo))
	}

	second := credentialItem(t, elems, 1)
	assertStringAttr(t, second, "credential_type", "s3-secret-key")
	assertInt64Attr(t, second, "expire_time_in_ms", 0)
	secondInfo := credentialInfo(t, second)
	assertMapEntry(t, secondInfo, "s3-access-key-id", "value1")
	assertMapEntry(t, secondInfo, "s3-secret-access-key", "value2")
	if len(secondInfo) != 2 {
		t.Errorf("expected 2 credential_info entries, got %d", len(secondInfo))
	}
}

func TestCredentialsDataSource_ReadEmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code": 0, "credentials": []}`))
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.New()
	d.(*ds.CredentialsDataSource).SetClient(c)

	resp, state := readCredentials(t, d, ds.CredentialsDataSourceModel{
		Metalake:     types.StringValue("test_metalake"),
		ResourceType: types.StringValue("CATALOG"),
		Resource:     types.StringValue("test_catalog"),
	})
	if resp.Diagnostics.HasError() {
		logDiagnostics(t, resp)
		t.Fatal("unexpected diagnostics errors")
	}

	if state.Credentials.IsNull() || state.Credentials.IsUnknown() {
		t.Fatalf("credentials must be a known (possibly empty) list, got %v", state.Credentials)
	}
	if n := len(state.Credentials.Elements()); n != 0 {
		t.Fatalf("expected empty credentials list, got %d entries", n)
	}
}

func TestCredentialsDataSource_ReadMissingCredentialInfo(t *testing.T) {
	// The spec documents `credentialInfo` with a `{}` default; a server that
	// omits it must still yield a known (empty) map, never null/unknown.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(`{"code":0,"credentials":[{"credentialType":"gcs-token","expireTimeInMs":0}]}`))
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.New()
	d.(*ds.CredentialsDataSource).SetClient(c)

	resp, state := readCredentials(t, d, ds.CredentialsDataSourceModel{
		Metalake:     types.StringValue("test_metalake"),
		ResourceType: types.StringValue("SCHEMA"),
		Resource:     types.StringValue("test_catalog.test_schema"),
	})
	if resp.Diagnostics.HasError() {
		logDiagnostics(t, resp)
		t.Fatal("unexpected diagnostics errors")
	}

	elems := state.Credentials.Elements()
	if len(elems) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(elems))
	}
	item := credentialItem(t, elems, 0)
	assertStringAttr(t, item, "credential_type", "gcs-token")
	info := credentialInfo(t, item)
	if len(info) != 0 {
		t.Fatalf("expected empty credential_info map, got %v", info)
	}
}

func TestCredentialsDataSource_ReadNotFound(t *testing.T) {
	// Both bodies are real Gravitino 1.3.0 responses: the first is the example
	// the spec references for this endpoint's 404 (metalakes.yaml), the second
	// was measured against a live server for a missing catalog.
	for name, body := range map[string]string{
		"NoSuchMetalakeException": noSuchMetalakeExample,
		"NoSuchCatalogException":  noSuchCatalogExample,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()

			c, _ := client.New(server.URL, nil)
			d := ds.New()
			d.(*ds.CredentialsDataSource).SetClient(c)

			resp, _ := readCredentials(t, d, ds.CredentialsDataSourceModel{
				Metalake:     types.StringValue("test_metalake"),
				ResourceType: types.StringValue("CATALOG"),
				Resource:     types.StringValue("test_catalog"),
			})

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected an error diagnostic for a 404 response")
			}
			logDiagnostics(t, resp)

			var detail strings.Builder
			for _, diag := range resp.Diagnostics.Errors() {
				detail.WriteString(diag.Detail())
				detail.WriteString("\n")
			}
			for _, want := range []string{"404", name, "does not exist"} {
				if !strings.Contains(detail.String(), want) {
					t.Errorf("diagnostics must mention %q, got:\n%s", want, detail.String())
				}
			}
			if !resp.State.Raw.IsNull() {
				t.Error("state must not be written when the API call failed")
			}
		})
	}
}

// TestGetCredentials_NotFoundBodyIsDetected proves the client classifies the
// real Gravitino 404 body as not-found (the payload carries application code
// 1003, not the HTTP status).
func TestGetCredentials_NotFoundBodyIsDetected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(noSuchMetalakeExample))
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	_, err := c.GetCredentials(context.Background(), "test_metalake", "CATALOG", "test_catalog")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !client.IsNotFoundError(err) {
		t.Fatalf("expected a not-found error, got %v", err)
	}
	if !strings.Contains(err.Error(), "NoSuchMetalakeException") {
		t.Errorf("error must carry the Gravitino error type, got %v", err)
	}
}

// TestGetCredentials_ResponseDTOMatchesSpec guards the JSON keys of the DTO.
func TestGetCredentials_ResponseDTOMatchesSpec(t *testing.T) {
	var result models.CredentialResponse
	if err := json.Unmarshal([]byte(credentialResponseExample), &result); err != nil {
		t.Fatalf("failed to decode the spec example: %v", err)
	}
	if len(result.Credentials) != 2 {
		t.Fatalf("expected 2 credentials under the `credentials` key, got %d", len(result.Credentials))
	}
	if result.Credentials[0].CredentialType != "s3-token" {
		t.Errorf("credentialType not decoded: %+v", result.Credentials[0])
	}
	if result.Credentials[0].ExpireTimeInMs != 1735891948411 {
		t.Errorf("expireTimeInMs not decoded: %+v", result.Credentials[0])
	}
	if result.Credentials[0].CredentialInfo["s3-access-key-id"] != "value1" {
		t.Errorf("credentialInfo not decoded: %+v", result.Credentials[0])
	}
}

func credentialItem(t *testing.T, elems []attr.Value, i int) map[string]attr.Value {
	t.Helper()
	if i >= len(elems) {
		t.Fatalf("index %d out of range (%d elements)", i, len(elems))
	}
	obj, ok := elems[i].(types.Object)
	if !ok {
		t.Fatalf("element %d is %T, want types.Object", i, elems[i])
	}
	if obj.IsNull() || obj.IsUnknown() {
		t.Fatalf("element %d must be a known object", i)
	}
	return obj.Attributes()
}

func credentialInfo(t *testing.T, item map[string]attr.Value) map[string]attr.Value {
	t.Helper()
	info, ok := item["credential_info"].(types.Map)
	if !ok {
		t.Fatalf("credential_info is %T, want types.Map", item["credential_info"])
	}
	if info.IsNull() || info.IsUnknown() {
		t.Fatalf("credential_info must be a known map, got %v", info)
	}
	return info.Elements()
}

func assertStringAttr(t *testing.T, item map[string]attr.Value, name, want string) {
	t.Helper()
	v, ok := item[name].(types.String)
	if !ok {
		t.Fatalf("%s is %T, want types.String", name, item[name])
	}
	if v.IsNull() || v.IsUnknown() {
		t.Fatalf("%s must be a known string, got %v", name, v)
	}
	if v.ValueString() != want {
		t.Errorf("%s = %q, want %q", name, v.ValueString(), want)
	}
}

func assertInt64Attr(t *testing.T, item map[string]attr.Value, name string, want int64) {
	t.Helper()
	v, ok := item[name].(types.Int64)
	if !ok {
		t.Fatalf("%s is %T, want types.Int64", name, item[name])
	}
	if v.IsNull() || v.IsUnknown() {
		t.Fatalf("%s must be a known int64, got %v", name, v)
	}
	if v.ValueInt64() != want {
		t.Errorf("%s = %d, want %d", name, v.ValueInt64(), want)
	}
}

func assertMapEntry(t *testing.T, m map[string]attr.Value, key, want string) {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("map entry %q missing (keys: %v)", key, m)
	}
	s, ok := v.(types.String)
	if !ok {
		t.Fatalf("map entry %q is %T, want types.String", key, v)
	}
	if s.ValueString() != want {
		t.Errorf("map entry %q = %q, want %q", key, s.ValueString(), want)
	}
}
