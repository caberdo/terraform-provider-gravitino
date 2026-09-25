resource "gravitino_view" "user_summary" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.example.name
  name     = "user_summary"
  comment  = "View aggregating user data"

  column = [{
    name    = "id"
    type    = "long"
    comment = "user id"
  }]

  representation = [{
    type    = "sql"
    dialect = "trino"
    sql     = "SELECT u.id, COUNT(o.order_id) AS order_count FROM users u LEFT JOIN orders o ON u.id = o.customer_id GROUP BY u.id"
  }]
}

resource "gravitino_view" "active_customers" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.example.name
  name     = "active_customers"
  comment  = "Customers with orders in last 90 days"

  column = [{
    name     = "id"
    type     = "long"
    comment  = "customer id"
    nullable = false
  }]

  representation = [{
    type    = "sql"
    dialect = "trino"
    sql     = "SELECT DISTINCT u.id, u.email FROM users u INNER JOIN orders o ON u.id = o.customer_id WHERE o.order_date >= current_date - INTERVAL '90' DAY"
  }]

  default_catalog = gravitino_catalog.hive.name
  default_schema  = gravitino_schema.example.name

  properties = {
    "refresh" = "daily"
    "owner"   = "analytics-team"
  }
}
