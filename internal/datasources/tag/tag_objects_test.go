package tag_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/tag"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// components/examples/MetadataObjectListResponse of tags.yaml.
const specMetadataObjectListResponse = `{
  "code": 0,
  "metadataObjects": [
    {"type": "CATALOG", "fullName": "my_catalog"},
    {"type": "SCHEMA", "fullName": "my_catalog.my_schema"},
    {"type": "TABLE", "fullName": "my_catalog.my_schema.my_table"}
  ]
}`

func newTagObjectsDataSource(t *testing.T, handler http.HandlerFunc) datasource.DataSource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	d := ds.NewTagObjectsDataSource()
	configureTagDataSource(t, d, c)
	return d
}

func TestTagObjectsDataSource_Metadata(t *testing.T) {
	d := ds.NewTagObjectsDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_tag_objects" {
		t.Fatalf("type name = %q, want gravitino_tag_objects", resp.TypeName)
	}
}

func TestTagObjectsDataSource_Schema(t *testing.T) {
	s := tagDataSourceSchema(t, ds.NewTagObjectsDataSource())
	for _, name := range []string{"metalake", "tag", "objects"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

func TestTagObjectsDataSource_Read(t *testing.T) {
	const path = "/api/metalakes/test_metalake/tags/my_tag1/objects"

	d := newTagObjectsDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == path {
			io.WriteString(w, specMetadataObjectListResponse)
			return
		}
		http.NotFound(w, req)
	})

	s := tagDataSourceSchema(t, d)
	ctx := context.Background()
	config := ds.TagObjectsDataSourceModel{
		Metalake: types.StringValue("test_metalake"),
		Tag:      types.StringValue("my_tag1"),
		Objects:  types.ListNull(types.ObjectType{AttrTypes: ds.TagObjectItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tagConfigValue(t, s, config)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	var got ds.TagObjectsDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	elements := got.Objects.Elements()
	if len(elements) != 3 {
		t.Fatalf("objects has %d entries, want 3", len(elements))
	}

	first := elements[0].(types.Object).Attributes()
	if first["type"].(types.String).ValueString() != "CATALOG" {
		t.Errorf("type = %v, want CATALOG", first["type"])
	}
	if first["full_name"].(types.String).ValueString() != "my_catalog" {
		t.Errorf("full_name = %v, want my_catalog", first["full_name"])
	}
}

func TestTagObjectsDataSource_Read_NotFound(t *testing.T) {
	d := newTagObjectsDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, dsSpecNoSuchTagException)
	})

	s := tagDataSourceSchema(t, d)
	ctx := context.Background()
	config := ds.TagObjectsDataSourceModel{
		Metalake: types.StringValue("test_metalake"),
		Tag:      types.StringValue("my_tag1"),
		Objects:  types.ListNull(types.ObjectType{AttrTypes: ds.TagObjectItemAttrTypes}),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tagConfigValue(t, s, config)}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a not-found diagnostic")
	}
}
