package client

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// DetectServerVersion records the version reported by GET /api/version.
//
// The probe is best effort: Configure logs a failure and continues with an
// unknown version. The two kinds of gate then diverge. The permissive gates
// (AtLeast / SupportsExternalType) treat an unknown version as supported, so a
// failed probe cannot reject a configuration the server would accept. The
// fail-closed gates (CheckMetadataObjectTypeSupported, CheckPolicyObjectTypes
// Supported, CheckIndexesSupported) re-probe through ResolveServerVersion and
// return an error when the version is still unknown, so a restricted value is
// never sent to a server of unknown version.
func (c *Client) DetectServerVersion(ctx context.Context) error {
	resp, err := c.GetVersion(ctx)
	if err != nil {
		return err
	}

	version := resp.Version.Version
	if !models.ValidServerVersion(version) {
		return fmt.Errorf("the server reported an unparsable version %q", version)
	}

	c.setServerVersion(version)
	return nil
}

// ServerVersion returns the detected Gravitino version, or the empty string when
// the server did not answer GET /api/version.
func (c *Client) ServerVersion() string {
	c.versionMu.RLock()
	defer c.versionMu.RUnlock()

	return c.version
}

// AtLeast reports whether the detected server is at least major.minor.patch. An
// unknown or unparsable version is treated as recent enough, so a failed version
// probe never blocks a feature the server may well support. Callers that must
// not assume support compare models.ServerVersionAtLeast directly, which fails
// closed.
func (c *Client) AtLeast(major, minor, patch int) bool {
	version := c.ServerVersion()
	if !models.ValidServerVersion(version) {
		return true
	}
	return models.ServerVersionAtLeast(version, major, minor, patch)
}

// SupportsExternalType reports whether the server accepts the "external" data
// type variant, which Gravitino added in 1.3.1.
func (c *Client) SupportsExternalType() bool {
	return c.AtLeast(1, 3, 1)
}

// ResolveServerVersion returns the version detected by DetectServerVersion at
// provider.Configure, or probes GET /api/version (and remembers the result) when
// no version is known yet, e.g. in a unit test that builds a client directly.
//
// The returned string is the raw version only when models.ValidServerVersion
// accepts it; callers compare it with models.ServerVersionAtLeast, which fails
// closed.
func (c *Client) ResolveServerVersion(ctx context.Context) (string, error) {
	if version := c.ServerVersion(); models.ValidServerVersion(version) {
		return version, nil
	}

	resp, err := c.GetVersion(ctx)
	if err != nil {
		return "", err
	}

	version := resp.Version.Version
	if models.ValidServerVersion(version) {
		c.setServerVersion(version)
	}
	return version, nil
}

// CheckExternalTypesSupported appends one diagnostic per column whose type uses
// the external variant when the connected server predates Gravitino 1.3.1, which
// introduced it, so the request fails with an explicit version diagnostic
// instead of the server's opaque 400. An undetected server version stays
// permissive.
func (c *Client) CheckExternalTypesSupported(columns []models.ColumnTFSDK, diags *diag.Diagnostics) {
	diags.Append(models.ExternalTypeColumnDiagnostics(columns, c.ServerVersion(), c.SupportsExternalType())...)
}

// CheckFunctionExternalTypesSupported appends one diagnostic per function data
// type (parameter, return type or return column) that uses the external variant
// when the connected server predates Gravitino 1.3.1, which introduced it. An
// undetected server version stays permissive, matching
// CheckExternalTypesSupported.
func (c *Client) CheckFunctionExternalTypesSupported(definitions []models.FunctionDefinition, diags *diag.Diagnostics) {
	diags.Append(models.FunctionExternalTypeDiagnostics(definitions, c.ServerVersion(), c.SupportsExternalType())...)
}

func (c *Client) setServerVersion(version string) {
	c.versionMu.Lock()
	c.version = version
	c.versionMu.Unlock()
}
