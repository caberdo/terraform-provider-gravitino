# The Iceberg REST service endpoint advertised by the Gravitino server
# (GET /api/system/iceberg-rest, Gravitino >= 1.3.1).
#
# `uri` is null when the server has no Iceberg REST service, the service does not
# use the dynamic catalog config provider, or it serves a different metalake.
data "gravitino_iceberg_rest_service" "current" {
  metalake = "my_metalake"
}
