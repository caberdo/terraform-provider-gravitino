package owner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/owner"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The exception payloads below are verbatim examples of owners.yaml
// (components/examples/NoSuchMetadataObjectException) / openapi.yaml, returned
// by Gravitino with HTTP status 404.
const noSuchMetadataObjectError = `{
  "code": 1003,
  "type": "NoSuchMetadataObjectException",
  "message": "Metadata object does not exist",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchUserException: Metadata object does not exist",
    "..."
  ]
}`

// ownerResponse is the OwnerResponse example of owners.yaml. Gravitino answers
// the owner type in lowercase, even when it was set as "USER".
const ownerResponse = `{"code":0,"owner":{"name":"user1","type":"user"}}`

var nonNullRaw = tftypes.NewValue(tftypes.String, "state")

func ownerSchema(t *testing.T) schema.Schema {
	t.Helper()
	r := res.NewOwnerResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func tfValue(t *testing.T, ctx context.Context, s schema.Schema, model res.OwnerResourceModel) tftypes.Value {
	t.Helper()
	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("failed to build object value: %v", diags)
	}
	v, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}
	return v
}

func baseModel() res.OwnerResourceModel {
	return res.OwnerResourceModel{
		Metalake:       types.StringValue("authz_ml"),
		ObjectType:     types.StringValue("CATALOG"),
		ObjectFullName: types.StringValue("c1"),
		OwnerName:      types.StringValue("user1"),
		OwnerType:      types.StringValue("USER"),
	}
}

func stringPlan(ctx context.Context, attr schema.StringAttribute, state, plan types.String) (types.String, bool) {
	req := planmodifier.StringRequest{
		State:       tfsdk.State{Raw: nonNullRaw},
		Plan:        tfsdk.Plan{Raw: nonNullRaw},
		Config:      tfsdk.Config{Raw: nonNullRaw},
		StateValue:  state,
		PlanValue:   plan,
		ConfigValue: plan,
	}
	resp := &planmodifier.StringResponse{PlanValue: plan}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyString(ctx, req, resp)
	}
	return resp.PlanValue, resp.RequiresReplace
}

func TestOwnerResource_MetadataAndSchema(t *testing.T) {
	r := res.NewOwnerResource()
	meta := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, meta)
	if meta.TypeName != "gravitino_owner" {
		t.Fatalf("expected gravitino_owner, got %s", meta.TypeName)
	}
}

// TestOwnerResource_SchemaUpdatePolicy pins the update policy: the owner
// binding cannot be moved between objects (owners.yaml only provides
// PUT /owners/{type}/{fullName}), so the object identity forces a replacement,
// while owner_name/owner_type are applied in place.
func TestOwnerResource_SchemaUpdatePolicy(t *testing.T) {
	ctx := context.Background()
	s := ownerSchema(t)

	for _, name := range []string{"metalake", "object_type", "object_full_name"} {
		attr := s.Attributes[name].(schema.StringAttribute)
		if _, requiresReplace := stringPlan(ctx, attr, types.StringValue("old"), types.StringValue("new")); !requiresReplace {
			t.Fatalf("%s: changing the owner binding target must force a replacement", name)
		}
	}

	for _, name := range []string{"owner_name", "owner_type"} {
		attr := s.Attributes[name].(schema.StringAttribute)
		if _, requiresReplace := stringPlan(ctx, attr, types.StringValue("old"), types.StringValue("new")); requiresReplace {
			t.Fatalf("%s: must be updateable in place via SetOwnerRequest", name)
		}
	}
}

func TestOwnerResource_Create(t *testing.T) {
	ctx := context.Background()
	s := ownerSchema(t)

	var method, requestPath string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, requestPath = r.Method, r.URL.Path
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{"code":0,"set":true}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewOwnerResource()
	r.(*res.OwnerResource).SetClient(c)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, baseModel())}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if method != http.MethodPut {
		t.Fatalf("expected PUT, got %s", method)
	}
	if want := "/api/metalakes/authz_ml/owners/CATALOG/c1"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}
	// OwnerSetRequest of owners.yaml is exactly {name, type}.
	wantBody := `{"name":"user1","type":"USER"}`
	gotBody, _ := json.Marshal(body)
	if string(gotBody) != wantBody {
		t.Fatalf("unexpected request body: want %s, got %s", wantBody, gotBody)
	}

	var state res.OwnerResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.ID.ValueString() != "authz_ml.CATALOG.c1" {
		t.Fatalf("unexpected id: %s", state.ID.ValueString())
	}
}

// TestOwnerResource_ReadNormalizesOwnerType covers the empirically measured
// behaviour that Gravitino answers with a lowercase owner type while the
// configuration (and the API enum) uses "USER"/"GROUP". Without normalisation
// the apply would fail with "Provider produced inconsistent result".
func TestOwnerResource_ReadNormalizesOwnerType(t *testing.T) {
	ctx := context.Background()
	s := ownerSchema(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, ownerResponse)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewOwnerResource()
	r.(*res.OwnerResource).SetClient(c)

	state := baseModel()
	state.ID = types.StringValue("authz_ml.CATALOG.c1")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var newState res.OwnerResourceModel
	if diags := resp.State.Get(ctx, &newState); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if newState.OwnerType.ValueString() != models.OwnerTypeUser {
		t.Fatalf("expected owner_type to be normalised to %s, got %s", models.OwnerTypeUser, newState.OwnerType.ValueString())
	}
	if newState.OwnerName.ValueString() != "user1" {
		t.Fatalf("unexpected owner_name: %s", newState.OwnerName.ValueString())
	}
}

func TestOwnerResource_ReadNotFoundRemovesState(t *testing.T) {
	ctx := context.Background()
	s := ownerSchema(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchMetadataObjectError)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewOwnerResource()
	r.(*res.OwnerResource).SetClient(c)

	state := baseModel()
	state.ID = types.StringValue("authz_ml.CATALOG.c1")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 must not produce an error, got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected the owner binding to be removed from state on 404")
	}
}

func TestOwnerResource_UpdateSendsNewOwner(t *testing.T) {
	ctx := context.Background()
	s := ownerSchema(t)

	var method, requestPath string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, requestPath = r.Method, r.URL.Path
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{"code":0,"set":true}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewOwnerResource()
	r.(*res.OwnerResource).SetClient(c)

	state := baseModel()
	state.ID = types.StringValue("authz_ml.CATALOG.c1")

	plan := baseModel()
	plan.OwnerName = types.StringValue("analyst")
	plan.OwnerType = types.StringValue("GROUP")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if method != http.MethodPut || requestPath != "/api/metalakes/authz_ml/owners/CATALOG/c1" {
		t.Fatalf("unexpected request %s %s", method, requestPath)
	}
	wantBody := `{"name":"analyst","type":"GROUP"}`
	gotBody, _ := json.Marshal(body)
	if string(gotBody) != wantBody {
		t.Fatalf("unexpected request body: want %s, got %s", wantBody, gotBody)
	}
}

// TestOwnerResource_DeleteIsStateOnly documents that Gravitino has no
// "unset owner" endpoint (owners.yaml only provides PUT and GET), so deleting
// the Terraform resource only drops the binding from state and must not call
// the API.
func TestOwnerResource_DeleteIsStateOnly(t *testing.T) {
	ctx := context.Background()
	s := ownerSchema(t)

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewOwnerResource()
	r.(*res.OwnerResource).SetClient(c)

	state := baseModel()
	state.ID = types.StringValue("authz_ml.CATALOG.c1")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if called {
		t.Fatal("owner deletion must not call the API: there is no remove-owner endpoint")
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected the resource to be removed from state")
	}
}

func TestOwnerResource_ImportState(t *testing.T) {
	ctx := context.Background()
	s := ownerSchema(t)
	r := res.NewOwnerResource().(resource.ResourceWithImportState)

	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, baseModel())}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "authz_ml.CATALOG.c1.schema1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state res.OwnerResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.Metalake.ValueString() != "authz_ml" {
		t.Fatalf("unexpected metalake: %s", state.Metalake.ValueString())
	}
	if state.ObjectType.ValueString() != "CATALOG" {
		t.Fatalf("unexpected object_type: %s", state.ObjectType.ValueString())
	}
	// object_full_name itself contains dots and must not be split further.
	if state.ObjectFullName.ValueString() != "c1.schema1" {
		t.Fatalf("unexpected object_full_name: %s", state.ObjectFullName.ValueString())
	}
}

func TestOwnerResource_ImportState_Invalid(t *testing.T) {
	ctx := context.Background()
	s := ownerSchema(t)
	r := res.NewOwnerResource().(resource.ResourceWithImportState)

	for _, id := range []string{
		"only_two.parts",
		"",
		"..",
		"authz_ml..c1",
		"authz_ml.catalog.c1", // object type must use the canonical upper case form
	} {
		resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tfValue(t, ctx, s, baseModel())}}
		r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected an error for invalid import ID %q", id)
		}
	}
}

// TestOwnerResource_CreateSchemaPathKeepsRelativeFullName: the API expects the
// full name relative to the metalake ("catalog.schema"), so the dotted value
// must be passed through unchanged (a metalake prefix would be rejected with
// HTTP 400 IllegalNamespaceException).
func TestOwnerResource_CreateSchemaPathKeepsRelativeFullName(t *testing.T) {
	ctx := context.Background()
	s := ownerSchema(t)

	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, `{"code":0,"set":true}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	r := res.NewOwnerResource()
	r.(*res.OwnerResource).SetClient(c)

	plan := baseModel()
	plan.ObjectType = types.StringValue("SCHEMA")
	plan.ObjectFullName = types.StringValue("c1.schema1")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: tfValue(t, ctx, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if want := "/api/metalakes/authz_ml/owners/SCHEMA/c1.schema1"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}
}
