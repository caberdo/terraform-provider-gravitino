# List the policies attached to a metadata object. The result includes policies
# inherited from the object's ancestors.
data "gravitino_object_policies" "orders" {
  metalake         = "example_metalake"
  object_type      = "TABLE"
  object_full_name = "sales.orders"
}
