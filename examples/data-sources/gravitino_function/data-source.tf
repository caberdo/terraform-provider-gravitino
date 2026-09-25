data "gravitino_function" "example" {
  metalake = "example_metalake"
  catalog  = "hive_catalog"
  schema   = "example_schema"
  name     = "add_one"
}

output "function_type" {
  description = "The type of the function: SCALAR, AGGREGATE or TABLE."
  value       = data.gravitino_function.example.function_type
}

output "function_definitions" {
  description = "The definitions of the function, including parameters, return type and implementations."
  value       = data.gravitino_function.example.definitions
}
