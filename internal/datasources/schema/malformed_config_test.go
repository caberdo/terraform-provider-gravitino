package schema_test

import (
	"context"
	"testing"

	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/schema"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestSchemaDataSources_RejectMalformedConfig keeps the configuration-decoding
// guard covered: a config that is not the schema's object type must produce a
// diagnostic instead of reaching the API client.
func TestSchemaDataSources_RejectMalformedConfig(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func() datasource.DataSource
	}{
		{"get", ds.NewSchemaDataSource},
		{"list", ds.NewSchemasDataSource},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.build()
			schemaResp := &datasource.SchemaResponse{}
			d.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)

			resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			d.Read(context.Background(), datasource.ReadRequest{Config: tfsdk.Config{
				Schema: schemaResp.Schema,
				Raw:    tftypes.NewValue(tftypes.String, "not a configuration object"),
			}}, resp)

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected a diagnostic for a configuration that is not an object")
			}
		})
	}
}
