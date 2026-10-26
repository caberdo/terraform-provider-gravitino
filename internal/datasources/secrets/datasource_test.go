package secrets

import (
	"context"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models/modeltest"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

func TestSecretsDataSource_Metadata(t *testing.T) {
	d := New()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_secrets" {
		t.Fatalf("expected gravitino_secrets, got %s", resp.TypeName)
	}
}

func TestSecretsDataSource_Schema(t *testing.T) {
	d := New()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	for _, name := range []string{"metalake", "resource_type", "resource", "secrets"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Fatalf("missing attribute %s", name)
		}
	}
}

// TestSecretsDataSourceModelsMatchSchema keeps the model in lockstep with the
// gravitino_secrets schema.
func TestSecretsDataSourceModelsMatchSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	New().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	modeltest.AssertSchemaMatches(t, SecretsDataSourceModel{}, resp.Schema.Attributes, resp.Schema.Blocks)
}
