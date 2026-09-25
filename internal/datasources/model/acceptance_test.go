package model_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func modelDSProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func TestAccModelsDataSource_List(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		collection := "/api/metalakes/probe_ml/catalogs/probe_cat/schemas/probe_sch/models"

		switch {
		case r.URL.Path == collection:
			// The list endpoint returns identifiers only, per the spec.
			_, _ = w.Write([]byte(`{"code": 0, "identifiers": [{"namespace": ["probe_ml", "probe_cat", "probe_sch"], "name": "model1"}, {"namespace": ["probe_ml", "probe_cat", "probe_sch"], "name": "model2"}]}`))
		case strings.HasPrefix(r.URL.Path, collection+"/"):
			_, _ = w.Write([]byte(modelResponseExample))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: modelDSProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_models" "all" {
  metalake = "probe_ml"
  catalog  = "probe_cat"
  schema   = "probe_sch"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_models.all", "models.#", "2"),
					resource.TestCheckResourceAttr("data.gravitino_models.all", "models.0.name", "model1"),
					resource.TestCheckResourceAttr("data.gravitino_models.all", "models.0.latest_version", "0"),
					resource.TestCheckResourceAttr("data.gravitino_models.all", "models.0.comment", "This is a comment"),
					resource.TestCheckResourceAttr("data.gravitino_models.all", "models.0.properties.%", "2"),
				),
			},
		},
	})
}

func TestAccModelDataSource_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.URL.Path != "/api/metalakes/probe_ml/catalogs/probe_cat/schemas/probe_sch/models/model1" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(modelResponseExample))
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: modelDSProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_model" "this" {
  metalake = "probe_ml"
  catalog  = "probe_cat"
  schema   = "probe_sch"
  name     = "model1"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_model.this", "comment", "This is a comment"),
					resource.TestCheckResourceAttr("data.gravitino_model.this", "latest_version", "0"),
					resource.TestCheckResourceAttr("data.gravitino_model.this", "properties.%", "2"),
					resource.TestCheckResourceAttr("data.gravitino_model.this", "audit.creator", "user1"),
				),
			},
		},
	})
}
