data "gravitino_functions" "example" {
  metalake = "example_metalake"
  catalog  = "hive_catalog"
  schema   = "example_schema"
}

output "function_names" {
  description = "The names of the functions in the schema."
  value       = [for function in data.gravitino_functions.example.functions : function.name]
}

output "table_functions" {
  description = "The functions that return columns instead of a single return type."
  value = [
    for function in data.gravitino_functions.example.functions : function.name
    if function.function_type == "TABLE"
  ]
}
