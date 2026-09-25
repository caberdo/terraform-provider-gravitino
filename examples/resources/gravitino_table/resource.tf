resource "gravitino_table" "users" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.example.name
  name     = "users"
  comment  = "User table"

  column {
    name     = "id"
    type     = "integer"
    nullable = false
  }
  column {
    name = "name"
    type = "string"
  }
  column {
    name = "email"
    type = "string"
  }
}

resource "gravitino_table" "partitioned" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.example.name
  name     = "orders"
  comment  = "Orders table with bucket partitioning"

  column {
    name = "order_id"
    type = "string"
  }
  column {
    name = "customer_id"
    type = "integer"
  }
  column {
    name = "amount"
    type = "double"
  }

  partitioning {
    strategy    = "bucket"
    field_name  = ["customer_id"]
    num_buckets = 16
  }
}

resource "gravitino_table" "sorted" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.example.name
  name     = "events"
  comment  = "Events table with range partitioning, distribution and a sort order"

  column {
    name = "event_id"
    type = "string"
  }
  column {
    name = "event_type"
    type = "string"
  }
  column {
    name = "user_id"
    type = "integer"
  }
  column {
    name = "event_time"
    type = "timestamp"
  }

  partitioning {
    strategy   = "range"
    field_name = ["event_time"]
  }

  distribution {
    strategy = "hash"
    number   = 4
  }

  sort_order {
    field_name = ["event_time"]
    direction  = "desc"
  }
  sort_order {
    field_name = ["event_type"]
    direction  = "asc"
  }
}
