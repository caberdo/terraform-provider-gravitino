package catalog_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// TestAccCatalogResource_InUseAndForceDestroy drives both lifecycle levers
// through Terraform Core, which is where an Optional+Computed in_use and the
// force_destroy default must survive the plan/apply round trip.
func TestAccCatalogResource_InUseAndForceDestroy(t *testing.T) {
	var inUseValues []bool
	var forceValues []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/lifecycle_ml/catalogs":
			var req models.CatalogCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(models.CatalogResponse{
				Code: 0,
				Catalog: models.Catalog{
					Name:     req.Name,
					Type:     req.Type,
					Provider: req.Provider,
					Comment:  req.Comment,
				},
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/metalakes/lifecycle_ml/catalogs/lifecycle_cat":
			var req models.CatalogSetRequest
			json.NewDecoder(r.Body).Decode(&req)
			inUseValues = append(inUseValues, req.InUse)
			json.NewEncoder(w).Encode(models.BaseResponse{Code: 0})
		case r.Method == http.MethodGet && r.URL.Path == "/api/metalakes/lifecycle_ml/catalogs/lifecycle_cat":
			json.NewEncoder(w).Encode(models.CatalogResponse{
				Code: 0,
				Catalog: models.Catalog{
					Name:     "lifecycle_cat",
					Type:     "relational",
					Provider: "hive",
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/metalakes/lifecycle_ml/catalogs/lifecycle_cat":
			forceValues = append(forceValues, r.URL.Query().Get("force"))
			json.NewEncoder(w).Encode(models.DropResponse{Code: 0, Dropped: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy: func(_ *terraform.State) error {
			for _, force := range forceValues {
				if force != "false" {
					return fmt.Errorf("destroy must use force=false by default, got force=%s", force)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_catalog" "this" {
  metalake         = "lifecycle_ml"
  name             = "lifecycle_cat"
  type             = "relational"
  catalog_provider = "hive"
  in_use           = false
  force_destroy    = false
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_catalog.this", "in_use", "false"),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "force_destroy", "false"),
				),
			},
			{
				Config: `
resource "gravitino_catalog" "this" {
  metalake         = "lifecycle_ml"
  name             = "lifecycle_cat"
  type             = "relational"
  catalog_provider = "hive"
  in_use           = true
}
`,
				Check: resource.TestCheckResourceAttr("gravitino_catalog.this", "in_use", "true"),
			},
			{
				ResourceName:      "gravitino_catalog.this",
				ImportState:       true,
				ImportStateId:     "lifecycle_ml.lifecycle_cat",
				ImportStateVerify: true,
			},
			{
				// The write-only attributes adopt a known value on import, so
				// the next plan must be empty instead of showing in_use or
				// force_destroy as "known after apply".
				Config: `
resource "gravitino_catalog" "this" {
  metalake         = "lifecycle_ml"
  name             = "lifecycle_cat"
  type             = "relational"
  catalog_provider = "hive"
  in_use           = true
}
`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})

	// Create disables it, the update enables it, and the destroy disables it
	// again before sending force=false.
	if len(inUseValues) != 3 || inUseValues[0] != false || inUseValues[1] != true || inUseValues[2] != false {
		t.Fatalf("expected PATCH inUse false/true/false, got %v", inUseValues)
	}
}
