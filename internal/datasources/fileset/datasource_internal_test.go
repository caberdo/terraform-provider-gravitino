package fileset

import (
	"context"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models/modeltest"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// TestFilesetItemModelMatchesSchema guards the unexported list item model, which
// the external fileset_test package cannot reach.
func TestFilesetItemModelMatchesSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	NewListDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)

	filesets, ok := resp.Schema.Attributes["filesets"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("filesets attribute = %T, want schema.ListNestedAttribute", resp.Schema.Attributes["filesets"])
	}
	modeltest.AssertAttributesMatch(t, filesetItemModel{}, modeltest.Keys(filesets.NestedObject.Attributes))
}
