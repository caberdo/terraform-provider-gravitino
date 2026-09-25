# Minimal role: the securable object list may be empty.
resource "gravitino_role" "readonly" {
  metalake          = gravitino_metalake.example.name
  name              = "readonly"
  securable_objects = []
}

# Role with securable objects and their privileges.
#
# `full_name` is relative to the metalake, so it never carries the metalake prefix:
#   METALAKE -> the metalake name itself
#   CATALOG  -> "my_catalog"
#   SCHEMA   -> "my_catalog.my_schema"
#   TABLE    -> "my_catalog.my_schema.my_table"
resource "gravitino_role" "data_engineer" {
  metalake = gravitino_metalake.example.name
  name     = "data_engineer"
  securable_objects = [
    {
      full_name = "catalog1"
      type      = "CATALOG"
      privileges = [
        { name = "USE_CATALOG", condition = "ALLOW" }
      ]
    },
    {
      full_name = "catalog1.schema1"
      type      = "SCHEMA"
      privileges = [
        { name = "USE_SCHEMA", condition = "ALLOW" },
        { name = "CREATE_TABLE", condition = "ALLOW" }
      ]
    },
    {
      full_name = "catalog1.schema1.table1"
      type      = "TABLE"
      privileges = [
        { name = "SELECT_TABLE", condition = "ALLOW" }
      ]
    }
  ]
}

# Role with properties. Gravitino has no role-property update API, so changing
# `properties` destroys and recreates the role.
resource "gravitino_role" "admin" {
  metalake = gravitino_metalake.example.name
  name     = "admin"
  properties = {
    "description" = "Full access role"
    "managed_by"  = "security-team"
  }
  securable_objects = [
    {
      # A METALAKE securable object is the metalake itself.
      full_name = gravitino_metalake.example.name
      type      = "METALAKE"
      privileges = [
        { name = "CREATE_CATALOG", condition = "ALLOW" },
        { name = "MANAGE_USERS", condition = "ALLOW" },
        { name = "MANAGE_GROUPS", condition = "ALLOW" },
        { name = "CREATE_ROLE", condition = "ALLOW" },
        { name = "MANAGE_GRANTS", condition = "ALLOW" }
      ]
    }
  ]
}
