package owner_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLiveAccOwnerResource exercises the owner resource against a real Gravitino server.
// Requires the server to run with authorization enabled
// (GRAVITINO_AUTHORIZATION_ENABLE=true), otherwise /metalakes/{m}/owners/... answers
// HTTP 405.
//
// The assertions also pin the object full name convention: it is relative to the
// metalake (a catalog is addressed as "my_catalog", not "my_metalake.my_catalog").
// A metalake-prefixed value is rejected by the server with HTTP 400
// IllegalNamespaceException.
func TestLiveAccOwnerResource(t *testing.T) {
	mlName := acceptance.UniqueName("ownml")
	catName := acceptance.UniqueName("owncat")
	groupName := acceptance.UniqueName("owngrp")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live owner metalake"
}

resource "gravitino_group" "owner" {
  metalake = gravitino_metalake.this.name
  name     = %[3]q
}

resource "gravitino_catalog" "this" {
  metalake         = gravitino_metalake.this.name
  name             = %[2]q
  type             = "fileset"
  catalog_provider = "fileset"
}

resource "gravitino_owner" "catalog" {
  metalake         = gravitino_metalake.this.name
  object_type      = "CATALOG"
  object_full_name = gravitino_catalog.this.name
  owner_name       = gravitino_group.owner.name
  owner_type       = "GROUP"
}
`, mlName, catName, groupName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_owner.catalog", "object_type", "CATALOG"),
					resource.TestCheckResourceAttr("gravitino_owner.catalog", "object_full_name", catName),
					resource.TestCheckResourceAttr("gravitino_owner.catalog", "owner_name", groupName),
					// The server serialises the owner type in lower case; the provider
					// exposes the canonical upper case form.
					resource.TestCheckResourceAttr("gravitino_owner.catalog", "owner_type", "GROUP"),
					resource.TestCheckResourceAttr("gravitino_owner.catalog", "id", mlName+".CATALOG."+catName),
				),
			},
			{
				RefreshState: true,
				Check:        resource.TestCheckResourceAttr("gravitino_owner.catalog", "owner_name", groupName),
			},
		},
	})
}
