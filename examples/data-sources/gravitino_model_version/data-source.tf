data "gravitino_model_version" "example" {
  metalake = "example_metalake"
  catalog  = "ml_catalog"
  schema   = "example_schema"
  model    = "fraud_detector"
  version  = 0
}

# A model version can also be looked up by one of its aliases.
data "gravitino_model_version" "production" {
  metalake = "example_metalake"
  catalog  = "ml_catalog"
  schema   = "example_schema"
  model    = "fraud_detector"
  alias    = "production"
}

output "production_version_uris" {
  value = data.gravitino_model_version.production.uris
}
