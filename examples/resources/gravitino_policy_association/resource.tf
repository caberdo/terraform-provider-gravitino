# Attach existing policies directly to a metadata object. Each policy must list
# the object type in its supported_object_types. Adding or removing entries
# updates the association in place; destroy detaches every policy this resource
# manages.
resource "gravitino_policy_association" "orders_masking" {
  metalake         = gravitino_metalake.example.name
  object_type      = "TABLE"
  object_full_name = "sales.orders"
  policies         = [gravitino_policy.data_retention.name]
}
