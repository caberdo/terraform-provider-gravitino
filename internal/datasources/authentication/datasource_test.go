package authentication_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/authentication"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPrincipalDataSource_Schema(t *testing.T) {
	d := ds.New()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_principal" {
		t.Fatalf("expected gravitino_principal, got %s", resp.TypeName)
	}
}

func TestPrincipalDataSource_Read(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/authn/me" {
			t.Errorf("expected path /api/authn/me, got %s", r.URL.Path)
		}

		resp := models.PrincipalResponse{
			Code:      0,
			Principal: "admin",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.New()
	d.(*ds.PrincipalDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	attrTypes := map[string]attr.Type{
		"name":  types.StringType,
		"roles": types.ListType{ElemType: types.StringType},
	}

	configModel := ds.PrincipalDataSourceModel{
		Name:  types.StringNull(),
		Roles: types.ListNull(types.StringType),
	}

	configObj, diags := types.ObjectValueFrom(ctx, attrTypes, configModel)
	if diags.HasError() {
		t.Fatalf("failed to create config object: %v", diags)
	}

	tfVal, err := configObj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}

	req := datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaObj, Raw: tfVal},
	}
	resp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaObj},
	}

	d.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		for _, diag := range resp.Diagnostics.Errors() {
			t.Logf("diag error: %s: %s", diag.Summary(), diag.Detail())
		}
		t.Fatal("unexpected diagnostics errors")
	}
}
