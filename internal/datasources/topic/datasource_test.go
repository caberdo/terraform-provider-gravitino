package topic_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/topic"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Payloads below are copied literally from the Gravitino v1.3.0 OpenAPI spec
// (docs/open-api/topics.yaml).

const (
	dsTopicBasePath = "/api/metalakes/test_metalake/catalogs/test_catalog/schemas/test_schema/topics"
	dsTopicItemPath = dsTopicBasePath + "/topic1"

	// components/examples/TopicResponse
	dsSpecTopicResponse = `{
  "code": 0,
  "topic": {
    "name": "topic1",
    "comment": "This is a topic",
    "properties": {
      "partition-count": "1",
      "replication-factor": "1"
    }
  }
}`

	// components/examples/NoSuchTopicException
	dsSpecNoSuchTopicException = `{
  "code": 1003,
  "type": "NoSuchTopicException",
  "message": "Topic does not exist",
  "stack": [
    "org.apache.gravitino.exceptions.NoSuchTopicException: Topic does not exist",
    "..."
  ]
}`
)

func topicDataSourceSchema(t *testing.T, d datasource.DataSource) schema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func topicAuditAttrTypes(t *testing.T, s schema.Schema) map[string]attr.Type {
	t.Helper()
	audit, ok := s.Attributes["audit"].(schema.ObjectAttribute)
	if !ok {
		t.Fatal("audit is not an object attribute")
	}
	return audit.AttributeTypes
}

func topicConfigValue(t *testing.T, s schema.Schema, model interface{}) tftypes.Value {
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

func newTopicDataSource(t *testing.T, handler http.HandlerFunc) datasource.DataSource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	d := ds.NewTopicDataSource()
	configureTopicDataSource(t, d, c)
	return d
}

func newTopicsDataSource(t *testing.T, handler http.HandlerFunc) datasource.DataSource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	d := ds.NewTopicsDataSource()
	configureTopicDataSource(t, d, c)
	return d
}

func configureTopicDataSource(t *testing.T, d datasource.DataSource, c *client.Client) {
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

func TestTopicDataSourceMetadata(t *testing.T) {
	d := ds.NewTopicDataSource()
	var req datasource.MetadataRequest
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), req, &resp)
	if resp.TypeName != "gravitino_topic" {
		t.Errorf("Expected type name gravitino_topic, got %s", resp.TypeName)
	}
}

func TestTopicsDataSourceMetadata(t *testing.T) {
	d := ds.NewTopicsDataSource()
	var req datasource.MetadataRequest
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), req, &resp)
	if resp.TypeName != "gravitino_topics" {
		t.Errorf("Expected type name gravitino_topics, got %s", resp.TypeName)
	}
}

func TestTopicDataSource_Read(t *testing.T) {
	var gotPath string
	d := newTopicDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		io.WriteString(w, dsSpecTopicResponse)
	})

	ctx := context.Background()
	s := topicDataSourceSchema(t, d)

	raw := topicConfigValue(t, s, ds.TopicDataSourceModel{
		Metalake:   types.StringValue("test_metalake"),
		Catalog:    types.StringValue("test_catalog"),
		Schema:     types.StringValue("test_schema"),
		Name:       types.StringValue("topic1"),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(topicAuditAttrTypes(t, s)),
	})

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if gotPath != dsTopicItemPath {
		t.Errorf("path = %q, want %q", gotPath, dsTopicItemPath)
	}

	var got ds.TopicDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Comment.ValueString() != "This is a topic" {
		t.Errorf("comment = %q", got.Comment.ValueString())
	}
	if got.Properties.IsNull() || len(got.Properties.Elements()) != 2 {
		t.Errorf("properties = %v, want the two spec properties", got.Properties)
	}
	if !got.Audit.IsNull() {
		t.Errorf("audit = %v, want null: the spec response declares no audit", got.Audit)
	}
}

func TestTopicDataSource_Read_NotFound(t *testing.T) {
	d := newTopicDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, dsSpecNoSuchTopicException)
	})

	ctx := context.Background()
	s := topicDataSourceSchema(t, d)

	raw := topicConfigValue(t, s, ds.TopicDataSourceModel{
		Metalake:   types.StringValue("test_metalake"),
		Catalog:    types.StringValue("test_catalog"),
		Schema:     types.StringValue("test_schema"),
		Name:       types.StringValue("topic1"),
		Comment:    types.StringNull(),
		Properties: types.MapNull(types.StringType),
		Audit:      types.ObjectNull(topicAuditAttrTypes(t, s)),
	})

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: raw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("a 404 must be reported as an error for a data source")
	}
}

// The list endpoint returns identifiers only (the spec declares no details
// parameter), so the data source loads every topic individually. Topics deleted
// between the list and the load (404) are skipped.
func TestTopicsDataSource_Read(t *testing.T) {
	d := newTopicsDataSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch r.URL.Path {
		case dsTopicBasePath:
			io.WriteString(w, `{"code":0,"identifiers":[{"namespace":["test_metalake","test_catalog","test_schema"],"name":"topic1"},{"namespace":["test_metalake","test_catalog","test_schema"],"name":"vanished"}]}`)
		case dsTopicItemPath:
			io.WriteString(w, dsSpecTopicResponse)
		case dsTopicBasePath + "/vanished":
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, dsSpecNoSuchTopicException)
		default:
			http.NotFound(w, r)
		}
	})

	ctx := context.Background()
	s := topicDataSourceSchema(t, d)

	topicsAttr, ok := s.Attributes["topics"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatal("topics is not a list nested attribute")
	}
	itemType, ok := topicsAttr.NestedObject.Type().(types.ObjectType)
	if !ok {
		t.Fatalf("topics items are not objects: %T", topicsAttr.NestedObject.Type())
	}

	raw := topicConfigValue(t, s, ds.TopicsDataSourceModel{
		Metalake: types.StringValue("test_metalake"),
		Catalog:  types.StringValue("test_catalog"),
		Schema:   types.StringValue("test_schema"),
		Topics:   types.ListNull(itemType),
	})

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got ds.TopicsDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if len(got.Topics.Elements()) != 1 {
		t.Fatalf("topics = %v, want only the still existing topic", got.Topics)
	}
	item, ok := got.Topics.Elements()[0].(types.Object)
	if !ok {
		t.Fatalf("topic item = %T, want types.Object", got.Topics.Elements()[0])
	}
	if name, ok := item.Attributes()["name"].(types.String); !ok || name.ValueString() != "topic1" {
		t.Errorf("name = %v", item.Attributes()["name"])
	}
	if comment, ok := item.Attributes()["comment"].(types.String); !ok || comment.ValueString() != "This is a topic" {
		t.Errorf("comment = %v", item.Attributes()["comment"])
	}
}

func TestTopicDataSource_SchemaAttributes(t *testing.T) {
	s := topicDataSourceSchema(t, ds.NewTopicDataSource())
	for _, name := range []string{"metalake", "catalog", "schema", "name", "comment", "properties", "audit"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}
