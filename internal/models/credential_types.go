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
	CredentialObjectTypeView     = "VIEW"
	CredentialObjectTypeColumn   = "COLUMN"
	CredentialObjectTypeFileset  = "FILESET"
	CredentialObjectTypeTopic    = "TOPIC"
	CredentialObjectTypeModel    = "MODEL"
	CredentialObjectTypeFunction = "FUNCTION"
	CredentialObjectTypeRole     = "ROLE"
)

// CredentialObjectTypes is the enum of the metadataObjectType path parameter of
// the Gravitino credentials endpoint, i.e. the v1.3.1 enum. VIEW and FUNCTION
// were added by 1.3.1 (the 1.3.0 enum stops at ROLE); see
// ObjectTypeRequiresGravitino131, which the credentials data source uses to
// reject them against an older server.
var CredentialObjectTypes = []string{
	CredentialObjectTypeMetalake,
	CredentialObjectTypeCatalog,
	CredentialObjectTypeSchema,
	CredentialObjectTypeTable,
	CredentialObjectTypeView,
	CredentialObjectTypeColumn,
	CredentialObjectTypeFileset,
	CredentialObjectTypeTopic,
	CredentialObjectTypeModel,
	CredentialObjectTypeFunction,
	CredentialObjectTypeRole,
}
