package tag_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/tag"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Payloads below are copied literally from the Gravitino v1.3.0 OpenAPI spec
// (docs/open-api/tags.yaml).

const (
	dsTagBasePath = "/api/metalakes/test_metalake/tags"
	dsTagItemPath = dsTagBasePath + "/my_tag1"

	// components/examples/TagResponse
	dsSpecTagResponse = `{
  "code": 0,
  "tag": {
    "name": "my_tag1",
    "comment": "This is my tag1",
    "properties": {
      "key1": "value1",
      "key2": "value2"
    },
    "audit": {
      "creator": "gravitino",
      "createTime": "2023-12-08T03:41:25.595Z"
    },
    "inherited": false
  }
}`

	// components/examples/TagListResponse
	dsSpecTagListResponse = `{
  "code": 0,
  "tags": [
    {
      "name": "my_tag1",
      "comment": "This is my tag1",
      "properties": {
        "key1": "value1",
        "key2": "value2"
      },
      "audit": {
        "creator": "gravitino",
        "createTime": "2023-12-08T03:41:25.595Z"
      },
      "inherited": null
    },
    {
      "name": "my_tag2",
      "comment": "This is my tag2",
      "properties": {
        "key2": "value2"
      },
      "audit": {
        "creator": "gravitino",
        "createTime": "2023-12-08T06:41:25.595Z"
      },
      "inherited": null
    }
  ]
}`

	// components/examples/NoSuchTagException
	dsSpecNoSuchTagException = `{
  "code": 1003,
  "type": "NoSuchTagException",
  "message": "Failed to operate tag(s) [my_tag] operation [LOAD] under metalake [my_test_metalake], reason [NoSuchTagException]",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchTagException: Tag my_tag does not exist",
    "..."
  ]
}`
)

func tagDataSourceSchema(t *testing.T, d datasource.DataSource) schema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func tagConfigValue(t *testing.T, s schema.Schema, model interface{}) tftypes.Value {
	t.Helper()
	ctx := context.Background()
	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("config: %v", diags)
	}
	raw, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("config value: %v", err)
	}
	return raw
}

func newGetTagDataSource(t *testing.T, handler http.HandlerFunc) datasource.DataSource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	d := ds.NewGetDataSource()
	configureTagDataSource(t, d, c)
	return d
}

func newListTagDataSource(t *testing.T, handler http.HandlerFunc) datasource.DataSource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	d := ds.NewListDataSource()
	configureTagDataSource(t, d, c)
	return d
}

func configureTagDataSource(t *testing.T, d datasource.DataSource, c *client.Client) {
	t.Helper()
	configured, ok := d.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatalf("%T does not implement DataSourceWithConfigure", d)
	}
	resp := &datasource.ConfigureResponse{}
	configured.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("configure diagnostics: %v", resp.Diagnostics)
	}
}

func TestTagDataSource_Schema(t *testing.T) {
	d := ds.NewGetDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_tag" {
		t.Fatalf("expected gravitino_tag, got %s", resp.TypeName)
	}

	s := tagDataSourceSchema(t, d)
	for _, name := range []string{"metalake", "name", "comment", "properties", "audit", "inherited"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	inherited, ok := s.Attributes["inherited"].(schema.BoolAttribute)
	if !ok {
		t.Fatal("inherited must be a bool attribute")
	}
	if !inherited.Computed {
		t.Error("inherited must be computed")
	}
}

func TestTagDataSource_Read(t *testing.T) {
	var gotPath string
	d := newGetTagDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		io.WriteString(w, dsSpecTagResponse)
	})

	ctx := context.Background()
	s := tagDataSourceSchema(t, d)
	auditAttrTypes := s.Attributes["audit"].(schema.ObjectAttribute).AttributeTypes

	raw := tagConfigValue(t, s, ds.TagDataSourceModel{
		Metalake:   types.StringValue("test_metalake"),
		Name:       types.StringValue("my_tag1"),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(auditAttrTypes),
		Inherited:  types.BoolNull(),
	})

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if gotPath != dsTagItemPath {
		t.Errorf("path = %q, want %q", gotPath, dsTagItemPath)
	}

	var got ds.TagDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Comment.ValueString() != "This is my tag1" {
		t.Errorf("comment = %q", got.Comment.ValueString())
	}
	if got.Inherited.IsNull() || got.Inherited.IsUnknown() || got.Inherited.ValueBool() {
		t.Errorf("inherited = %v, want false from the spec response", got.Inherited)
	}
	if got.Audit.IsNull() || got.Audit.IsUnknown() {
		t.Errorf("audit = %v, want the server value", got.Audit)
	}
}

func TestTagDataSource_Read_NotFound(t *testing.T) {
	d := newGetTagDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, dsSpecNoSuchTagException)
	})

	ctx := context.Background()
	s := tagDataSourceSchema(t, d)
	auditAttrTypes := s.Attributes["audit"].(schema.ObjectAttribute).AttributeTypes

	raw := tagConfigValue(t, s, ds.TagDataSourceModel{
		Metalake:   types.StringValue("test_metalake"),
		Name:       types.StringValue("my_tag1"),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(auditAttrTypes),
		Inherited:  types.BoolNull(),
	})

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: raw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("a 404 must be reported as an error for a data source")
	}
}

func TestTagsDataSource_Schema(t *testing.T) {
	d := ds.NewListDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_tags" {
		t.Fatalf("expected gravitino_tags, got %s", resp.TypeName)
	}

	s := tagDataSourceSchema(t, d)
	tags, ok := s.Attributes["tags"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatal("tags is not a list nested attribute")
	}
	for _, name := range []string{"name", "comment", "properties", "audit", "inherited"} {
		if _, ok := tags.NestedObject.Attributes[name]; !ok {
			t.Errorf("missing tag attribute %q", name)
		}
	}
}

func TestTagsDataSource_Read(t *testing.T) {
	var gotQuery string
	d := newListTagDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.URL.Path != dsTagBasePath {
			t.Errorf("path = %q, want %q", r.URL.Path, dsTagBasePath)
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		io.WriteString(w, dsSpecTagListResponse)
	})

	ctx := context.Background()
	s := tagDataSourceSchema(t, d)

	tagsAttr, ok := s.Attributes["tags"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatal("tags is not a list nested attribute")
	}
	itemType, ok := tagsAttr.NestedObject.Type().(types.ObjectType)
	if !ok {
		t.Fatalf("tag items are not objects: %T", tagsAttr.NestedObject.Type())
	}

	raw := tagConfigValue(t, s, ds.TagsDataSourceModel{
		Metalake: types.StringValue("test_metalake"),
		Tags:     types.ListNull(itemType),
	})

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if gotQuery != "details=true" {
		t.Errorf("query = %q, want details=true", gotQuery)
	}

	var got ds.TagsDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if len(got.Tags.Elements()) != 2 {
		t.Fatalf("tags = %v, want the two spec tags", got.Tags)
	}
	for i, element := range got.Tags.Elements() {
		item, ok := element.(types.Object)
		if !ok {
			t.Fatalf("tag item = %T, want types.Object", element)
		}
		inherited, ok := item.Attributes()["inherited"].(types.Bool)
		if !ok {
			t.Fatalf("tag %d: inherited = %T", i, item.Attributes()["inherited"])
		}
		if !inherited.IsNull() || inherited.IsUnknown() {
			t.Errorf("tag %d: inherited = %v, want null (the spec example uses null)", i, inherited)
		}
		if audit, ok := item.Attributes()["audit"].(types.Object); !ok || audit.IsNull() {
			t.Errorf("tag %d: audit = %v, want the server value", i, item.Attributes()["audit"])
		}
	}
	first, ok := got.Tags.Elements()[0].(types.Object)
	if !ok {
		t.Fatal("first tag is not an object")
	}
	if name, ok := first.Attributes()["name"].(types.String); !ok || name.ValueString() != "my_tag1" {
		t.Errorf("first tag name = %v", first.Attributes()["name"])
	}
	if comment, ok := first.Attributes()["comment"].(types.String); !ok || comment.ValueString() != "This is my tag1" {
		t.Errorf("first tag comment = %v", first.Attributes()["comment"])
	}
}

func TestTagsDataSource_Read_NotFound(t *testing.T) {
	d := newListTagDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"code":1003,"type":"NoSuchMetalakeException","message":"Metalake does not exist","stack":["org.apache.gravitino.exceptions.NoSuchMetalakeException: Metalake does not exist"]}`)
	})

	ctx := context.Background()
	s := tagDataSourceSchema(t, d)
	tagsAttr, ok := s.Attributes["tags"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatal("tags is not a list nested attribute")
	}
	itemType, ok := tagsAttr.NestedObject.Type().(types.ObjectType)
	if !ok {
		t.Fatalf("tag items are not objects: %T", tagsAttr.NestedObject.Type())
	}

	raw := tagConfigValue(t, s, ds.TagsDataSourceModel{
		Metalake: types.StringValue("test_metalake"),
		Tags:     types.ListNull(itemType),
	})

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: raw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("a 404 must be reported as an error for a data source")
	}
}

var _ = attr.Type(nil)
