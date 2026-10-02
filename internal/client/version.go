package client

import (
	"context"
	"strconv"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

// serverVersion is the Gravitino server version detected at provider
// configuration time. GET /api/version reports `<major>.<minor>.<patch>`
// (for example "1.3.0"), optionally with a pre-release suffix such as
// "-SNAPSHOT". The suffix is ignored: gating decides whether a feature is part
// of the release line, not how the server binary was built.
type serverVersion struct {
	major, minor, patch int
	raw                 string
}

func (c *Client) GetVersion(ctx context.Context) (*models.VersionResponse, error) {
	var result models.VersionResponse
	err := c.Get(ctx, "/version", &result)
	return &result, err
}

// SetServerVersion records the version reported by GET /api/version. It is
// called once during provider configuration. A version that cannot be parsed
// leaves the client versionless, in which case version-gated attributes are
// not rejected client-side; the server stays the authority.
func (c *Client) SetServerVersion(version string) {
	parsed, ok := parseServerVersion(version)
	if !ok {
		return
	}
	c.serverVersion = &parsed
}

// ServerVersion returns the raw version string reported by the server, or ""
// when it is unknown (not detected, or unparseable).
func (c *Client) ServerVersion() string {
	if c.serverVersion == nil {
		return ""
	}
	return c.serverVersion.raw
}

// AtLeast reports whether the detected server version is at least
// major.minor.patch. It reports false when the version is unknown; callers that
// gate features on a version must distinguish "unknown" (do not gate) from
// "too old" (reject) via ServerVersion().
func (c *Client) AtLeast(major, minor, patch int) bool {
	if c.serverVersion == nil {
		return false
	}
	got := *c.serverVersion
	if got.major != major {
		return got.major > major
	}
	if got.minor != minor {
		return got.minor > minor
	}
	return got.patch >= patch
}

// parseServerVersion parses a Gravitino version string of the form
// `<major>[.<minor>[.<patch>]]`, tolerating a leading "v" and a pre-release or
// build suffix ("1.3.1-SNAPSHOT", "1.3.0+build.1").
func parseServerVersion(version string) (serverVersion, bool) {
	core := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	if core == "" {
		return serverVersion{}, false
	}

	parts := strings.Split(core, ".")
	if len(parts) > 3 {
		return serverVersion{}, false
	}

	var numbers [3]int
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return serverVersion{}, false
		}
		numbers[i] = n
	}

	return serverVersion{major: numbers[0], minor: numbers[1], patch: numbers[2], raw: version}, true
}
