package user_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/user"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// userResponse is the verbatim `UserResponse` example of users.yaml
// (components/examples/UserResponse).
const userResponse = `{
  "code": 0,
  "user": {
    "name": "user1",
    "roles": [],
    "audit": {
      "creator": "gravitino",
      "createTime": "2023-12-08T06:41:25.595Z"
    }
  }
}`

// nameListResponse is the verbatim `NameListResponse` example of users.yaml.
const nameListResponse = `{
  "code": 0,
  "names": [ "user1", "user2" ]
}`

// noSuchUserError is the verbatim `NoSuchUserException` example of users.yaml.
const noSuchUserError = `{
  "code": 1003,
  "type": "NoSuchUserException",
  "message": "User does not exist",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchUserException: User does not exist",
    "..."
  ]
}`

// noSuchMetalakeError is the verbatim `NoSuchMetalakeException` example of
// metalakes.yaml.
const noSuchMetalakeError = `{
  "code": 1003,
  "type": "NoSuchMetalakeException",
  "message": "Metalake my_test_metalake does not exist",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchMetalakeException: Metalake my_test_metalake does not exist",
    "..."
  ]
}`

func userSchema(t *testing.T, d datasource.DataSource) schema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func tfValue(t *testing.T, ctx context.Context, s schema.Schema, model any) tftypes.Value {
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

func newServer(t *testing.T, handler http.HandlerFunc) *client.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	return c
}

func TestUsersDataSource_Metadata(t *testing.T) {
	d := ds.NewListDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_users" {
		t.Fatalf("expected gravitino_users, got %s", resp.TypeName)
	}
}

func TestUserDataSource_Metadata(t *testing.T) {
	d := ds.NewGetDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_user" {
		t.Fatalf("expected gravitino_user, got %s", resp.TypeName)
	}
}

func TestUsersDataSource_Read(t *testing.T) {
	ctx := context.Background()
	d := ds.NewListDataSource()
	s := userSchema(t, d)

	var requestPath string
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, nameListResponse)
	})
	d.(*ds.UsersDataSource).SetClient(c)

	config := ds.UsersDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Names:    types.ListNull(types.StringType),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tfValue(t, ctx, s, config)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if want := "/api/metalakes/my_test_metalake/users"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}

	var state ds.UsersDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	names := state.Names.Elements()
	if len(names) != 2 || names[0].(types.String).ValueString() != "user1" || names[1].(types.String).ValueString() != "user2" {
		t.Fatalf("unexpected names: %v", state.Names)
	}
}

func TestUserDataSource_Read(t *testing.T) {
	ctx := context.Background()
	d := ds.NewGetDataSource()
	s := userSchema(t, d)

	var requestPath string
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		fmt.Fprint(w, userResponse)
	})
	d.(*ds.UserDataSource).SetClient(c)

	config := ds.UserDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Name:     types.StringValue("user1"),
		Roles:    types.SetNull(types.StringType),
		Audit:    types.ObjectNull(ds.AuditAttrTypes),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tfValue(t, ctx, s, config)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if want := "/api/metalakes/my_test_metalake/users/user1"; requestPath != want {
		t.Fatalf("expected path %s, got %s", want, requestPath)
	}

	var state ds.UserDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.Roles.IsUnknown() || state.Roles.IsNull() {
		t.Fatalf("roles must be set from the response, got %v", state.Roles)
	}
	if state.Audit.IsNull() || state.Audit.IsUnknown() {
		t.Fatal("audit must be set from the response")
	}
}

func TestUserDataSource_ReadNotFound(t *testing.T) {
	ctx := context.Background()
	d := ds.NewGetDataSource()
	s := userSchema(t, d)

	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchUserError)
	})
	d.(*ds.UserDataSource).SetClient(c)

	config := ds.UserDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Name:     types.StringValue("user1"),
		Roles:    types.SetNull(types.StringType),
		Audit:    types.ObjectNull(ds.AuditAttrTypes),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tfValue(t, ctx, s, config)}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic when the user does not exist")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "User not found") {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func TestUsersDataSource_ReadMetalakeNotFound(t *testing.T) {
	ctx := context.Background()
	d := ds.NewListDataSource()
	s := userSchema(t, d)

	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, noSuchMetalakeError)
	})
	d.(*ds.UsersDataSource).SetClient(c)

	config := ds.UsersDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Names:    types.ListNull(types.StringType),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tfValue(t, ctx, s, config)}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic when the metalake does not exist")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Metalake not found") {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func TestUsersDataSource_ReadServerError(t *testing.T) {
	ctx := context.Background()
	d := ds.NewListDataSource()
	s := userSchema(t, d)

	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalServerError","message":"boom","stack":["..."]}`)
	})
	d.(*ds.UsersDataSource).SetClient(c)

	config := ds.UsersDataSourceModel{
		Metalake: types.StringValue("my_test_metalake"),
		Names:    types.ListNull(types.StringType),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tfValue(t, ctx, s, config)}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for a 500 response")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), `Failed listing users "my_test_metalake"`) {
		t.Fatalf("unexpected summary: %s", resp.Diagnostics.Errors()[0].Summary())
	}
}
