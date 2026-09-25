data "gravitino_model" "example" {
  metalake = "example_metalake"
  catalog  = "ml_catalog"
  schema   = "example_schema"
  name     = "fraud_detector"
}

output "latest_version" {
  description = "The latest version number of the model. Model artifacts live on the model versions."
  value       = data.gravitino_model.example.latest_version
}
