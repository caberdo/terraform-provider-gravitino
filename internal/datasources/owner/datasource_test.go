package owner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	ds "github.com/gravitino/terraform-provider-gravitino/internal/datasources/owner"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestOwnerDataSource_Schema(t *testing.T) {
	d := ds.NewOwnerDataSource()
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.TODO(), datasource.MetadataRequest{}, resp)
	if resp.TypeName != "gravitino_owner" {
		t.Fatalf("expected gravitino_owner, got %s", resp.TypeName)
	}
}

func TestOwnerDataSource_Read(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/api/metalakes/test_metalake/owners/CATALOG/test_catalog"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}

		resp := models.OwnerResponse{
			Code: 0,
			Owner: models.Owner{
				Name: "admin",
				Type: "USER",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewOwnerDataSource()
	d.(*ds.OwnerDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	schemaObj := schemaResp.Schema

	attrTypes := map[string]attr.Type{
		"metalake":         types.StringType,
		"object_type":      types.StringType,
		"object_full_name": types.StringType,
		"owner_name":       types.StringType,
		"owner_type":       types.StringType,
	}

	configModel := ds.OwnerDataSourceModel{
		Metalake:       types.StringValue("test_metalake"),
		ObjectType:     types.StringValue("CATALOG"),
		ObjectFullName: types.StringValue("test_catalog"),
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

// TestOwnerDataSource_ReadNormalizesOwnerType: Gravitino answers the owner type
// in lowercase ("user") while the API enum uses "USER"; the data source must
// expose the canonical value.
func TestOwnerDataSource_ReadNormalizesOwnerType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		// Verbatim OwnerResponse example of owners.yaml (lowercase type).
		fmt.Fprint(w, `{"code":0,"owner":{"name":"user1","type":"user"}}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewOwnerDataSource()
	d.(*ds.OwnerDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)

	configObj, _ := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"metalake":         types.StringType,
		"object_type":      types.StringType,
		"object_full_name": types.StringType,
		"owner_name":       types.StringType,
		"owner_type":       types.StringType,
	}, ds.OwnerDataSourceModel{
		Metalake:       types.StringValue("authz_ml"),
		ObjectType:     types.StringValue("CATALOG"),
		ObjectFullName: types.StringValue("c1"),
	})
	raw, _ := configObj.ToTerraformValue(ctx)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state ds.OwnerDataSourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	if state.OwnerType.ValueString() != models.OwnerTypeUser {
		t.Fatalf("expected %s, got %s", models.OwnerTypeUser, state.OwnerType.ValueString())
	}
}

func TestOwnerDataSource_ReadNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		// Verbatim NoSuchMetadataObjectException example of owners.yaml.
		fmt.Fprint(w, `{"code":1003,"type":"NoSuchMetadataObjectException","message":"Metadata object does not exist","stack":["org.apache.gravitino.exceptions.NoSuchUserException: Metadata object does not exist","..."]}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewOwnerDataSource()
	d.(*ds.OwnerDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)

	configObj, _ := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"metalake":         types.StringType,
		"object_type":      types.StringType,
		"object_full_name": types.StringType,
		"owner_name":       types.StringType,
		"owner_type":       types.StringType,
	}, ds.OwnerDataSourceModel{
		Metalake:       types.StringValue("authz_ml"),
		ObjectType:     types.StringValue("CATALOG"),
		ObjectFullName: types.StringValue("missing"),
	})
	raw, _ := configObj.ToTerraformValue(ctx)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the owner does not exist")
	}
	if summary := resp.Diagnostics.Errors()[0].Summary(); summary != "Owner not found" {
		t.Fatalf("unexpected summary: %s", summary)
	}
}

func TestOwnerDataSource_ReadServerErrorUsesGravitinoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"code":1100,"type":"InternalError","message":"boom","stack":["..."]}`)
	}))
	defer server.Close()

	c, _ := client.New(server.URL, nil)
	d := ds.NewOwnerDataSource()
	d.(*ds.OwnerDataSource).SetClient(c)

	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)

	configObj, _ := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"metalake":         types.StringType,
		"object_type":      types.StringType,
		"object_full_name": types.StringType,
		"owner_name":       types.StringType,
		"owner_type":       types.StringType,
	}, ds.OwnerDataSourceModel{
		Metalake:       types.StringValue("authz_ml"),
		ObjectType:     types.StringValue("CATALOG"),
		ObjectFullName: types.StringValue("c1"),
	})
	raw, _ := configObj.ToTerraformValue(ctx)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a 500 response")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != `Failed reading owner "c1"` {
		t.Fatalf("unexpected summary: %s", got)
	}
}
