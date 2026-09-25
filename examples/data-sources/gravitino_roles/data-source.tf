data "gravitino_roles" "example" {
  metalake      = "example_metalake"
  resource_type = "TABLE"
  resource      = "catalog1.schema1.table1"
}

output "role_names" {
  value = data.gravitino_roles.example.names
}
