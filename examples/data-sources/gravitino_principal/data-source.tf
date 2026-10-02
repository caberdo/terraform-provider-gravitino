# The authenticated principal and service-admin status, as resolved by the
# server (GET /api/authn/me). `name` is the principal; `service_admin` is
# `true` only when the server (Gravitino 1.3.1+) reports the credential as a
# service administrator. The endpoint returns no roles here.
data "gravitino_principal" "current" {}
