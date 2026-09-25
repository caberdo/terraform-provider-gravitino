# Minimal group. The API has no group update endpoint, so changing the name or
# metalake replaces the resource.
resource "gravitino_group" "analysts" {
  metalake = gravitino_metalake.example.name
  name     = "analytics_team"
}

# Group with roles assigned. Roles are granted and revoked in-place through the
# permissions API.
resource "gravitino_group" "engineers" {
  metalake = gravitino_metalake.example.name
  name     = "engineering"
  roles    = ["admin"]
}
