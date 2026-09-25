package models

// Metadata object types accepted by the `metadataObjectType` path parameter of
// the credentials endpoint
// (docs/open-api/openapi.yaml#/components/parameters/metadataObjectType).
//
// These are intentionally credential-specific constants: the credentials
// endpoint has its own enum and must not silently follow changes made to the
// privileges/statistics/owner object type lists.
const (
	CredentialObjectTypeMetalake = "METALAKE"
	CredentialObjectTypeCatalog  = "CATALOG"
	CredentialObjectTypeSchema   = "SCHEMA"
	CredentialObjectTypeTable    = "TABLE"
	CredentialObjectTypeColumn   = "COLUMN"
	CredentialObjectTypeFileset  = "FILESET"
	CredentialObjectTypeTopic    = "TOPIC"
	CredentialObjectTypeModel    = "MODEL"
	CredentialObjectTypeRole     = "ROLE"
)

// CredentialObjectTypes is the exact enum of the metadataObjectType path
// parameter of the Gravitino v1.3.0 credentials endpoint.
var CredentialObjectTypes = []string{
	CredentialObjectTypeMetalake,
	CredentialObjectTypeCatalog,
	CredentialObjectTypeSchema,
	CredentialObjectTypeTable,
	CredentialObjectTypeColumn,
	CredentialObjectTypeFileset,
	CredentialObjectTypeTopic,
	CredentialObjectTypeModel,
	CredentialObjectTypeRole,
}
