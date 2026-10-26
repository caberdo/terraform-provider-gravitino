# List the tags attached to a metadata object. The result includes tags
# inherited from the object's ancestors.
data "gravitino_object_tags" "orders" {
  metalake         = "example_metalake"
  object_type      = "TABLE"
  object_full_name = "sales.orders"
}
