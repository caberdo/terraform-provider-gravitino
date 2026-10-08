package client

import "net/url"

// The helpers below build the URL paths of the Gravitino REST API. Every path
// segment that carries user data is escaped with url.PathEscape, so callers must
// pass the raw names, not pre-escaped ones.

// metalakePath builds /metalakes/{metalake}.
func metalakePath(metalake string) string {
	return "/metalakes/" + url.PathEscape(metalake)
}

// catalogPath builds /metalakes/{metalake}/catalogs/{catalog}.
func catalogPath(metalake, catalog string) string {
	return metalakePath(metalake) + "/catalogs/" + url.PathEscape(catalog)
}

// schemaPath builds /metalakes/{metalake}/catalogs/{catalog}/schemas/{schema}.
func schemaPath(metalake, catalog, schema string) string {
	return catalogPath(metalake, catalog) + "/schemas/" + url.PathEscape(schema)
}

// catalogCollectionPath builds the collection path of a catalog-scoped entity,
// for example .../schemas/{schema}/filesets.
func catalogCollectionPath(metalake, catalog, schema, kind string) string {
	return schemaPath(metalake, catalog, schema) + "/" + kind
}

// catalogEntityPath builds the path of a single catalog-scoped entity, for
// example .../schemas/{schema}/filesets/{name}.
func catalogEntityPath(metalake, catalog, schema, kind, name string) string {
	return catalogCollectionPath(metalake, catalog, schema, kind) + "/" + url.PathEscape(name)
}

// collectionPath builds a metalake-scoped collection path, for example
// /metalakes/{metalake}/roles.
func collectionPath(metalake, kind string) string {
	return metalakePath(metalake) + "/" + kind
}

// entityPath builds a metalake-scoped entity path, for example
// /metalakes/{metalake}/roles/{name}.
func entityPath(metalake, kind, name string) string {
	return collectionPath(metalake, kind) + "/" + url.PathEscape(name)
}

// objectPath builds /metalakes/{metalake}/objects/{objectType}/{objectFullName},
// the prefix of the metadata-object scoped endpoints (statistics, credentials,
// secrets, tags, roles and policies).
func objectPath(metalake, objectType, objectFullName string) string {
	return metalakePath(metalake) + "/objects/" + url.PathEscape(objectType) + "/" + url.PathEscape(objectFullName)
}

// jobRunsPath builds /metalakes/{metalake}/jobs/runs.
func jobRunsPath(metalake string) string {
	return metalakePath(metalake) + "/jobs/runs"
}

// jobRunPath builds /metalakes/{metalake}/jobs/runs/{jobID}.
func jobRunPath(metalake, jobID string) string {
	return jobRunsPath(metalake) + "/" + url.PathEscape(jobID)
}

// jobTemplatesPath builds /metalakes/{metalake}/jobs/templates.
func jobTemplatesPath(metalake string) string {
	return metalakePath(metalake) + "/jobs/templates"
}

// jobTemplatePath builds /metalakes/{metalake}/jobs/templates/{name}.
func jobTemplatePath(metalake, name string) string {
	return jobTemplatesPath(metalake) + "/" + url.PathEscape(name)
}
