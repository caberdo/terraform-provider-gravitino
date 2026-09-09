package metalake_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/metalake"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMetalakeResource_Schema(t *testing.T) {
	r := res.NewMetalakeResource()
	resp := &resource.MetadataResponse{}
	r.(interface {
		Metadata(context.Context, resource.MetadataRequest, *resource.MetadataResponse)
	}).Metadata(context.TODO(), resource.MetadataRequest{ProviderTypeName: "gravitino"}, resp)
	if resp.TypeName != "gravitino_metalake" {
		t.Fatalf("expected gravitino_metalake, got %s", resp.TypeName)
	}
}

func TestMetalakeResource_Create(t *testing.T) {
	var receivedBody models.MetalakeCreateRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		json.NewDecoder(r.Body).Decode(&receivedBody)
		resp := models.MetalakeResponse{
			Code: 0,
			Metalake: models.Metalake{
				Name:       receivedBody.Name,
				Comment:    receivedBody.Comment,
				Properties: receivedBody.Properties,
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewMetalakeResource()
	r.(*res.MetalakeResource).SetClient(c)

	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	propsMap, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"key1": "val1"})

	planModel := res.MetalakeResourceModel{
		Name:       types.StringValue("test_ml"),
		Comment:    types.StringValue("test comment"),
		Properties: propsMap,
		Audit:      types.ObjectNull(models.AuditAttrTypes),
	}

	planObj, diags := types.ObjectValueFrom(ctx, schemaObj.Type().(types.ObjectType).AttributeTypes(), planModel)
	if diags.HasError() {
		t.Fatalf("failed to create plan object: %v", diags)
	}
	tfVal, err := planObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}

	req := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schemaObj, Raw: tfVal},
	}
	respVal := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaObj},
	}

	r.Create(ctx, req, respVal)

	if respVal.Diagnostics.HasError() {
		for _, d := range respVal.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", d.Summary(), d.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	if receivedBody.Name != "test_ml" {
		t.Errorf("expected name test_ml, got %s", receivedBody.Name)
	}
}

func TestMetalakeResource_Delete(t *testing.T) {
	var deleteCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalled = true
		}
		resp := models.DropResponse{Code: 0, Dropped: true}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewMetalakeResource()
	r.(*res.MetalakeResource).SetClient(c)

	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	stateModel := res.MetalakeResourceModel{
		ID:         types.StringValue("test_ml"),
		Name:       types.StringValue("test_ml"),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(models.AuditAttrTypes),
	}

	stateObj, diags := types.ObjectValueFrom(ctx, schemaObj.Type().(types.ObjectType).AttributeTypes(), stateModel)
	if diags.HasError() {
		t.Fatalf("failed to create state object: %v", diags)
	}
	tfVal, err := stateObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert: %v", err)
	}

	req := resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaObj, Raw: tfVal},
	}
	respVal := &resource.DeleteResponse{
		State: tfsdk.State{Schema: schemaObj},
	}

	r.Delete(ctx, req, respVal)

	if respVal.Diagnostics.HasError() {
		for _, d := range respVal.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", d.Summary(), d.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	if !deleteCalled {
		t.Fatal("delete was not called")
	}
}

func TestMetalakeResource_Import(t *testing.T) {
	r := res.NewMetalakeResource().(resource.ResourceWithImportState)

	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.(resource.Resource).Schema(ctx, resource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	nullModel := res.MetalakeResourceModel{
		Name:       types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(models.AuditAttrTypes),
	}
	nullObj, diags := types.ObjectValueFrom(ctx, schemaObj.Type().(types.ObjectType).AttributeTypes(), nullModel)
	if diags.HasError() {
		t.Fatalf("failed to create null object: %v", diags)
	}
	rawVal, err := nullObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert: %v", err)
	}

	req := resource.ImportStateRequest{ID: "my_metalake"}
	respVal := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaObj, Raw: rawVal},
	}

	r.ImportState(ctx, req, respVal)

	if respVal.Diagnostics.HasError() {
		for _, d := range respVal.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", d.Summary(), d.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}
}
