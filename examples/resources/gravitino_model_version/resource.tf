# Gravitino assigns the version number, so the artifact location is the only
# thing that has to be configured. Either uri (the unnamed uri) or uris (a map of
# uri name to uri) must be set.
resource "gravitino_model_version" "v1" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.ml.name
  schema   = gravitino_schema.example.name
  model    = gravitino_model.example.name
  uri      = "s3://models/fraud_detector/v1.0"
  aliases  = ["production", "latest"]
  comment  = "Initial production version"
}

resource "gravitino_model_version" "v2" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.ml.name
  schema   = gravitino_schema.example.name
  model    = gravitino_model.example.name
  uris = {
    "s3"  = "s3://models/fraud_detector/v2.0"
    "gcs" = "gs://models/fraud_detector/v2.0"
  }
  aliases = ["staging"]
  comment = "Improved model with new features"
  properties = {
    "accuracy"  = "0.95"
    "framework" = "pytorch"
  }
}

output "model_version_numbers" {
  description = "The version numbers Gravitino assigned to the linked model versions."
  value       = [gravitino_model_version.v1.version, gravitino_model_version.v2.version]
}
