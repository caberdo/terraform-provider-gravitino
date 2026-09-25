# The metalake, catalog and schema are referenced by name here; in a full
# configuration they can also reference the managed resources, for example
# metalake = gravitino_metalake.example.name.
resource "gravitino_function" "add_one" {
  metalake      = "example_metalake"
  catalog       = "hive_catalog"
  schema        = "example_schema"
  name          = "add_one"
  function_type = "SCALAR"
  deterministic = true
  comment       = "A simple scalar function that adds one"

  definitions = [
    {
      parameters = [
        {
          name      = "x"
          data_type = "integer"
        }
      ]
      return_type = "integer"

      impls = [
        {
          language = "SQL"
          runtime  = "SPARK"
          sql      = "x + 1"
        }
      ]
    }
  ]
}

# A table function returns columns instead of a single return type, and its
# implementation can be a SQL expression, a JAVA class or Python code.
resource "gravitino_function" "generate_series" {
  metalake      = "example_metalake"
  catalog       = "hive_catalog"
  schema        = "example_schema"
  name          = "generate_series"
  function_type = "TABLE"
  deterministic = true
  comment       = "A table function that generates a series of integers"

  definitions = [
    {
      parameters = [
        {
          name      = "start_val"
          data_type = "integer"
        },
        {
          name      = "end_val"
          data_type = "integer"
        }
      ]

      return_columns = [
        {
          name      = "value"
          data_type = "integer"
          comment   = "The generated integer value"
        }
      ]

      impls = [
        {
          language   = "JAVA"
          runtime    = "SPARK"
          class_name = "com.example.GenerateSeriesFunction"
          resources = {
            jars = ["hdfs:///path/to/udtf.jar"]
          }
        }
      ]
    }
  ]
}

output "add_one_id" {
  description = "The compound identifier of the registered scalar function."
  value       = gravitino_function.add_one.id
}

output "generate_series_return_columns" {
  description = "The columns the table function returns."
  value       = gravitino_function.generate_series.definitions[0].return_columns
}
