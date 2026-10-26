package catalog

import (
	"context"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models/modeltest"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// TestCatalogItemModelMatchesSchema guards the unexported list item model, which
// the external catalog_test package cannot reach.
func TestCatalogItemModelMatchesSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	NewListDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)

	catalogs, ok := resp.Schema.Attributes["catalogs"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("catalogs attribute = %T, want schema.ListNestedAttribute", resp.Schema.Attributes["catalogs"])
	}
	modeltest.AssertAttributesMatch(t, catalogItemModel{}, modeltest.Keys(catalogs.NestedObject.Attributes))
}
