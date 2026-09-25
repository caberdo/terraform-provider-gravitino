# Set a user as owner of a catalog.
#
# object_full_name is relative to the metalake: CATALOG takes the bare catalog
# name (a "metalake.catalog" value is rejected with HTTP 400
# IllegalNamespaceException), SCHEMA takes "catalog.schema", TABLE takes
# "catalog.schema.table".
resource "gravitino_owner" "catalog_owner" {
  metalake         = gravitino_metalake.example.name
  object_type      = "CATALOG"
  object_full_name = gravitino_catalog.hive.name
  owner_name       = "data_engineer"
  owner_type       = "USER"
}

# Set a group as owner of a schema
resource "gravitino_owner" "schema_owner" {
  metalake         = gravitino_metalake.example.name
  object_type      = "SCHEMA"
  object_full_name = "${gravitino_catalog.hive.name}.${gravitino_schema.example.name}"
  owner_name       = "engineering"
  owner_type       = "GROUP"
}

# Take ownership of the metalake itself (the full name is the metalake name)
resource "gravitino_owner" "metalake_owner" {
  metalake         = gravitino_metalake.example.name
  object_type      = "METALAKE"
  object_full_name = gravitino_metalake.example.name
  owner_name       = "platform_admin"
  owner_type       = "USER"
}
