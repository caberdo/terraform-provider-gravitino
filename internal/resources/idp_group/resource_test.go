package idp_group

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
	idpAddGroupRequestExample     = `{"group": "engineers"}`
	idpAddGroupResponseExample    = `{"code": 0, "group": {"name": "engineers", "users": []}}`
	idpGroupGetResponseExample    = `{"code": 0, "group": {"name": "engineers", "users": ["alice", "bob"]}}`
	idpGroupMembershipRequest     = `{"usersToAdd": ["alice", "bob"], "usersToRemove": ["carol"]}`
	idpGroupMembershipResponse    = `{"code": 0, "group": {"name": "engineers", "users": ["alice", "bob"]}}`
	idpGroupMembershipAddOnlyBody = `{"usersToAdd": ["bob"]}`
	// IdpGroupNotFoundException, operation GET.
	idpGroupNotFoundExample = `{"code": 1003, "type": "NotFoundException", "message": "Failed to operate built-in IdP group [missing-group] operation [GET], reason [IdP group not found: missing-group]"}`
	// The RemoveResponse payload of the DELETE endpoints.
	idpRemoveResponseExample = `{"code": 0, "removed": true}`
)

func idpGroupSchema(t *testing.T, r resource.Resource) schema.Schema {
	t.Helper()

	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func newIdpGroupResourceWithServer(t *testing.T, handler http.HandlerFunc) resource.Resource {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	r := New()
	r.(*IdpGroupResource).SetClient(c)
	return r
}

func idpGroupRaw(t *testing.T, s schema.Schema, model IdpGroupResourceModel) tftypes.Value {
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

func idpGroupState(t *testing.T, state tfsdk.State) IdpGroupResourceModel {
	t.Helper()

	var model IdpGroupResourceModel
	if diags := state.Get(context.Background(), &model); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return model
}

func idpGroupUsers(t *testing.T, set types.Set) []string {
	t.Helper()

	users := make([]string, 0, len(set.Elements()))
	for _, v := range set.Elements() {
		s, ok := v.(types.String)
		if !ok {
			t.Fatalf("unexpected element type %T", v)
		}
		users = append(users, s.ValueString())
	}
	return users
}

// assertJSONEquals compares two JSON documents structurally, so it also fails
// when a request carries fields the spec does not define (e.g. `comment`).
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

func TestIdpGroupResource_Metadata(t *testing.T) {
	r := New()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_idp_group" {
		t.Fatalf("expected gravitino_idp_group, got %s", resp.TypeName)
	}
}

func TestIdpGroupResource_Schema(t *testing.T) {
	s := idpGroupSchema(t, New())

	for _, name := range []string{"id", "name", "users"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Fatalf("missing attribute %s", name)
		}
	}

	// IdpGroup only carries `name` and `users`: the API has no comment field.
	if _, ok := s.Attributes["comment"]; ok {
		t.Error("comment must not exist: the IDP API has no group comment")
	}
	if len(s.Attributes) != 3 {
		t.Errorf("expected exactly 3 attributes, got %d", len(s.Attributes))
	}
}

func TestIdpGroupResource_CreateWithMembers(t *testing.T) {
	type request struct {
		method string
		path   string
		body   string
	}
	var requests []request

	r := newIdpGroupResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, request{method: r.Method, path: r.URL.Path, body: string(body)})

		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(idpAddGroupResponseExample))
		default:
			_, _ = w.Write([]byte(idpGroupMembershipResponse))
		}
	})

	ctx := context.Background()
	s := idpGroupSchema(t, r)
	users, _ := types.SetValueFrom(ctx, types.StringType, []string{"alice", "bob"})
	plan := IdpGroupResourceModel{
		ID:    types.StringNull(),
		Name:  types.StringValue("engineers"),
		Users: users,
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: idpGroupRaw(t, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if len(requests) != 2 {
		t.Fatalf("expected a group create followed by a membership change, got %d requests", len(requests))
	}
	if requests[0].method != http.MethodPost || requests[0].path != "/api/idp/groups" {
		t.Errorf("unexpected create request %s %s", requests[0].method, requests[0].path)
	}
	assertJSONEquals(t, requests[0].body, idpAddGroupRequestExample)

	if requests[1].method != http.MethodPut || requests[1].path != "/api/idp/groups/engineers/users" {
		t.Errorf("unexpected membership request %s %s", requests[1].method, requests[1].path)
	}
	assertJSONEquals(t, requests[1].body, `{"usersToAdd": ["alice", "bob"]}`)

	state := idpGroupState(t, resp.State)
	if state.ID.ValueString() != "engineers" {
		t.Errorf("id = %q, want engineers", state.ID.ValueString())
	}
	if got := idpGroupUsers(t, state.Users); !reflect.DeepEqual(got, []string{"alice", "bob"}) {
		t.Errorf("users = %v, want [alice bob]", got)
	}
}

func TestIdpGroupResource_CreateWithoutMembers(t *testing.T) {
	var requests int

	r := newIdpGroupResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(idpAddGroupResponseExample))
	})

	ctx := context.Background()
	s := idpGroupSchema(t, r)
	plan := IdpGroupResourceModel{
		ID:    types.StringNull(),
		Name:  types.StringValue("engineers"),
		Users: types.SetNull(types.StringType),
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: idpGroupRaw(t, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if requests != 1 {
		t.Errorf("expected exactly one request (no membership change), got %d", requests)
	}

	state := idpGroupState(t, resp.State)
	if state.Users.IsNull() || state.Users.IsUnknown() {
		t.Fatalf("users must be a known set, got %v", state.Users)
	}
	if n := len(state.Users.Elements()); n != 0 {
		t.Errorf("expected 0 users, got %d", n)
	}
}

func TestIdpGroupResource_Read(t *testing.T) {
	var gotPath string

	r := newIdpGroupResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(idpGroupGetResponseExample))
	})

	ctx := context.Background()
	s := idpGroupSchema(t, r)
	state := IdpGroupResourceModel{
		ID:    types.StringValue("engineers"),
		Name:  types.StringValue("engineers"),
		Users: types.SetNull(types.StringType),
	}

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: idpGroupRaw(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if gotPath != "/api/idp/groups/engineers" {
		t.Errorf("expected path /api/idp/groups/engineers, got %s", gotPath)
	}

	got := idpGroupState(t, resp.State)
	if users := idpGroupUsers(t, got.Users); !reflect.DeepEqual(users, []string{"alice", "bob"}) {
		t.Errorf("users = %v, want [alice bob]", users)
	}
}

func TestIdpGroupResource_ReadNotFound(t *testing.T) {
	r := newIdpGroupResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(idpGroupNotFoundExample))
	})

	ctx := context.Background()
	s := idpGroupSchema(t, r)
	state := IdpGroupResourceModel{
		ID:    types.StringValue("missing-group"),
		Name:  types.StringValue("missing-group"),
		Users: types.SetNull(types.StringType),
	}

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: idpGroupRaw(t, s, state)}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a 404 must remove the resource from state")
	}
}

func TestIdpGroupResource_UpdateMembership(t *testing.T) {
	tests := []struct {
		name     string
		state    []string
		plan     []string
		wantBody string
	}{
		{
			name:     "adds and removes",
			state:    []string{"carol"},
			plan:     []string{"alice", "bob"},
			wantBody: idpGroupMembershipRequest,
		},
		{
			name:     "adds only",
			state:    []string{"alice"},
			plan:     []string{"alice", "bob"},
			wantBody: idpGroupMembershipAddOnlyBody,
		},
		{
			name:     "removes only",
			state:    []string{"alice", "bob"},
			plan:     []string{"alice"},
			wantBody: `{"usersToRemove": ["bob"]}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				gotMethod string
				gotPath   string
				gotBody   string
			)

			r := newIdpGroupResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				body, _ := io.ReadAll(r.Body)
				gotBody = string(body)

				w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
				_, _ = w.Write([]byte(idpGroupMembershipResponse))
			})

			ctx := context.Background()
			s := idpGroupSchema(t, r)

			stateUsers, _ := types.SetValueFrom(ctx, types.StringType, tc.state)
			planUsers, _ := types.SetValueFrom(ctx, types.StringType, tc.plan)
			state := IdpGroupResourceModel{
				ID:    types.StringValue("engineers"),
				Name:  types.StringValue("engineers"),
				Users: stateUsers,
			}
			plan := IdpGroupResourceModel{
				ID:    types.StringValue("engineers"),
				Name:  types.StringValue("engineers"),
				Users: planUsers,
			}

			resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
			r.Update(ctx, resource.UpdateRequest{
				Plan:  tfsdk.Plan{Schema: s, Raw: idpGroupRaw(t, s, plan)},
				State: tfsdk.State{Schema: s, Raw: idpGroupRaw(t, s, state)},
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}

			if gotMethod != http.MethodPut {
				t.Errorf("expected PUT, got %s", gotMethod)
			}
			if gotPath != "/api/idp/groups/engineers/users" {
				t.Errorf("expected path /api/idp/groups/engineers/users, got %s", gotPath)
			}
			assertJSONEquals(t, gotBody, tc.wantBody)

			got := idpGroupState(t, resp.State)
			if users := idpGroupUsers(t, got.Users); !reflect.DeepEqual(users, []string{"alice", "bob"}) {
				t.Errorf("users = %v, want [alice bob] from the membership response", users)
			}
		})
	}
}

func TestIdpGroupResource_UpdateWithoutChange(t *testing.T) {
	var called bool

	r := newIdpGroupResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code": 1001, "type": "RuntimeException", "message": "must not be called"}`))
	})

	ctx := context.Background()
	s := idpGroupSchema(t, r)
	users, _ := types.SetValueFrom(ctx, types.StringType, []string{"alice", "bob"})
	state := IdpGroupResourceModel{
		ID:    types.StringValue("engineers"),
		Name:  types.StringValue("engineers"),
		Users: users,
	}

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: idpGroupRaw(t, s, state)},
		State: tfsdk.State{Schema: s, Raw: idpGroupRaw(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if called {
		t.Error("an unchanged membership must not trigger a request")
	}
}

func TestIdpGroupResource_Delete(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotQuery  string
	)

	r := newIdpGroupResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(idpRemoveResponseExample))
	})

	ctx := context.Background()
	s := idpGroupSchema(t, r)
	state := IdpGroupResourceModel{
		ID:    types.StringValue("engineers"),
		Name:  types.StringValue("engineers"),
		Users: types.SetNull(types.StringType),
	}

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: idpGroupRaw(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if gotPath != "/api/idp/groups/engineers" {
		t.Errorf("expected path /api/idp/groups/engineers, got %s", gotPath)
	}
	// A group owns its members, so it must be removed with force=true.
	if gotQuery != "force=true" {
		t.Errorf("expected query force=true, got %q", gotQuery)
	}
}

func TestIdpGroupResource_DeleteNotFoundIsSuccess(t *testing.T) {
	r := newIdpGroupResourceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(idpGroupNotFoundExample))
	})

	ctx := context.Background()
	s := idpGroupSchema(t, r)
	state := IdpGroupResourceModel{
		ID:    types.StringValue("engineers"),
		Name:  types.StringValue("engineers"),
		Users: types.SetNull(types.StringType),
	}

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: idpGroupRaw(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("deleting an already deleted group must succeed: %v", resp.Diagnostics)
	}
}

func TestIdpGroupResource_ImportState(t *testing.T) {
	r := New().(resource.ResourceWithImportState)

	ctx := context.Background()
	s := idpGroupSchema(t, New())

	nullRaw := idpGroupRaw(t, s, IdpGroupResourceModel{
		ID:    types.StringNull(),
		Name:  types.StringNull(),
		Users: types.SetNull(types.StringType),
	})
	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: nullRaw}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "engineers"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var name types.String
	if diags := resp.State.GetAttribute(ctx, path.Root("name"), &name); diags.HasError() {
		t.Fatalf("failed to read imported name: %v", diags)
	}
	if name.ValueString() != "engineers" {
		t.Errorf("imported name = %q, want engineers", name.ValueString())
	}
}

func TestIdpGroupResource_ImportState_Invalid(t *testing.T) {
	r := New().(resource.ResourceWithImportState)

	ctx := context.Background()
	s := idpGroupSchema(t, New())

	nullRaw := idpGroupRaw(t, s, IdpGroupResourceModel{
		ID:    types.StringNull(),
		Name:  types.StringNull(),
		Users: types.SetNull(types.StringType),
	})
	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: nullRaw}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: ""}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for an empty import identifier")
	}
}

// TestIdpGroupDTOsMatchSpec guards the client DTOs against the spec examples.
func TestIdpGroupDTOsMatchSpec(t *testing.T) {
	var addRequest models.IdpAddGroupRequest
	if err := json.Unmarshal([]byte(idpAddGroupRequestExample), &addRequest); err != nil {
		t.Fatalf("failed to decode AddGroupRequest example: %v", err)
	}
	if addRequest.Group != "engineers" {
		t.Errorf("unexpected AddGroupRequest: %+v", addRequest)
	}

	var membership models.IdpGroupMembershipChangeRequest
	if err := json.Unmarshal([]byte(idpGroupMembershipRequest), &membership); err != nil {
		t.Fatalf("failed to decode GroupMembershipChangeRequest example: %v", err)
	}
	if !reflect.DeepEqual(membership.UsersToAdd, []string{"alice", "bob"}) {
		t.Errorf("usersToAdd = %v, want [alice bob]", membership.UsersToAdd)
	}
	if !reflect.DeepEqual(membership.UsersToRemove, []string{"carol"}) {
		t.Errorf("usersToRemove = %v, want [carol]", membership.UsersToRemove)
	}

	var addResponse models.IdpGroupResponse
	if err := json.Unmarshal([]byte(idpAddGroupResponseExample), &addResponse); err != nil {
		t.Fatalf("failed to decode IdpGroupAddResponse example: %v", err)
	}
	if addResponse.Code != 0 || addResponse.Group.Name != "engineers" || len(addResponse.Group.Users) != 0 {
		t.Errorf("unexpected IdpGroupAddResponse: %+v", addResponse)
	}

	var getResponse models.IdpGroupResponse
	if err := json.Unmarshal([]byte(idpGroupGetResponseExample), &getResponse); err != nil {
		t.Fatalf("failed to decode IdpGroupGetResponse example: %v", err)
	}
	if !reflect.DeepEqual(getResponse.Group.Users, []string{"alice", "bob"}) {
		t.Errorf("unexpected IdpGroupGetResponse: %+v", getResponse)
	}
}
