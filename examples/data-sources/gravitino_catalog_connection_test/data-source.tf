# Test a proposed catalog configuration before creating the catalog.
data "gravitino_catalog_connection_test" "proposed" {
  metalake         = "example_metalake"
  name             = "hive_catalog"
  type             = "relational"
  catalog_provider = "hive"
  comment          = "A Hive catalog"

  properties = {
    "metastore.uris" = "thrift://127.0.0.1:9083"
  }
}

# Test the stored configuration of an existing catalog.
# Requires Gravitino 1.3.1 or newer.
data "gravitino_catalog_connection_test" "existing" {
  metalake = "example_metalake"
  catalog  = "hive_catalog"
}

output "proposed_connection_succeeded" {
  value = data.gravitino_catalog_connection_test.proposed.success
}

output "existing_connection_message" {
  value = data.gravitino_catalog_connection_test.existing.message
}
