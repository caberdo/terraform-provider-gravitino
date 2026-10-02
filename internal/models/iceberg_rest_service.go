package models

// IcebergRESTServiceResponse is the response of GET /api/system/iceberg-rest
// (Gravitino v1.3.1, docs/open-api/system.yaml,
// components/schemas/IcebergRESTServiceResponse).
//
// URI is nullable: the server reports null when no Iceberg REST service is
// registered, the service does not use the dynamic catalog config provider, or
// it serves a different metalake than the requested one.
type IcebergRESTServiceResponse struct {
	Code int     `json:"code"`
	URI  *string `json:"uri"`
}
