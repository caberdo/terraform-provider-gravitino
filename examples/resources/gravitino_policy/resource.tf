# A custom policy is applied to metadata objects (catalogs, schemas, tables,
# filesets, topics, models) and managed through the custom policy content.
resource "gravitino_policy" "data_retention" {
  metalake = gravitino_metalake.example.name
  name     = "data_retention"
  comment  = "Retain data for compliance"

  policy_type = "custom"
  enabled     = true

  # One or more of CATALOG, SCHEMA, TABLE, FILESET, TOPIC, MODEL.
  supported_object_types = ["CATALOG", "SCHEMA", "TABLE"]

  properties = {
    key1 = "value1"
  }

  # The API accepts any JSON value per rule; because this map is string-typed in
  # Terraform, non-string server values are rendered as their JSON encoding
  # (the server value 123 is read back as "123").
  custom_rules = {
    rule1 = "123"
  }
}
