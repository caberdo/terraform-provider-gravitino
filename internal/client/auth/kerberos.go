package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"strings"
	"sync"

	"github.com/jcmturner/gokrb5/v8/client"
	"github.com/jcmturner/gokrb5/v8/config"
	"github.com/jcmturner/gokrb5/v8/credentials"
	"github.com/jcmturner/gokrb5/v8/keytab"
	"github.com/jcmturner/gokrb5/v8/spnego"
)

// hasNegotiateChallenge reports whether the response carries an SPNEGO
// challenge. Real servers answer with `Negotiate <base64-token>`, so the scheme
// (the first token) is compared case-insensitively, as RFC 7235 requires.
func hasNegotiateChallenge(resp *http.Response) bool {
	for _, v := range resp.Header.Values("Www-Authenticate") {
		fields := strings.Fields(v)
		if len(fields) > 0 && strings.EqualFold(fields[0], "Negotiate") {
			return true
		}
	}
	return false
}

type KerberosProvider struct {
	principal string
	krbClient *client.Client
	transport http.RoundTripper
}

func NewKerberosProvider(principal, keytabPath string, useTicketCache bool) (*KerberosProvider, error) {
	if principal == "" {
		return nil, fmt.Errorf("kerberos principal is required")
	}

	var krbClient *client.Client

	cfg, err := loadKerberosConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load krb5 config: %w", err)
	}

	if useTicketCache {
		ccachePath := os.Getenv("KRB5CCNAME")
		if ccachePath == "" {
			u, err := user.Current()
			if err != nil {
				return nil, fmt.Errorf("failed to determine current user for default ccache path: %w", err)
			}
			ccachePath = "/tmp/krb5cc_" + u.Uid
		}

		cc, err := credentials.LoadCCache(ccachePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load Kerberos CCache from %s: %w", ccachePath, err)
		}

		krbClient, err = client.NewFromCCache(cc, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create Kerberos client from CCache: %w", err)
		}
	} else {
		if keytabPath == "" {
			return nil, fmt.Errorf("keytab path is required when use_ticket_cache is false")
		}

		kt, err := keytab.Load(keytabPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load keytab from %s: %w", keytabPath, err)
		}

		realm := extractRealm(principal)
		krbClient = client.NewWithKeytab(principal, realm, kt, cfg)
	}

	return &KerberosProvider{
		principal: principal,
		krbClient: krbClient,
		transport: &spnegoRoundTripper{
			client: krbClient,
		},
	}, nil
}

func (p *KerberosProvider) Header(ctx context.Context) (string, string, error) {
	return "", "", nil
}

func (p *KerberosProvider) WrapTransport(base http.RoundTripper) http.RoundTripper {
	if rt, ok := p.transport.(*spnegoRoundTripper); ok {
		rt.setBase(base)
	}
	return p.transport
}

func (p *KerberosProvider) Close() error {
	if p.krbClient == nil {
		return nil
	}
	p.krbClient.Destroy()
	return nil
}

type spnegoRoundTripper struct {
	mu     sync.Mutex
	base   http.RoundTripper
	client *client.Client
}

// setBase records the underlying transport once. It is safe to call repeatedly
// and from concurrent goroutines; the first base wins, so repeated wrapping does
// not silently swap the transport under an in-flight request.
func (rt *spnegoRoundTripper) setBase(base http.RoundTripper) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.base == nil {
		rt.base = base
	}
}

func (rt *spnegoRoundTripper) baseTransport() http.RoundTripper {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.base != nil {
		return rt.base
	}
	return http.DefaultTransport
}

func (rt *spnegoRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := spnego.SetSPNEGOHeader(rt.client, req, ""); err != nil {
		return nil, fmt.Errorf("kerberos authentication failed: %w", err)
	}

	base := rt.baseTransport()

	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if resp.StatusCode == http.StatusUnauthorized && hasNegotiateChallenge(resp) {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		retryReq, err := cloneRequestForRetry(req)
		if err != nil {
			return nil, fmt.Errorf("kerberos authentication retry failed: %w", err)
		}
		if err := spnego.SetSPNEGOHeader(rt.client, retryReq, ""); err != nil {
			return nil, fmt.Errorf("kerberos authentication retry failed: %w", err)
		}

		return base.RoundTrip(retryReq)
	}

	return resp, nil
}

// cloneRequestForRetry copies req and rewinds its body. req.Clone alone shares
// the (already consumed) body, which would make a retried POST/PUT send an empty
// body while still advertising the original Content-Length.
func cloneRequestForRetry(req *http.Request) (*http.Request, error) {
	retryReq := req.Clone(req.Context())
	if req.Body == nil {
		return retryReq, nil
	}
	if req.GetBody == nil {
		return nil, errors.New("request body is not rewindable")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	retryReq.Body = body
	return retryReq, nil
}

func extractRealm(principal string) string {
	parts := strings.SplitN(principal, "@", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func loadKerberosConfig() (*config.Config, error) {
	if cfgPath := os.Getenv("KRB5_CONFIG"); cfgPath != "" {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load KRB5_CONFIG %s: %w", cfgPath, err)
		}
		return cfg, nil
	}

	cfg, err := config.Load("/etc/krb5.conf")
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to load /etc/krb5.conf: %w", err)
	}

	return config.New(), nil
}
