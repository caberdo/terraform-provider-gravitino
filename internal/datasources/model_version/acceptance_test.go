package model_version_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func modelVersionDSProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// TestAccModelVersionsDataSource_VersionNumbers is the regression test for the
// `versions` key of the spec's ModelVersionListResponse: a server that answers
// the details list with version numbers must still produce a non-empty list.
func TestAccModelVersionsDataSource_VersionNumbers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		collection := "/api/metalakes/probe_ml/catalogs/probe_cat/schemas/probe_sch/models/model1/versions"

		switch {
		case r.URL.Path == collection:
			// ModelVersionListResponse example of the spec, verbatim.
			_, _ = w.Write([]byte(`{
  "code": 0,
  "versions": [0, 1, 2]
}`))
		case strings.HasPrefix(r.URL.Path, collection+"/"):
			version, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, collection+"/"))
			_, _ = w.Write([]byte(modelVersionResponseFor(version)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: modelVersionDSProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_model_versions" "all" {
  metalake = "probe_ml"
  catalog  = "probe_cat"
  schema   = "probe_sch"
  model    = "model1"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_model_versions.all", "versions.#", "3"),
					resource.TestCheckResourceAttr("data.gravitino_model_versions.all", "versions.0.version", "0"),
					resource.TestCheckResourceAttr("data.gravitino_model_versions.all", "versions.1.version", "1"),
					resource.TestCheckResourceAttr("data.gravitino_model_versions.all", "versions.2.version", "2"),
					resource.TestCheckResourceAttr("data.gravitino_model_versions.all", "versions.0.uris.hdfs", "hdfs://path/to/model"),
				),
			},
		},
	})
}

func TestAccModelVersionDataSource_ByAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		if r.URL.Path != "/api/metalakes/probe_ml/catalogs/probe_cat/schemas/probe_sch/models/model1/aliases/alias1" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(modelVersionResponseExample))
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: modelVersionDSProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_model_version" "by_alias" {
  metalake = "probe_ml"
  catalog  = "probe_cat"
  schema   = "probe_sch"
  model    = "model1"
  alias    = "alias1"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_model_version.by_alias", "version", "0"),
					resource.TestCheckResourceAttr("data.gravitino_model_version.by_alias", "uris.hdfs", "hdfs://path/to/model"),
					resource.TestCheckResourceAttr("data.gravitino_model_version.by_alias", "aliases.#", "2"),
				),
			},
		},
	})
}
