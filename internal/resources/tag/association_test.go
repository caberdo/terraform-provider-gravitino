package tag_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/resources/resourcetest"
	res "github.com/gravitino/terraform-provider-gravitino/internal/resources/tag"
	"github.com/gravitino/terraform-provider-gravitino/internal/tfutil"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Payloads below are copied literally from the Gravitino v1.3.1 OpenAPI spec
// (docs/open-api/tags.yaml).
const (
	tagAssociationBasePath = "/api/metalakes/test_metalake/objects/TABLE/sales.orders/tags"

	// components/examples/TagAssociate
	specTagAssociate = `{
  "tagsToAdd": ["my_tag1", "my_tag2"],
  "tagsToRemove": ["my_tag3"]
}`

	// components/examples/NameListResponse
	specTagAssociationNames = `{
  "code": 0,
  "names": ["my_tag1", "my_tag2"]
}`

	// components/examples/TagListResponse, with `inherited` set so the Read
	// filter can be exercised. The 1.3.1 spec explicitly documents the field and
	// shows it as null on a direct association.
	specTagAssociationList = `{
  "code": 0,
  "tags": [
    {
      "name": "my_tag1",
      "comment": "This is my tag1",
      "properties": {"key1": "value1", "key2": "value2"},
      "audit": {"creator": "gravitino", "createTime": "2023-12-08T03:41:25.595Z"},
      "inherited": false
    },
    {
      "name": "my_tag2",
      "comment": "This is my tag2",
      "properties": {"key2": "value2"},
      "audit": {"creator": "gravitino", "createTime": "2023-12-08T06:41:25.595Z"},
      "inherited": true
    }
  ]
}`
)

type associationRecorder struct {
	mu       sync.Mutex
	requests []associationRequest
}

type associationRequest struct {
	Method   string
	Path     string
	RawQuery string
	Body     map[string]interface{}
}

func (rec *associationRecorder) record(r *http.Request) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var body map[string]interface{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	rec.requests = append(rec.requests, associationRequest{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Body:     body,
	})
}

func (rec *associationRecorder) count(method string) int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	n := 0
	for _, req := range rec.requests {
		if req.Method == method {
			n++
		}
	}
	return n
}

func (rec *associationRecorder) only(t *testing.T, method string) associationRequest {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var found []associationRequest
	for _, req := range rec.requests {
		if req.Method == method {
			found = append(found, req)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly one %s request, got %d (%v)", method, len(found), rec.requests)
	}
	return found[0]
}

func newTagAssociation(t *testing.T, rec *associationRecorder, handler func(w http.ResponseWriter, r *http.Request)) *res.TagAssociationResource {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", tagJSONMedia)
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	r := res.NewAssociation()
	r.(*res.TagAssociationResource).SetClient(c)
	return r.(*res.TagAssociationResource)
}

func tagAssociationSchema(t *testing.T) schema.Schema {
	t.Helper()
	r := res.NewAssociation()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func tagAssociationModel(t *testing.T, objectType, objectFullName string, tags ...string) res.TagAssociationResourceModel {
	t.Helper()
	set, diags := tfutil.StringsToSet(tags)
	if diags.HasError() {
		t.Fatalf("tags set: %v", diags)
	}
	return res.TagAssociationResourceModel{
		ID:             types.StringUnknown(),
		Metalake:       types.StringValue("test_metalake"),
		ObjectType:     types.StringValue(objectType),
		ObjectFullName: types.StringValue(objectFullName),
		Tags:           set,
	}
}

func tagAssociationValue(t *testing.T, s schema.Schema, model res.TagAssociationResourceModel) tftypes.Value {
	t.Helper()
	return resourcetest.TFValue(t, context.Background(), s, model)
}

func TestTagAssociationResource_Metadata(t *testing.T) {
	r := res.NewAssociation()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_tag_association" {
		t.Fatalf("type name = %q, want gravitino_tag_association", resp.TypeName)
	}
}

func TestTagAssociationResource_Schema(t *testing.T) {
	s := tagAssociationSchema(t)
	for _, name := range []string{"id", "metalake", "object_type", "object_full_name", "tags"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	tags, ok := s.Attributes["tags"].(schema.SetAttribute)
	if !ok {
		t.Fatal("tags must be a set attribute")
	}
	if !tags.Required {
		t.Error("tags must be required")
	}

	for _, name := range []string{"metalake", "object_type", "object_full_name"} {
		attr, ok := s.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s must be a string attribute", name)
		}
		if len(attr.PlanModifiers) == 0 {
			t.Errorf("%s must carry RequiresReplace", name)
		}
	}

	id, ok := s.Attributes["id"].(schema.StringAttribute)
	if !ok || len(id.PlanModifiers) != 1 {
		t.Fatalf("id must carry exactly one plan modifier, got %#v", s.Attributes["id"])
	}
	modifier, ok := id.PlanModifiers[0].(models.CompoundIDPlanModifier)
	if !ok {
		t.Fatalf("id plan modifier = %T, want models.CompoundIDPlanModifier", id.PlanModifiers[0])
	}
	if got := modifier.Components; len(got) != 3 || got[0] != "metalake" || got[1] != "object_type" || got[2] != "object_full_name" {
		t.Errorf("id components = %v", got)
	}
}

func TestTagAssociationResource_Create_SendsTagsToAdd(t *testing.T) {
	rec := &associationRecorder{}
	r := newTagAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost && req.URL.Path == tagAssociationBasePath {
			io.WriteString(w, specTagAssociationNames)
			return
		}
		http.NotFound(w, req)
	})

	s := tagAssociationSchema(t)
	ctx := context.Background()
	plan := tagAssociationModel(t, "TABLE", "sales.orders", "my_tag1", "my_tag2")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: tagAssociationValue(t, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	sent := rec.only(t, http.MethodPost)
	want := map[string]interface{}{"tagsToAdd": []interface{}{"my_tag1", "my_tag2"}}
	resourcetest.AssertJSONEqualUnordered(t, string(mustJSON(t, want)), mustJSON(t, sent.Body))

	var got res.TagAssociationResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.ID.ValueString() != "test_metalake.TABLE.sales.orders" {
		t.Errorf("id = %q", got.ID.ValueString())
	}
	if names := tfutil.SetToStrings(got.Tags); !sameSet(names, []string{"my_tag1", "my_tag2"}) {
		t.Errorf("tags = %v", names)
	}
}

func TestTagAssociationResource_Create_EmptyTagsSendsNothing(t *testing.T) {
	rec := &associationRecorder{}
	r := newTagAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		http.NotFound(w, req)
	})

	s := tagAssociationSchema(t)
	ctx := context.Background()
	plan := tagAssociationModel(t, "TABLE", "sales.orders")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: tagAssociationValue(t, s, plan)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}
	if n := rec.count(http.MethodPost); n != 0 {
		t.Errorf("an empty tag set must not send a request, got %d POSTs", n)
	}
}

func TestTagAssociationResource_Update_SendsDiff(t *testing.T) {
	rec := &associationRecorder{}
	r := newTagAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost && req.URL.Path == tagAssociationBasePath {
			io.WriteString(w, specTagAssociationNames)
			return
		}
		http.NotFound(w, req)
	})

	s := tagAssociationSchema(t)
	ctx := context.Background()
	state := tagAssociationModel(t, "TABLE", "sales.orders", "my_tag3")
	plan := tagAssociationModel(t, "TABLE", "sales.orders", "my_tag1", "my_tag2")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tagAssociationValue(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tagAssociationValue(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	sent := rec.only(t, http.MethodPost)
	// The state/plan diff is exactly the spec's TagAssociate example.
	resourcetest.AssertJSONEqualUnordered(t, specTagAssociate, mustJSON(t, sent.Body))
}

func TestTagAssociationResource_Update_NoDiffSendsNothing(t *testing.T) {
	rec := &associationRecorder{}
	r := newTagAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		http.NotFound(w, req)
	})

	s := tagAssociationSchema(t)
	ctx := context.Background()
	state := tagAssociationModel(t, "TABLE", "sales.orders", "my_tag1")
	plan := tagAssociationModel(t, "TABLE", "sales.orders", "my_tag1")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tagAssociationValue(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tagAssociationValue(t, s, state)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}
	if n := rec.count(http.MethodPost); n != 0 {
		t.Errorf("an unchanged tag set must not send a request, got %d POSTs", n)
	}
}

func TestTagAssociationResource_Read_IgnoresInheritedTags(t *testing.T) {
	rec := &associationRecorder{}
	r := newTagAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == tagAssociationBasePath {
			io.WriteString(w, specTagAssociationList)
			return
		}
		http.NotFound(w, req)
	})

	s := tagAssociationSchema(t)
	ctx := context.Background()
	state := tagAssociationModel(t, "TABLE", "sales.orders", "my_tag1", "my_tag2")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tagAssociationValue(t, s, state)}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: tagAssociationValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	get := rec.only(t, http.MethodGet)
	if get.RawQuery != "details=true" {
		t.Errorf("query = %q, want details=true so inherited can be told apart", get.RawQuery)
	}

	var got res.TagAssociationResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if names := tfutil.SetToStrings(got.Tags); !sameSet(names, []string{"my_tag1"}) {
		t.Errorf("tags = %v, want only the direct tag my_tag1", names)
	}
}

func TestTagAssociationResource_Read_NotFoundRemovesState(t *testing.T) {
	rec := &associationRecorder{}
	r := newTagAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, specNoSuchTagException)
	})

	s := tagAssociationSchema(t)
	ctx := context.Background()
	state := tagAssociationModel(t, "TABLE", "sales.orders", "my_tag1")
	raw := tagAssociationValue(t, s, state)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 read must not error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("404 read must remove the resource from state")
	}
}

func TestTagAssociationResource_Delete_RemovesEveryTagInState(t *testing.T) {
	rec := &associationRecorder{}
	r := newTagAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			io.WriteString(w, `{"code":0,"names":[]}`)
			return
		}
		http.NotFound(w, req)
	})

	s := tagAssociationSchema(t)
	ctx := context.Background()
	state := tagAssociationModel(t, "TABLE", "sales.orders", "my_tag1", "my_tag2")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tagAssociationValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", resp.Diagnostics)
	}

	sent := rec.only(t, http.MethodPost)
	want := map[string]interface{}{"tagsToRemove": []interface{}{"my_tag1", "my_tag2"}}
	resourcetest.AssertJSONEqualUnordered(t, string(mustJSON(t, want)), mustJSON(t, sent.Body))
}

func TestTagAssociationResource_Delete_NotFoundIsSuccess(t *testing.T) {
	rec := &associationRecorder{}
	r := newTagAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, specNoSuchTagException)
	})

	s := tagAssociationSchema(t)
	ctx := context.Background()
	state := tagAssociationModel(t, "TABLE", "sales.orders", "my_tag1")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: tagAssociationValue(t, s, state)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("404 delete must be treated as success: %v", resp.Diagnostics)
	}
}

func TestTagAssociationResource_ImportState(t *testing.T) {
	r := res.NewAssociation().(resource.ResourceWithImportState)
	ctx := context.Background()
	s := tagAssociationSchema(t)
	nullModel := tagAssociationModel(t, "", "")
	nullModel.ID = types.StringNull()

	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tagAssociationValue(t, s, nullModel)}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "my_metalake.TABLE.sales.orders"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	var got res.TagAssociationResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Metalake.ValueString() != "my_metalake" {
		t.Errorf("metalake = %q", got.Metalake.ValueString())
	}
	if got.ObjectType.ValueString() != "TABLE" {
		t.Errorf("object_type = %q", got.ObjectType.ValueString())
	}
	if got.ObjectFullName.ValueString() != "sales.orders" {
		t.Errorf("object_full_name = %q", got.ObjectFullName.ValueString())
	}
	if got.ID.ValueString() != "my_metalake.TABLE.sales.orders" {
		t.Errorf("id = %q", got.ID.ValueString())
	}
}

func TestTagAssociationResource_ImportState_Invalid(t *testing.T) {
	for _, id := range []string{"no_dots", "my_metalake.TABLE", "my_metalake.METALAKE.a", "my_metalake.ROLE.a", "my_metalake.TABLE."} {
		t.Run(id, func(t *testing.T) {
			r := res.NewAssociation().(resource.ResourceWithImportState)
			ctx := context.Background()
			s := tagAssociationSchema(t)
			nullModel := tagAssociationModel(t, "", "")
			nullModel.ID = types.StringNull()

			resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tagAssociationValue(t, s, nullModel)}}
			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error for invalid import ID %q", id)
			}
		})
	}
}

// VIEW only exists in the metadataObjectType enum from Gravitino 1.3.1 on. The
// resource must reject it before calling the association endpoint, naming the
// required version.
func TestTagAssociationResource_Create_RejectsViewOn130Server(t *testing.T) {
	rec := &associationRecorder{}
	r := newTagAssociation(t, rec, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/api/version" {
			io.WriteString(w, `{"code":0,"version":{"version":"1.3.0"}}`)
			return
		}
		http.NotFound(w, req)
	})

	s := tagAssociationSchema(t)
	ctx := context.Background()
	plan := tagAssociationModel(t, "VIEW", "sales.orders_view", "my_tag1")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: tagAssociationValue(t, s, plan)}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a version diagnostic for VIEW on a 1.3.0 server")
	}
	if n := rec.count(http.MethodPost); n != 0 {
		t.Errorf("the association endpoint must not be called, got %d POSTs", n)
	}
	if !diagnosticContains(resp.Diagnostics, "requires Gravitino 1.3.1") {
		t.Errorf("diagnostics = %v, want the required version named", resp.Diagnostics)
	}
}

func mustJSON(t *testing.T, value interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	return strings.Join(g, "\x00") == strings.Join(w, "\x00")
}

func diagnosticContains(diags diag.Diagnostics, substr string) bool {
	for _, d := range diags {
		if strings.Contains(d.Summary(), substr) || strings.Contains(d.Detail(), substr) {
			return true
		}
	}
	return false
}
