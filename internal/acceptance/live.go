package acceptance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// liveClient builds the provider client for the server under test and validates
// that it is a real Gravitino reporting the expected version.
func liveClient(t *testing.T) (*client.Client, string) {
	t.Helper()

	uri := strings.TrimRight(os.Getenv("GRAVITINO_URI"), "/")
	if uri == "" {
		t.Skip("GRAVITINO_URI is not set; skipping live acceptance tests")
	}

	c, err := provider.NewClientFromEnv(uri)
	if err != nil {
		t.Fatalf("invalid GRAVITINO_URI %q or authentication configuration: %v", uri, err)
	}

	ver, err := c.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("server at %s is not reachable or not Gravitino: %v", uri, err)
	}
	if ver.Version.Version == "" {
		t.Fatalf("server at %s returned an empty version; refusing to run against a mock", uri)
	}
	if want := os.Getenv("GRAVITINO_EXPECT_VERSION"); want != "" && ver.Version.Version != want {
		t.Fatalf("server at %s reports version %q, want %q", uri, ver.Version.Version, want)
	}

	return c, ver.Version.Version
}

func LivePreCheck(t *testing.T) func() {
	return func() {
		_, _ = liveClient(t)
	}
}

// SkipBefore skips a live test when the server predates major.minor.patch, the
// convention the suite uses for a feature a newer release introduced.
func SkipBefore(t *testing.T, major, minor, patch int, feature string) {
	t.Helper()

	_, version := liveClient(t)
	if !models.ServerVersionAtLeast(version, major, minor, patch) {
		t.Skipf("%s requires Gravitino >= %d.%d.%d, server reports %s", feature, major, minor, patch, version)
	}
}

// ServerVersionAtLeast reports whether the live server is at least
// major.minor.patch, without skipping the test. Use it to choose between
// version-specific steps.
func ServerVersionAtLeast(t *testing.T, major, minor, patch int) bool {
	t.Helper()

	_, version := liveClient(t)
	return models.ServerVersionAtLeast(version, major, minor, patch)
}

// ExpectedPrincipal returns the principal the server attributes a request to,
// for tests that compare an audit creator. Gravitino 1.3.0 added GET
// /api/authn/me; on an older server only the no-authentication default can be
// known, so a test that needs an authenticated principal skips.
func ExpectedPrincipal(t *testing.T) string {
	t.Helper()

	c, version := liveClient(t)
	if models.ServerVersionAtLeast(version, 1, 3, 0) {
		principal, err := c.GetAuthenticatedPrincipal(context.Background())
		if err != nil {
			t.Fatalf("reading the authenticated principal: %v", err)
		}
		return principal.Principal
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv("GRAVITINO_AUTH"))) {
	case "", "none":
		return "anonymous"
	default:
		t.Skip("the audit principal requires Gravitino >= 1.3.0 when authentication is enabled")
		return ""
	}
}

func ProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func UniqueName(prefix string) string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return fmt.Sprintf("acc_%s_%s", prefix, hex.EncodeToString(b))
}
