resource "gravitino_model" "example" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.ml.name
  schema   = gravitino_schema.example.name
  name     = "fraud_detector"
  comment  = "ML model for fraud detection"
  properties = {
    "framework" = "pytorch"
  }
}

output "latest_model_version" {
  description = "The version number Gravitino assigned to the newest model version."
  value       = gravitino_model.example.latest_version
}
