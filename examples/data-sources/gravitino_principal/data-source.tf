# The authenticated principal, as resolved by the server (GET /api/authn/me).
# The endpoint returns the principal name only, so no roles are exposed here.
data "gravitino_principal" "current" {}
