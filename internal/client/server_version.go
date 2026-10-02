package client

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// serverVersion is a Gravitino release version. The patch component is kept
// because version specific behaviour differs within a minor release: the
// external data type exists in 1.3.1 but not in 1.3.0.
type serverVersion struct {
	major int
	minor int
	patch int
}

// DetectServerVersion records the version reported by GET /api/version.
//
// The probe is best effort: Configure logs a failure and continues with an
// unknown version, which the feature gates treat as "supported" so a failed
// probe cannot reject a configuration the server would accept.
func (c *Client) DetectServerVersion(ctx context.Context) error {
	resp, err := c.GetVersion(ctx)
	if err != nil {
		return err
	}

	version, err := parseServerVersion(resp.Version.Version)
	if err != nil {
		return err
	}

	c.versionMu.Lock()
	c.version = version
	c.versionMu.Unlock()
	return nil
}

// ServerVersion returns the detected Gravitino version, or the empty string when
// the server did not answer GET /api/version.
func (c *Client) ServerVersion() string {
	c.versionMu.RLock()
	defer c.versionMu.RUnlock()

	if c.version == nil {
		return ""
	}
	return c.version.String()
}

// AtLeast reports whether the detected server is at least major.minor.patch. An
// unknown server version is treated as recent enough, so a failed version probe
// never blocks a feature the server may well support.
func (c *Client) AtLeast(major, minor, patch int) bool {
	c.versionMu.RLock()
	defer c.versionMu.RUnlock()

	if c.version == nil {
		return true
	}
	return c.version.atLeast(major, minor, patch)
}

// SupportsExternalType reports whether the server accepts the "external" data
// type variant, which Gravitino added in 1.3.1.
func (c *Client) SupportsExternalType() bool {
	return c.AtLeast(1, 3, 1)
}

func (v serverVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}

func (v serverVersion) atLeast(major, minor, patch int) bool {
	if v.major != major {
		return v.major > major
	}
	if v.minor != minor {
		return v.minor > minor
	}
	return v.patch >= patch
}

// parseServerVersion parses "major.minor.patch" and ignores a suffix such as
// "-SNAPSHOT"; a missing component counts as zero.
func parseServerVersion(raw string) (*serverVersion, error) {
	core := strings.TrimSpace(raw)
	if core == "" {
		return nil, fmt.Errorf("the server reported an empty version")
	}
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}

	parts := strings.Split(core, ".")
	if len(parts) > 3 {
		return nil, fmt.Errorf("unexpected server version %q", raw)
	}

	numbers := [3]int{}
	for i, part := range parts {
		number, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("unexpected server version %q: %w", raw, err)
		}
		numbers[i] = number
	}

	return &serverVersion{major: numbers[0], minor: numbers[1], patch: numbers[2]}, nil
}
