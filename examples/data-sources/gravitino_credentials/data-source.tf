# `credentials` is a list of the credential objects held by the metadata object;
# each entry has credential_type, expire_time_in_ms and (sensitive) credential_info.
data "gravitino_credentials" "example" {
  metalake      = "example_metalake"
  resource_type = "TABLE"
  resource      = "hive_catalog.example_schema.users"
}
