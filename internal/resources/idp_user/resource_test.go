package idp_user

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The JSON literals below are copied verbatim from the `components/examples`
// section of the Gravitino v1.3.0 IDP spec (docs/open-api/idp/idp.yaml).
const (
	idpAddUserRequestExample        = `{"user": "alice", "password": "Passw0rd-1234"}`
	idpAddUserResponseExample       = `{"code": 0, "user": {"name": "alice", "groups": []}}`
	idpUserGetResponseExample       = `{"code": 0, "user": {"name": "alice", "groups": ["engineers"]}}`
	idpChangePasswordRequestExample = `{"password": "Passw0rd-5678"}`
	idpUserUpdateResponseExample    = `{"code": 0, "user": {"name": "alice", "groups": ["engineers"]}}`
	// IdpNotFoundException, operation GET.
	idpUserNotFoundExample = `{"code": 1003, "type": "NotFoundException", "message": "Failed to operate built-in IdP user [missing-user] operation [GET], reason [IdP user not found: missing-user]"}`
	// The RemoveResponse payload of the DELETE endpoints.
	idpRemoveResponseExample = `{"code": 0, "removed": true}`
)

func idpUserSchema(t *testing.T, r resource.Resource) schema.Schema {
	t.Helper()

	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// newIdpUserResourceWithServer returns a resource wired to a client pointing at
// an in-memory Gravitino stand-in.
func newIdpUserResourceWithServer(t *testing.T, handler http.HandlerFunc) resource.Resource {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	r := New()
	r.(*IdpUserResource).SetClient(c)
	return r
}

func idpUserRaw(t *testing.T, s schema.Schema, model IdpUserResourceModel) tftypes.Value {
	t.Helper()

	ctx := context.Background()
	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("failed to build object: %v", diags)
	}
	raw, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert object: %v", err)
	}
	return raw
}

func idpUserState(t *testing.T, state tfsdk.State) IdpUserResourceModel {
	t.Helper()

	var model IdpUserResourceModel
	if diags := state.Get(context.Background(), &model); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return model
}

// assertJSONEquals compares two JSON documents structurally, so it also fails
// when a request carries fields the spec does not define (e.g. `enabled`).
func assertJSONEquals(t *testing.T, got, want string) {
	t.Helper()

	var gotVal, wantVal interface{}
	if err := json.Unmarshal([]byte(got), &gotVal); err != nil {
		t.Fatalf("invalid JSON %q: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("invalid JSON %q: %v", want, err)
	}
	if !reflect.DeepEqual(gotVal, wantVal) {
		t.Errorf("unexpected JSON\n got: %s\nwant: %s", got, want)
	}
}

func TestIdpUserResource_Metadata(t *testing.T) {
	r := New()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_idp_user" {
		t.Fatalf("expected gravitino_idp_user, got %s", resp.TypeName)
	}
}

func TestIdpUserResource_Schema(t *testing.T) {
	s := idpUserSchema(t, New())

	for _, name := range []string{"id", "name", "password", "groups"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Fatalf("missing attribute %s", name)
		}
	}

	// IdpUser only carries `name` and `groups`, so an `enabled` attribute could
	// never be written or read.
	if _, ok := s.Attributes["enabled"]; ok {
		t.Error("enabled must not exist: the IDP API has no enable/disable flag")
	}
	if len(s.Attributes) != 4 {
		t.Errorf("expected exactly 4 attributes, got %d", len(s.Attributes))
	}
}

func TestIdpUserResource_Create(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   string
	)

	r := newIdpUserResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)

		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(idpAddUserResponseExample))
	})

	ctx := context.Background()
	s := idpUserSchema(t, r)
	plan := IdpUserResourceModel{
		ID:       types.StringNull(),
		Name:     types.StringValue("alice"),
		Password: types.StringValue("Passw0rd-1234"),
		Groups:   types.SetNull(types.StringType),
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: idpUserRaw(t, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/api/idp/users" {
		t.Errorf("expected path /api/idp/users, got %s", gotPath)
	}
	assertJSONEquals(t, gotBody, idpAddUserRequestExample)

	state := idpUserState(t, resp.State)
	if state.ID.ValueString() != "alice" {
		t.Errorf("id = %q, want alice", state.ID.ValueString())
	}
	if state.Name.ValueString() != "alice" {
		t.Errorf("name = %q, want alice", state.Name.ValueString())
	}
	if state.Password.ValueString() != "Passw0rd-1234" {
		t.Errorf("password must be kept in state, got %q", state.Password.ValueString())
	}
	if state.Groups.IsNull() || state.Groups.IsUnknown() {
		t.Fatalf("groups must be a known set, got %v", state.Groups)
	}
	if n := len(state.Groups.Elements()); n != 0 {
		t.Errorf("expected 0 groups, got %d", n)
	}
}

func TestIdpUserResource_Read(t *testing.T) {
	var gotPath string

	r := newIdpUserResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(idpUserGetResponseExample))
	})

	ctx := context.Background()
	s := idpUserSchema(t, r)
	state := IdpUserResourceModel{
		ID:       types.StringValue("alice"),
		Name:     types.StringValue("alice"),
		Password: types.StringValue("Passw0rd-1234"),
		Groups:   types.SetNull(types.StringType),
	}

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: idpUserRaw(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if gotPath != "/api/idp/users/alice" {
		t.Errorf("expected path /api/idp/users/alice, got %s", gotPath)
	}

	got := idpUserState(t, resp.State)
	if got.ID.ValueString() != "alice" {
		t.Errorf("id = %q, want alice", got.ID.ValueString())
	}
	groups := got.Groups.Elements()
	if len(groups) != 1 || groups[0].(types.String).ValueString() != "engineers" {
		t.Errorf("groups = %v, want [engineers]", groups)
	}
	if got.Password.ValueString() != "Passw0rd-1234" {
		t.Errorf("password must be preserved by Read, got %q", got.Password.ValueString())
	}
}

func TestIdpUserResource_ReadNotFound(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			// IdpNotFoundException of the spec: the 404 payload carries
			// application code 1003, not the HTTP status.
			name: "gravitino error body",
			body: idpUserNotFoundExample,
		},
		{
			// A Gravitino without the idp-basic plugin answers with an empty
			// Jetty 404 because the route does not exist at all.
			name: "empty body",
			body: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newIdpUserResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(tc.body))
			})

			ctx := context.Background()
			s := idpUserSchema(t, r)
			state := IdpUserResourceModel{
				ID:       types.StringValue("missing-user"),
				Name:     types.StringValue("missing-user"),
				Password: types.StringValue("Passw0rd-1234"),
				Groups:   types.SetNull(types.StringType),
			}

			resp := &resource.ReadResponse{State: tfsdk.State{Schema: s}}
			r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: idpUserRaw(t, s, state)}}, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
			if !resp.State.Raw.IsNull() {
				t.Error("a 404 must remove the resource from state")
			}
		})
	}
}

func TestIdpUserResource_UpdateChangesPassword(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   string
	)

	r := newIdpUserResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)

		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(idpUserUpdateResponseExample))
	})

	ctx := context.Background()
	s := idpUserSchema(t, r)

	state := IdpUserResourceModel{
		ID:       types.StringValue("alice"),
		Name:     types.StringValue("alice"),
		Password: types.StringValue("Passw0rd-1234"),
		Groups:   types.SetNull(types.StringType),
	}
	plan := IdpUserResourceModel{
		ID:       types.StringValue("alice"),
		Name:     types.StringValue("alice"),
		Password: types.StringValue("Passw0rd-5678"),
		Groups:   types.SetNull(types.StringType),
	}

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: idpUserRaw(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: idpUserRaw(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
	if gotPath != "/api/idp/users/alice" {
		t.Errorf("expected path /api/idp/users/alice, got %s", gotPath)
	}
	// PUT /idp/users/{user} accepts the password and nothing else.
	assertJSONEquals(t, gotBody, idpChangePasswordRequestExample)

	got := idpUserState(t, resp.State)
	if got.Password.ValueString() != "Passw0rd-5678" {
		t.Errorf("password = %q, want Passw0rd-5678", got.Password.ValueString())
	}
	groups := got.Groups.Elements()
	if len(groups) != 1 || groups[0].(types.String).ValueString() != "engineers" {
		t.Errorf("groups = %v, want [engineers] from the update response", groups)
	}
}

func TestIdpUserResource_UpdateWithoutPasswordChange(t *testing.T) {
	var called bool

	r := newIdpUserResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code": 1001, "type": "RuntimeException", "message": "must not be called"}`))
	})

	ctx := context.Background()
	s := idpUserSchema(t, r)

	state := IdpUserResourceModel{
		ID:       types.StringValue("alice"),
		Name:     types.StringValue("alice"),
		Password: types.StringValue("Passw0rd-1234"),
		Groups:   types.SetNull(types.StringType),
	}

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: idpUserRaw(t, s, state)},
		State: tfsdk.State{Schema: s, Raw: idpUserRaw(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if called {
		t.Error("an unchanged password must not trigger a request")
	}
}

func TestIdpUserResource_Delete(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
	)

	r := newIdpUserResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(idpRemoveResponseExample))
	})

	ctx := context.Background()
	s := idpUserSchema(t, r)
	state := IdpUserResourceModel{
		ID:       types.StringValue("alice"),
		Name:     types.StringValue("alice"),
		Password: types.StringValue("Passw0rd-1234"),
		Groups:   types.SetNull(types.StringType),
	}

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: idpUserRaw(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if gotPath != "/api/idp/users/alice" {
		t.Errorf("expected path /api/idp/users/alice, got %s", gotPath)
	}
}

func TestIdpUserResource_DeleteNotFoundIsSuccess(t *testing.T) {
	r := newIdpUserResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(idpUserNotFoundExample))
	})

	ctx := context.Background()
	s := idpUserSchema(t, r)
	state := IdpUserResourceModel{
		ID:       types.StringValue("alice"),
		Name:     types.StringValue("alice"),
		Password: types.StringValue("Passw0rd-1234"),
		Groups:   types.SetNull(types.StringType),
	}

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: idpUserRaw(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("deleting an already deleted user must succeed: %v", resp.Diagnostics)
	}
}

func TestIdpUserResource_ImportState(t *testing.T) {
	r := New().(resource.ResourceWithImportState)

	ctx := context.Background()
	s := idpUserSchema(t, New())

	nullRaw := idpUserRaw(t, s, IdpUserResourceModel{
		ID:       types.StringNull(),
		Name:     types.StringNull(),
		Password: types.StringNull(),
		Groups:   types.SetNull(types.StringType),
	})
	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: nullRaw}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "alice"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var name types.String
	if diags := resp.State.GetAttribute(ctx, path.Root("name"), &name); diags.HasError() {
		t.Fatalf("failed to read imported name: %v", diags)
	}
	if name.ValueString() != "alice" {
		t.Errorf("imported name = %q, want alice", name.ValueString())
	}
}

func TestIdpUserResource_ImportState_Invalid(t *testing.T) {
	r := New().(resource.ResourceWithImportState)

	for _, id := range []string{"", "bad:name"} {
		t.Run(id, func(t *testing.T) {
			ctx := context.Background()
			s := idpUserSchema(t, New())

			nullRaw := idpUserRaw(t, s, IdpUserResourceModel{
				ID:       types.StringNull(),
				Name:     types.StringNull(),
				Password: types.StringNull(),
				Groups:   types.SetNull(types.StringType),
			})
			resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: nullRaw}}
			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error diagnostic for import identifier %q", id)
			}
		})
	}
}

// TestIdpUserDTOsMatchSpec guards the client DTOs against the spec examples.
func TestIdpUserDTOsMatchSpec(t *testing.T) {
	var addRequest models.IdpAddUserRequest
	if err := json.Unmarshal([]byte(idpAddUserRequestExample), &addRequest); err != nil {
		t.Fatalf("failed to decode AddUserRequest example: %v", err)
	}
	if addRequest.User != "alice" || addRequest.Password != "Passw0rd-1234" {
		t.Errorf("unexpected AddUserRequest: %+v", addRequest)
	}

	var changeRequest models.IdpChangePasswordRequest
	if err := json.Unmarshal([]byte(idpChangePasswordRequestExample), &changeRequest); err != nil {
		t.Fatalf("failed to decode ChangePasswordRequest example: %v", err)
	}
	if changeRequest.Password != "Passw0rd-5678" {
		t.Errorf("unexpected ChangePasswordRequest: %+v", changeRequest)
	}

	var addResponse models.IdpUserResponse
	if err := json.Unmarshal([]byte(idpAddUserResponseExample), &addResponse); err != nil {
		t.Fatalf("failed to decode IdpUserAddResponse example: %v", err)
	}
	if addResponse.Code != 0 || addResponse.User.Name != "alice" || len(addResponse.User.Groups) != 0 {
		t.Errorf("unexpected IdpUserAddResponse: %+v", addResponse)
	}

	var getResponse models.IdpUserResponse
	if err := json.Unmarshal([]byte(idpUserGetResponseExample), &getResponse); err != nil {
		t.Fatalf("failed to decode IdpUserGetResponse example: %v", err)
	}
	if getResponse.User.Name != "alice" || len(getResponse.User.Groups) != 1 || getResponse.User.Groups[0] != "engineers" {
		t.Errorf("unexpected IdpUserGetResponse: %+v", getResponse)
	}
}
