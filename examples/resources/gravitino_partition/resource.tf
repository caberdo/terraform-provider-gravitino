resource "gravitino_partition" "q1_2024" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.example.name
  table    = gravitino_table.partitioned.name

  # Gravitino discriminates partitions by type: identity, range or list.
  type        = "identity"
  field_names = [["order_date"]]
  values = [
    {
      data_type = "date"
      value     = "2024-01-01"
    }
  ]
}

resource "gravitino_partition" "q2_2024" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.example.name
  table    = gravitino_table.partitioned.name

  type        = "identity"
  field_names = [["order_date"]]
  values = [
    {
      data_type = "date"
      value     = "2024-04-01"
    }
  ]
}

resource "gravitino_partition" "eu_region" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.example.name
  table    = gravitino_table.sorted.name

  type        = "identity"
  field_names = [["region"]]
  values = [
    {
      data_type = "string"
      value     = "eu"
    }
  ]
}

resource "gravitino_partition" "us_region" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.example.name
  table    = gravitino_table.sorted.name

  type        = "identity"
  field_names = [["region"]]
  values = [
    {
      data_type = "string"
      value     = "us"
    }
  ]
}
