# Gravitino stores statistics per metadata object. Reserved statistics such as
# row_count are supplied by the catalog and cannot be written through Gravitino;
# this resource owns the custom-* statistics, and a write outside Terraform is
# either re-applied or dropped on the next apply.
resource "gravitino_statistics" "orders" {
  metalake         = gravitino_metalake.example.name
  object_type      = "TABLE"
  object_full_name = "sales.orders"

  statistics = {
    "custom-row-count"  = "1000"
    "custom-total-size" = "20480"
  }
}
