package policy_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/policy"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPolicyResource_Schema(t *testing.T) {
	r := res.New()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.TODO(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_policy" {
		t.Fatalf("expected gravitino_policy, got %s", resp.TypeName)
	}
}

func TestPolicyResource_Create(t *testing.T) {
	var receivedBody models.PolicyCreateRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)

		var content *models.PolicyContent
		if receivedBody.Content != nil {
			content = &models.PolicyContent{
				SupportedObjectTypes: receivedBody.Content.SupportedObjectTypes,
				Properties:           receivedBody.Content.Properties,
				CustomRules:          receivedBody.Content.CustomRules,
			}
		}

		resp := models.PolicyResponse{
			Code: 0,
			Policy: models.Policy{
				Name:       receivedBody.Name,
				Comment:    receivedBody.Comment,
				PolicyType: receivedBody.PolicyType,
				Enabled:    receivedBody.Enabled,
				Content:    content,
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.New()
	r.(*res.PolicyResource).SetClient(c)

	ctx := context.Background()

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	planModel := res.PolicyResourceModel{
		Metalake:   types.StringValue("test_metalake"),
		Name:       types.StringValue("test_policy"),
		Comment:    types.StringValue("test comment"),
		PolicyType: types.StringValue("custom"),
		Enabled:    types.BoolValue(true),
		SupportedObjectTypes: types.SetValueMust(types.StringType, []attr.Value{
			types.StringValue("SCHEMA"),
			types.StringValue("TABLE"),
		}),
		Properties:  types.MapNull(types.StringType),
		CustomRules: types.MapNull(types.StringType),
		Audit:       types.ObjectNull(res.AuditAttrTypes),
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
	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaObj},
	}

	r.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	if receivedBody.Name != "test_policy" {
		t.Errorf("expected name test_policy, got %s", receivedBody.Name)
	}
	if receivedBody.PolicyType != "custom" {
		t.Errorf("expected policy_type custom, got %s", receivedBody.PolicyType)
	}
	if receivedBody.Content == nil {
		t.Fatal("expected content in request")
	}
	if len(receivedBody.Content.SupportedObjectTypes) != 2 {
		t.Errorf("expected 2 supported object types, got %d", len(receivedBody.Content.SupportedObjectTypes))
	}
}

func TestPolicyResource_ImportState(t *testing.T) {
	r := res.New().(resource.ResourceWithImportState)

	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.(resource.Resource).Schema(ctx, resource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	nullModel := res.PolicyResourceModel{
		Metalake:             types.StringNull(),
		Name:                 types.StringNull(),
		Comment:              types.StringNull(),
		PolicyType:           types.StringNull(),
		Enabled:              types.BoolNull(),
		SupportedObjectTypes: types.SetNull(types.StringType),
		Properties:           types.MapNull(types.StringType),
		CustomRules:          types.MapNull(types.StringType),
		Audit:                types.ObjectNull(res.AuditAttrTypes),
	}
	nullObj, diags := types.ObjectValueFrom(ctx, schemaObj.Type().(types.ObjectType).AttributeTypes(), nullModel)
	if diags.HasError() {
		t.Fatalf("failed to create null object: %v", diags)
	}
	rawVal, err := nullObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}

	req := resource.ImportStateRequest{
		ID: "my_metalake.my_policy",
	}
	resp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaObj, Raw: rawVal},
	}

	r.ImportState(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}
}

func TestPolicyResource_ImportState_Invalid(t *testing.T) {
	r := res.New().(resource.ResourceWithImportState)

	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.(resource.Resource).Schema(ctx, resource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	nullModel := res.PolicyResourceModel{
		Metalake:             types.StringNull(),
		Name:                 types.StringNull(),
		Comment:              types.StringNull(),
		PolicyType:           types.StringNull(),
		Enabled:              types.BoolNull(),
		SupportedObjectTypes: types.SetNull(types.StringType),
		Properties:           types.MapNull(types.StringType),
		CustomRules:          types.MapNull(types.StringType),
		Audit:                types.ObjectNull(res.AuditAttrTypes),
	}
	nullObj, diags := types.ObjectValueFrom(ctx, schemaObj.Type().(types.ObjectType).AttributeTypes(), nullModel)
	if diags.HasError() {
		t.Fatalf("failed to create null object: %v", diags)
	}
	rawVal, err := nullObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}

	req := resource.ImportStateRequest{
		ID: "no_dots_here",
	}
	resp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaObj, Raw: rawVal},
	}

	r.ImportState(ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid import ID")
	}
}

func TestPolicyResource_Delete(t *testing.T) {
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
	r := res.New()
	r.(*res.PolicyResource).SetClient(c)

	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	stateModel := res.PolicyResourceModel{
		ID:                   types.StringValue("test_metalake.test_policy"),
		Metalake:             types.StringValue("test_metalake"),
		Name:                 types.StringValue("test_policy"),
		Comment:              types.StringNull(),
		PolicyType:           types.StringValue("custom"),
		Enabled:              types.BoolValue(true),
		SupportedObjectTypes: types.SetValueMust(types.StringType, []attr.Value{types.StringValue("SCHEMA")}),
		Properties:           types.MapNull(types.StringType),
		CustomRules:          types.MapNull(types.StringType),
		Audit:                types.ObjectNull(res.AuditAttrTypes),
	}

	stateObj, diags := types.ObjectValueFrom(ctx, schemaObj.Type().(types.ObjectType).AttributeTypes(), stateModel)
	if diags.HasError() {
		t.Fatalf("failed to create state object: %v", diags)
	}

	tfVal, err := stateObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}

	req := resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaObj, Raw: tfVal},
	}
	resp := &resource.DeleteResponse{
		State: tfsdk.State{Schema: schemaObj},
	}

	r.Delete(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}

	if !deleteCalled {
		t.Fatal("delete was not called")
	}
}
