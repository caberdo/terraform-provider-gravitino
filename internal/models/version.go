package models

import (
	"strconv"
	"strings"
)

type VersionResponse struct {
	Code    int     `json:"code"`
	Version Version `json:"version"`
}

type Version struct {
	Version     string `json:"version"`
	CompileDate string `json:"compileDate"`
	GitCommit   string `json:"gitCommit"`
}

// ServerVersionAtLeast reports whether a Gravitino server version string as
// returned by GET /api/version (e.g. "1.3.1", "v1.3.1", "1.4.0-SNAPSHOT") is at
// least major.minor.patch. Any pre-release/build suffix is ignored, so an
// incubating or snapshot build of a release counts as that release. A version
// that cannot be parsed reports false: the caller is about to use an API that
// only exists from that version on, so an unknown server version must not be
// assumed to support it.
func ServerVersionAtLeast(version string, major, minor, patch int) bool {
	parsed, ok := parseServerVersion(version)
	if !ok {
		return false
	}
	if parsed[0] != major {
		return parsed[0] > major
	}
	if parsed[1] != minor {
		return parsed[1] > minor
	}
	return parsed[2] >= patch
}

// ObjectTypeRequiresGravitino131 reports whether a statistics/credentials
// `metadataObjectType` value only exists from Gravitino 1.3.1 on: VIEW and
// FUNCTION were added to the v1.3.0 enum (which stops at ROLE) in v1.3.1.
func ObjectTypeRequiresGravitino131(objectType string) bool {
	switch CanonicalObjectType(objectType) {
	case ObjectTypeView, ObjectTypeFunction:
		return true
	default:
		return false
	}
}

// ValidServerVersion reports whether version is a Gravitino version string that
// ServerVersionAtLeast can compare: a leading "v" and a pre-release or build
// suffix are accepted, a missing component counts as zero.
func ValidServerVersion(version string) bool {
	_, ok := parseServerVersion(version)
	return ok
}

// IndexTypeRequiresGravitino131 reports whether an indexes.yaml#/IndexSpec
// `indexType` value only exists from Gravitino 1.3.1 on: the data-skipping index
// types were added to the v1.3.0 enum (which stops at unique_key) in v1.3.1,
// together with the optional `properties` of an index.
func IndexTypeRequiresGravitino131(indexType string) bool {
	switch strings.ToLower(strings.TrimSpace(indexType)) {
	case "data_skipping_minmax", "data_skipping_bloom_filter", "data_skipping_set":
		return true
	default:
		return false
	}
}

// parseServerVersion splits a version string into its major, minor and patch
// numbers, ignoring a leading "v" and any "-suffix"/"+build" metadata.
func parseServerVersion(version string) ([3]int, bool) {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return [3]int{}, false
	}

	var parsed [3]int
	for i, part := range strings.Split(v, ".") {
		if i >= len(parsed) {
			break
		}
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		parsed[i] = n
	}
	return parsed, true
}
