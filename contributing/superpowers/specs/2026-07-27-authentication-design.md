# Authentication Provider Design

**Date:** 2026-07-27
**Status:** Design approved, awaiting implementation

## Overview

Implement all 4 Gravitino authentication methods in the Terraform provider: Simple, Basic, OAuth (static + client credentials), and Kerberos. Currently only Basic and static OAuth bearer token are supported.

## Architecture

### Modular Auth Provider Pattern

New package `internal/client/auth/` with a provider interface and 5 implementations:

```go
package auth

type AuthProvider interface {
    Header(ctx context.Context) (string, string, error)
}
```

`Header()` returns `(headerName, headerValue, error)`. For most providers this is `("Authorization", "<scheme> <token>", nil)`.

#### Transport Provider Extension

For auth types that need HTTP transport-level handling (Kerberos SPNEGO), an optional interface:

```go
type TransportProvider interface {
    WrapTransport(base http.RoundTripper) http.RoundTripper
}
```

### Provider Schema

`GravitinoProviderModel` expands from 5 to 12 attributes:

| Attribute | Type | Env Variable | Auth Method | Description |
|-----------|------|-------------|-------------|-------------|
| `uri` | string | `GRAVITINO_URI` | all | Server URI |
| `auth` | string | `GRAVITINO_AUTH` | all | `"simple"`, `"basic"`, `"oauth"`, `"kerberos"` |
| `username` | string | `GRAVITINO_USERNAME` | simple, basic | Username |
| `password` | string (sensitive) | `GRAVITINO_PASSWORD` | basic | Password |
| `oauth_token` | string (sensitive) | `GRAVITINO_OAUTH_TOKEN` | oauth (static) | Static bearer token |
| `oauth_client_id` | string | `GRAVITINO_OAUTH_CLIENT_ID` | oauth (credentials) | OAuth2 client ID |
| `oauth_client_secret` | string (sensitive) | `GRAVITINO_OAUTH_CLIENT_SECRET` | oauth (credentials) | OAuth2 client secret |
| `oauth_server_uri` | string | `GRAVITINO_OAUTH_SERVER_URI` | oauth (credentials) | OAuth2 server URI |
| `oauth_token_path` | string | `GRAVITINO_OAUTH_TOKEN_PATH` | oauth (credentials) | Token endpoint path |
| `oauth_scope` | string | `GRAVITINO_OAUTH_SCOPE` | oauth (credentials) | Token scope |
| `kerberos_principal` | string | `GRAVITINO_KERBEROS_PRINCIPAL` | kerberos | Kerberos principal |
| `kerberos_keytab` | string (sensitive) | `GRAVITINO_KERBEROS_KEYTAB` | kerberos | Path to keytab file |
| `kerberos_use_ticket_cache` | bool | `GRAVITINO_KERBEROS_USE_TICKET_CACHE` | kerberos | Use OS ticket cache |

`auth` validator changes from `OneOf("basic", "oauth")` to `OneOf("simple", "basic", "oauth", "kerberos")`.

## Implementation

### SimpleProvider (`internal/client/auth/simple.go`)

- Username from config, then `GRAVITINO_USER` env var, then `os/user.Current().Username`
- Sends `Authorization: Basic base64(username + ":")` (empty password)
- Gravitino server maps this to the anonymous user if no password

### BasicProvider (`internal/client/auth/basic.go`)

- Mirrors current `setAuth()` logic with `req.SetBasicAuth()`
- Pure extraction from current client, no functional change

### OAuthStaticProvider (`internal/client/auth/oauth_static.go`)

- Mirrors current bearer token logic
- Pure extraction from current client, no functional change

### OAuthCredentialsProvider (`internal/client/auth/oauth_credentials.go`)

- POST to `<server_uri>/<token_path>` with `grant_type=client_credentials`
- Parses `access_token` and `expires_in` from response
- Caches token in-memory with `sync.RWMutex`
- Refreshes at `expires_in * 0.9` seconds (10% safety margin)
- No `refresh_token` needed — client_credentials grant gets new tokens directly

### KerberosProvider (`internal/client/auth/kerberos.go`)

- Uses `github.com/jcmturner/gokrb5/v8` library
- Two modes:
  - **Keytab**: `krb5.NewClientWithKeytab(principal, keytabPath)`
  - **Ticket cache**: `krb5.NewClientWithCCache(principal)`
- Uses `spnego.NewTransport(krb5Client)` which handles the full SPNEGO negotiation (initial request → 401 challenge → service ticket → retry)
- `WrapTransport()` returns the SPNEGO transport wrapping the base transport
- `Header()` returns empty — the transport handles auth automatically

### Client Refactoring (`internal/client/client.go`)

`Client` struct simplifies from 5 auth fields to 1 `auth.AuthProvider`:

```go
type Client struct {
    baseURL      string
    httpClient   *http.Client
    authProvider auth.AuthProvider
}
```

Constructor:
```go
func New(uri string, authProvider auth.AuthProvider) (*Client, error)
```

In the constructor, if `authProvider` implements `TransportProvider`, wrap the `http.Client.Transport`.

`setAuth()` is removed; `doRequest()` calls `authProvider.Header(ctx)` instead.

### Provider Configure (`internal/provider/provider.go`)

New `buildAuthProvider()` function with a `switch` on `auth` value:

```go
func buildAuthProvider(auth string, ...) (auth.AuthProvider, error) {
    switch auth {
    case "simple":
        return auth.NewSimpleProvider(username), nil
    case "basic":
        return auth.NewBasicProvider(username, password), nil
    case "oauth":
        if oauthToken != "" {
            return auth.NewOAuthStaticProvider(oauthToken), nil
        }
        return auth.NewOAuthCredentialsProvider(clientID, secret, uri, path, scope)
    case "kerberos":
        return auth.NewKerberosProvider(principal, keytab, useCache)
    default:
        return nil, fmt.Errorf("unknown auth method: %s", auth)
    }
}
```

## Dependencies

- `github.com/jcmturner/gokrb5/v8` — Kerberos/SPNEGO support
- `golang.org/x/oauth2` — optional, if we want to use its token types; alternatively hand-roll the token exchange

No new dependencies needed for Simple, Basic, or static OAuth — all use standard `net/http`.

## Files Changed

| File | Change |
|------|--------|
| `internal/client/auth/provider.go` | New — AuthProvider + TransportProvider interfaces |
| `internal/client/auth/simple.go` | New — SimpleProvider |
| `internal/client/auth/basic.go` | New — BasicProvider (moved from client.go) |
| `internal/client/auth/oauth_static.go` | New — OAuthStaticProvider (moved from client.go) |
| `internal/client/auth/oauth_credentials.go` | New — OAuthCredentialsProvider |
| `internal/client/auth/kerberos.go` | New — KerberosProvider |
| `internal/client/client.go` | Refactor — remove auth fields, use AuthProvider |
| `internal/client/authentication.go` | Refactor — use AuthProvider from client |
| `internal/provider/provider.go` | Expand schema, buildAuthProvider logic |
| `go.mod` / `go.sum` | Add gokrb5 dependency |
| `README.md` | Update auth configuration docs |
| `docs/index.md` | Regenerate provider docs |
| `examples/provider/provider.tf` | Add examples for all auth modes |
| `internal/client/auth/provider_test.go` | New — unit tests for each provider |

## Testing

- Unit tests per auth provider (header format, token refresh, error cases)
- Integration test with gokrb5 mock KDC (if feasible) or documented manual test procedure
- Acceptance tests via Docker Compose (existing test framework, add auth config variants)

## Future Considerations

- TLS client certificate authentication
- Custom auth providers (pluggable)
- OAuth2 password grant (deprecated but Gravitino supports it)
- Token revocation on provider shutdown
