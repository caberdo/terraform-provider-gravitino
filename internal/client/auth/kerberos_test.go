package auth

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jcmturner/gokrb5/v8/iana/etypeID"
	"github.com/jcmturner/gokrb5/v8/keytab"
)

func TestHasNegotiateChallenge(t *testing.T) {
	tests := map[string]struct {
		header string
		want   bool
	}{
		"real spnego challenge with token": {header: "Negotiate YIIFhgYGKwYBBQUCoIIF", want: true},
		"bare negotiate":                   {header: "Negotiate", want: true},
		"trailing space":                   {header: "Negotiate ", want: true},
		"leading space":                    {header: " Negotiate abc", want: true},
		"basic auth":                       {header: `Basic realm="x"`, want: false},
		"empty":                            {header: "", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tc.header != "" {
				resp.Header.Set("WWW-Authenticate", tc.header)
			}
			if got := hasNegotiateChallenge(resp); got != tc.want {
				t.Errorf("hasNegotiateChallenge(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}

func TestCloneRequestForRetry_RewindsBody(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com/api/metalakes", strings.NewReader(`{"name":"ml"}`))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	// Simulate the first attempt consuming the body.
	if _, err := io.ReadAll(req.Body); err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	retry, err := cloneRequestForRetry(req)
	if err != nil {
		t.Fatalf("cloneRequestForRetry() error = %v", err)
	}

	got, err := io.ReadAll(retry.Body)
	if err != nil {
		t.Fatalf("ReadAll(retry.Body) error = %v", err)
	}
	if want := `{"name":"ml"}`; string(got) != want {
		t.Errorf("retried body = %q, want %q", got, want)
	}
}

func TestCloneRequestForRetry_NonRewindableBody(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com/api/metalakes", io.NopCloser(bytes.NewBufferString("x")))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.GetBody = nil

	if _, err := cloneRequestForRetry(req); err == nil {
		t.Error("cloneRequestForRetry() error = nil for a non-rewindable body, want an error")
	}
}

const testKrb5Conf = `[libdefaults]
 default_realm = EXAMPLE.COM
 dns_lookup_kdc = false
 dns_lookup_realm = false

[realms]
 EXAMPLE.COM = {
  kdc = 127.0.0.1:1
 }
`

// writeKrb5Conf writes a minimal krb5.conf and points KRB5_CONFIG at it.
func writeKrb5Conf(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "krb5.conf")
	if err := os.WriteFile(path, []byte(testKrb5Conf), 0o600); err != nil {
		t.Fatalf("writing krb5.conf: %v", err)
	}
	t.Setenv("KRB5_CONFIG", path)
}

// writeKeytab writes a keytab holding one principal. The provider never contacts
// a KDC while it is constructed, so this is enough to exercise the keytab path.
func writeKeytab(t *testing.T, principal, realm string) string {
	t.Helper()

	kt := keytab.New()
	if err := kt.AddEntry(principal, realm, "password", time.Now(), 1, etypeID.AES256_CTS_HMAC_SHA1_96); err != nil {
		t.Fatalf("adding a keytab entry: %v", err)
	}

	path := filepath.Join(t.TempDir(), "test.keytab")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating the keytab: %v", err)
	}
	defer f.Close()
	if _, err := kt.Write(f); err != nil {
		t.Fatalf("writing the keytab: %v", err)
	}
	return path
}

func TestLoadKerberosConfig(t *testing.T) {
	writeKrb5Conf(t)
	cfg, err := loadKerberosConfig()
	if err != nil {
		t.Fatalf("loadKerberosConfig() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("loadKerberosConfig() returned a nil config")
	}

	t.Setenv("KRB5_CONFIG", filepath.Join(t.TempDir(), "absent.conf"))
	if _, err := loadKerberosConfig(); err == nil {
		t.Fatal("loadKerberosConfig() must report a missing KRB5_CONFIG file")
	}
}

func TestExtractRealm(t *testing.T) {
	cases := map[string]string{
		"user@REALM":    "REALM",
		"user@a@b":      "a@b",
		"user":          "",
		"http/server@X": "X",
		"":              "",
	}
	for principal, want := range cases {
		if got := extractRealm(principal); got != want {
			t.Errorf("extractRealm(%q) = %q, want %q", principal, got, want)
		}
	}
}

func TestNewKerberosProvider_RequiresPrincipal(t *testing.T) {
	if _, err := NewKerberosProvider("", "", false); err == nil {
		t.Fatal("a missing principal must be rejected")
	}
}

func TestNewKerberosProvider_RequiresKeytab(t *testing.T) {
	writeKrb5Conf(t)
	if _, err := NewKerberosProvider("user@EXAMPLE.COM", "", false); err == nil {
		t.Fatal("a missing keytab path must be rejected")
	}
}

func TestNewKerberosProvider_KeytabMissing(t *testing.T) {
	writeKrb5Conf(t)
	_, err := NewKerberosProvider("user@EXAMPLE.COM", filepath.Join(t.TempDir(), "absent.keytab"), false)
	if err == nil {
		t.Fatal("a missing keytab file must be rejected")
	}
}

func TestNewKerberosProvider_TicketCacheMissing(t *testing.T) {
	writeKrb5Conf(t)
	t.Setenv("KRB5CCNAME", filepath.Join(t.TempDir(), "absent.ccache"))
	if _, err := NewKerberosProvider("user@EXAMPLE.COM", "", true); err == nil {
		t.Fatal("a missing ccache must be rejected")
	}
}

// TestNewKerberosProvider_DefaultTicketCache covers the default ccache path: when
// KRB5CCNAME is unset the path is derived from the current user, and a missing
// cache is reported with that path.
func TestNewKerberosProvider_DefaultTicketCache(t *testing.T) {
	writeKrb5Conf(t)
	t.Setenv("KRB5CCNAME", "")
	_, err := NewKerberosProvider("user@EXAMPLE.COM", "", true)
	if err == nil {
		t.Fatal("a missing default ccache must be rejected")
	}
	if !strings.Contains(err.Error(), "failed to load Kerberos CCache") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKerberosProvider_KeytabLifecycle(t *testing.T) {
	writeKrb5Conf(t)
	keytabPath := writeKeytab(t, "user", "EXAMPLE.COM")

	p, err := NewKerberosProvider("user@EXAMPLE.COM", keytabPath, false)
	if err != nil {
		t.Fatalf("NewKerberosProvider() error = %v", err)
	}

	name, value, err := p.Header(context.Background())
	if err != nil || name != "" || value != "" {
		t.Fatalf("Header() = (%q, %q, %v), want empty strings and no error", name, value, err)
	}

	base := &recordingTransport{}
	if got := p.WrapTransport(base); got == nil {
		t.Fatal("WrapTransport() returned nil")
	}
	// The first base wins, so a second wrap does not swap the transport.
	if got := p.WrapTransport(http.DefaultTransport); got == nil {
		t.Fatal("WrapTransport() returned nil on the second call")
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestSpnegoRoundTripper_BaseTransport(t *testing.T) {
	rt := &spnegoRoundTripper{}
	if got := rt.baseTransport(); got != http.DefaultTransport {
		t.Fatalf("baseTransport() = %v, want http.DefaultTransport without a base", got)
	}

	first := &recordingTransport{}
	second := &recordingTransport{}
	rt.setBase(first)
	rt.setBase(second)
	if got := rt.baseTransport(); got != http.RoundTripper(first) {
		t.Fatalf("baseTransport() = %v, want the first base", got)
	}
}

// TestSpnegoRoundTripper_RoundTripWithoutCredentials covers the failure path of
// RoundTrip: acquiring a service ticket without a reachable KDC fails before the
// request is sent.
func TestSpnegoRoundTripper_RoundTripWithoutCredentials(t *testing.T) {
	writeKrb5Conf(t)
	keytabPath := writeKeytab(t, "user", "EXAMPLE.COM")

	p, err := NewKerberosProvider("user@EXAMPLE.COM", keytabPath, false)
	if err != nil {
		t.Fatalf("NewKerberosProvider() error = %v", err)
	}
	defer func() { _ = p.Close() }()

	base := &recordingTransport{}
	rt := p.WrapTransport(base)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/api/version", nil)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("RoundTrip() must fail when no service ticket can be acquired")
	}
	if !strings.Contains(err.Error(), "kerberos authentication failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if base.calls != 0 {
		t.Errorf("no request may reach the base transport, got %d", base.calls)
	}
}

// recordingTransport counts the requests that reach it.
type recordingTransport struct {
	calls int
}

func (rt *recordingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	rt.calls++
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}}, nil
}
