package tag_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/tag"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func newObjectTagsDataSource(t *testing.T, handler http.HandlerFunc) datasource.DataSource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	d := ds.NewObjectTagsDataSource()
	configureTagDataSource(t, d, c)
	return d
}

func TestObjectTagsDataSource_Metadata(t *testing.T) {
	d := ds.NewObjectTagsDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_object_tags" {
		t.Fatalf("type name = %q, want gravitino_object_tags", resp.TypeName)
	}
}

func TestObjectTagsDataSource_Schema(t *testing.T) {
	s := tagDataSourceSchema(t, ds.NewObjectTagsDataSource())
	for _, name := range []string{"metalake", "object_type", "object_full_name", "tags"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

func TestObjectTagsDataSource_Read(t *testing.T) {
	const path = "/api/metalakes/test_metalake/objects/TABLE/sales.orders/tags"

	var gotQuery string
	d := newObjectTagsDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == path {
			gotQuery = req.URL.RawQuery
			io.WriteString(w, dsSpecTagListResponse)
			return
		}
		http.NotFound(w, req)
	})

	s := tagDataSourceSchema(t, d)
	ctx := context.Background()
	config := ds.ObjectTagsDataSourceModel{
		Metalake:       types.StringValue("test_metalake"),
		ObjectType:     types.StringValue("TABLE"),
		ObjectFullName: types.StringValue("sales.orders"),
		Tags:           types.ListNull(types.StringType),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tagConfigValue(t, s, config)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if gotQuery != "details=true" {
		t.Errorf("query = %q, want details=true", gotQuery)
	}

	var got ds.ObjectTagsDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	names := listToStrings(t, got.Tags)
	if len(names) != 2 || names[0] != "my_tag1" || names[1] != "my_tag2" {
		t.Errorf("tags = %v, want the spec's two tag names", names)
	}
}

func TestObjectTagsDataSource_Read_NotFound(t *testing.T) {
	d := newObjectTagsDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, dsSpecNoSuchTagException)
	})

	s := tagDataSourceSchema(t, d)
	ctx := context.Background()
	config := ds.ObjectTagsDataSourceModel{
		Metalake:       types.StringValue("test_metalake"),
		ObjectType:     types.StringValue("TABLE"),
		ObjectFullName: types.StringValue("sales.orders"),
		Tags:           types.ListNull(types.StringType),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tagConfigValue(t, s, config)}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a not-found diagnostic")
	}
}

func TestObjectTagsDataSource_Read_RejectsViewOn130Server(t *testing.T) {
	var requestedObjects bool
	d := newObjectTagsDataSource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/api/version" {
			io.WriteString(w, `{"code":0,"version":{"version":"1.3.0"}}`)
			return
		}
		requestedObjects = true
		http.NotFound(w, req)
	})

	s := tagDataSourceSchema(t, d)
	ctx := context.Background()
	config := ds.ObjectTagsDataSourceModel{
		Metalake:       types.StringValue("test_metalake"),
		ObjectType:     types.StringValue("VIEW"),
		ObjectFullName: types.StringValue("sales.orders_view"),
		Tags:           types.ListNull(types.StringType),
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tagConfigValue(t, s, config)}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a version diagnostic for VIEW on a 1.3.0 server")
	}
	if requestedObjects {
		t.Error("the object endpoint must not be called on a 1.3.0 server")
	}
	if !diagnosticContains(resp.Diagnostics, "requires Gravitino 1.3.1") {
		t.Errorf("diagnostics = %v, want the required version named", resp.Diagnostics)
	}
}

func listToStrings(t *testing.T, list types.List) []string {
	t.Helper()
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	values := make([]string, 0, len(list.Elements()))
	for _, element := range list.Elements() {
		value, ok := element.(types.String)
		if !ok {
			t.Fatalf("element %v is not a string", element)
		}
		values = append(values, value.ValueString())
	}
	return values
}

func diagnosticContains(diags diag.Diagnostics, substr string) bool {
	for _, d := range diags {
		if strings.Contains(d.Summary(), substr) || strings.Contains(d.Detail(), substr) {
			return true
		}
	}
	return false
}
