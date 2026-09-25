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
	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

func LivePreCheck(t *testing.T) func() {
	return func() {
		uri := strings.TrimRight(os.Getenv("GRAVITINO_URI"), "/")
		if uri == "" {
			t.Skip("GRAVITINO_URI is not set; skipping live acceptance tests")
		}

		c, err := client.New(uri, nil)
		if err != nil {
			t.Fatalf("invalid GRAVITINO_URI %q: %v", uri, err)
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
