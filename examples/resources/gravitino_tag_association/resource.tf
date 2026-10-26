# Attach existing tags directly to a metadata object. The object is identified
# by its type and its full name relative to the metalake (without the metalake
# prefix). Adding or removing entries updates the association in place; destroy
# detaches every tag this resource manages.
resource "gravitino_tag_association" "orders_pii" {
  metalake         = gravitino_metalake.example.name
  object_type      = "TABLE"
  object_full_name = "sales.orders"
  tags             = [gravitino_tag.pii.name, gravitino_tag.confidential.name]
}

# A column carries tags with its own full name, including the table name.
resource "gravitino_tag_association" "orders_email_column" {
  metalake         = gravitino_metalake.example.name
  object_type      = "COLUMN"
  object_full_name = "sales.orders.email"
  tags             = [gravitino_tag.pii.name]
}
