package models

import "strings"

const (
	PrivilegeCreateCatalog    = "CREATE_CATALOG"
	PrivilegeUseCatalog       = "USE_CATALOG"
	PrivilegeCreateSchema     = "CREATE_SCHEMA"
	PrivilegeUseSchema        = "USE_SCHEMA"
	PrivilegeCreateTable      = "CREATE_TABLE"
	PrivilegeModifyTable      = "MODIFY_TABLE"
	PrivilegeSelectTable      = "SELECT_TABLE"
	PrivilegeCreateFileset    = "CREATE_FILESET"
	PrivilegeWriteFileset     = "WRITE_FILESET"
	PrivilegeReadFileset      = "READ_FILESET"
	PrivilegeCreateTopic      = "CREATE_TOPIC"
	PrivilegeProduceTopic     = "PRODUCE_TOPIC"
	PrivilegeConsumeTopic     = "CONSUME_TOPIC"
	PrivilegeManageUsers      = "MANAGE_USERS"
	PrivilegeManageGroups     = "MANAGE_GROUPS"
	PrivilegeCreateRole       = "CREATE_ROLE"
	PrivilegeManageGrants     = "MANAGE_GRANTS"
	PrivilegeRegisterModel    = "REGISTER_MODEL"
	PrivilegeLinkModelVersion = "LINK_MODEL_VERSION"
	PrivilegeUseModel         = "USE_MODEL"
	PrivilegeRegisterFunction = "REGISTER_FUNCTION"
	PrivilegeExecuteFunction  = "EXECUTE_FUNCTION"
	PrivilegeModifyFunction   = "MODIFY_FUNCTION"
	PrivilegeCreateTag        = "CREATE_TAG"
	PrivilegeApplyTag         = "APPLY_TAG"
	PrivilegeCreatePolicy     = "CREATE_POLICY"
	PrivilegeApplyPolicy      = "APPLY_POLICY"
	PrivilegeRegisterJob      = "REGISTER_JOB_TEMPLATE"
	PrivilegeUseJob           = "USE_JOB_TEMPLATE"
	PrivilegeRunJob           = "RUN_JOB"
)

var AllPrivileges = []string{
	PrivilegeCreateCatalog,
	PrivilegeUseCatalog,
	PrivilegeCreateSchema,
	PrivilegeUseSchema,
	PrivilegeCreateTable,
	PrivilegeModifyTable,
	PrivilegeSelectTable,
	PrivilegeCreateFileset,
	PrivilegeWriteFileset,
	PrivilegeReadFileset,
	PrivilegeCreateTopic,
	PrivilegeProduceTopic,
	PrivilegeConsumeTopic,
	PrivilegeManageUsers,
	PrivilegeManageGroups,
	PrivilegeCreateRole,
	PrivilegeManageGrants,
	PrivilegeRegisterModel,
	PrivilegeLinkModelVersion,
	PrivilegeUseModel,
	PrivilegeRegisterFunction,
	PrivilegeExecuteFunction,
	PrivilegeModifyFunction,
	PrivilegeCreateTag,
	PrivilegeApplyTag,
	PrivilegeCreatePolicy,
	PrivilegeApplyPolicy,
	PrivilegeRegisterJob,
	PrivilegeUseJob,
	PrivilegeRunJob,
}

const (
	ObjectTypeMetalake    = "METALAKE"
	ObjectTypeCatalog     = "CATALOG"
	ObjectTypeSchema      = "SCHEMA"
	ObjectTypeTable       = "TABLE"
	ObjectTypeColumn      = "COLUMN"
	ObjectTypeFileset     = "FILESET"
	ObjectTypeTopic       = "TOPIC"
	ObjectTypeRole        = "ROLE"
	ObjectTypeModel       = "MODEL"
	ObjectTypeFunction    = "FUNCTION"
	ObjectTypeTag         = "TAG"
	ObjectTypePolicy      = "POLICY"
	ObjectTypeJobTemplate = "JOB_TEMPLATE"
)

// AllObjectTypes holds the metadata object types accepted in the
// `metadataObjectType` path parameter and in SecurableObject.type, exactly as listed
// in the enums of roles.yaml (metadataObjectTypeOfRole) and openapi.yaml
// (metadataObjectType): METALAKE, CATALOG, SCHEMA, TABLE, FILESET, TOPIC, ROLE,
// MODEL, FUNCTION, TAG, POLICY and JOB_TEMPLATE.
//
// Gravitino is case-insensitive on input (it parses the value with
// `MetadataObject.Type.valueOf(type.toUpperCase(Locale.ROOT))`) but always serialises
// lower case in its responses, so resources and data sources normalise the values of a
// response with CanonicalObjectType before they reach the Terraform state.
var AllObjectTypes = []string{
	ObjectTypeMetalake,
	ObjectTypeCatalog,
	ObjectTypeSchema,
	ObjectTypeTable,
	ObjectTypeFileset,
	ObjectTypeTopic,
	ObjectTypeRole,
	ObjectTypeModel,
	ObjectTypeFunction,
	ObjectTypeTag,
	ObjectTypePolicy,
	ObjectTypeJobTemplate,
}

// CanonicalObjectType returns the upper-case spelling of a metadata object type. It is
// the spelling of the spec enums and the spelling this provider reports in the state:
// Gravitino accepts any casing on input but always answers with lower case, which would
// otherwise cause "Provider produced inconsistent result after apply".
func CanonicalObjectType(objectType string) string {
	return strings.ToUpper(strings.TrimSpace(objectType))
}

// CanonicalPrivilege returns the upper-case spelling of a privilege name or condition
// (Privilege.name and Privilege.condition), for the same reason as CanonicalObjectType.
func CanonicalPrivilege(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

const (
	PrivilegeConditionAllow = "ALLOW"
	PrivilegeConditionDeny  = "DENY"
)

const (
	OwnerTypeUser  = "USER"
	OwnerTypeGroup = "GROUP"
)

var OwnerObjectTypes = []string{
	ObjectTypeMetalake,
	ObjectTypeCatalog,
	ObjectTypeSchema,
	ObjectTypeTable,
	ObjectTypeFileset,
	ObjectTypeTopic,
	ObjectTypeRole,
}

var StatisticsObjectTypes = []string{
	ObjectTypeMetalake,
	ObjectTypeCatalog,
	ObjectTypeSchema,
	ObjectTypeTable,
	ObjectTypeColumn,
	ObjectTypeFileset,
	ObjectTypeTopic,
	ObjectTypeModel,
	ObjectTypeRole,
}
