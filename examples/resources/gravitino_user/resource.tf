# Minimal user. The API has no user update endpoint, so changing the name or
# metalake replaces the resource.
resource "gravitino_user" "analyst" {
  metalake = gravitino_metalake.example.name
  name     = "data_analyst"
}

# User with roles assigned. Roles are granted and revoked in-place through the
# permissions API.
resource "gravitino_user" "engineer" {
  metalake = gravitino_metalake.example.name
  name     = "data_engineer"
  roles    = ["admin", "readonly"]
}
