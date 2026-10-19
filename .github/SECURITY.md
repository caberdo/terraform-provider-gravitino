# Security Policy

This document is both the security policy for the Terraform provider for Apache
Gravitino and the assurance case behind it. Mechanism claims name the file or
code path that implements them so a reviewer can check each one against the
source.

Module: `github.com/gravitino/terraform-provider-gravitino`
Registry: `registry.terraform.io/caberdo/gravitino`
License: Apache-2.0. Repository: <https://github.com/caberdo/terraform-provider-gravitino>

## Supported versions

The provider currently ships as the 0.x line. The latest release is `v0.9.0`
(published 2026-10-08); there are 24 tags in the repository. Security fixes land
on `main` and in the next release from that line.

| Version | Supported |
|---------|-----------|
| 0.x (latest `v0.9.0`) | Yes |
| Anything older than the most recent release | No |

Because the 0.x line has no long-term-support branches, only the most recent
release is supported. Users on an older tag should upgrade before reporting a
defect that already has a fix on `main`.

## Reporting a vulnerability

Report privately through GitHub private vulnerability reporting:

<https://github.com/caberdo/terraform-provider-gravitino/security/advisories/new>

Do not open a public issue for a suspected vulnerability. GitHub Discussions are
disabled on this repository; the private advisory form above is the reporting
channel.

Include, as far as you can determine:

- The provider version (and the release tag) you tested.
- The Terraform CLI version and the operating system.
- The Gravitino server version (`1.3.0` or `1.3.1` are the supported servers).
- A description of the impact: credential exposure, privilege escalation,
  integrity loss, denial of service.
- Reproduction steps, ideally a minimal Terraform configuration and the exact
  commands run.
- A proof of concept if available.
- Whether the issue is already public anywhere, and under what embargo.

Response timelines the maintainer aims to meet:

| Stage | Target |
|-------|--------|
| Acknowledgement of the report | 48 hours |
| Assessment and severity triage | 7 days |
| Fix release | 14 days |

On receipt the maintainer acknowledges the report, reproduces it, agrees a
severity and an embargo with the reporter, prepares a fix on a private branch,
and publishes a GitHub security advisory with the fix. Where a reporter cannot
reproduce on the latest release, the maintainer will say so in the advisory.

Credit policy: reporters are credited by name or handle in the published
advisory unless they ask to remain anonymous. Reporters may request the
severity wording be reviewed before publication.

## Requirements

What the shipped provider must guarantee, and where each guarantee lives:

- Four authentication methods: HTTP Basic (`internal/client/auth/basic.go` and
  `internal/client/auth/simple.go`), a static OAuth2 bearer token
  (`internal/client/auth/oauth_static.go`), the OAuth2 client-credentials flow
  (`internal/client/auth/oauth_credentials.go`), and Kerberos/SPNEGO from a
  keytab or an existing ticket cache (`internal/client/auth/kerberos.go`).
- Credentials are only ever sent as the `Authorization` header. Each auth
  provider returns exactly that header key (`internal/client/auth/`), and the
  HTTP client attaches it in `internal/client/client.go`.
- Credentials are never logged, and the provider configuration value `uri` is
  never logged either. Provider code uses `tflog.Debug` at CRUD boundaries;
  these calls do not carry credential values.
- The 12 `Sensitive` schema attributes that carry credentials or secrets are
  marked `Sensitive: true` (for example `internal/provider/provider.go` for the
  provider configuration, and `internal/datasources/credential/datasource.go`
  and `internal/datasources/secrets/datasource.go` for the data sources that
  return server-returned secrets).
- TLS certificate verification is on by default and cannot be turned off. There
  is no `InsecureSkipVerify` and no provider option to disable verification;
  the client relies on Go's standard-library defaults (TLS 1.2 minimum,
  certificate verification on). See `internal/client/client.go`.
- State contents are the user's responsibility. The provider writes what the
  server and the configuration supply into Terraform state; it does not encrypt
  state or choose a backend.

## Threat model

Trust boundaries and what crosses each one:

| Boundary | Data crossing | Controls |
|----------|---------------|----------|
| Terraform CLI ↔ plugin process | Configuration values, credentials, resource state | Terraform plugin protocol 6.0; `Sensitive` marking; credentials carried only in the `Authorization` header |
| Provider → Gravitino REST API over the configured `uri` | Resources, data-source reads, catalog and metadata payloads | Scheme restricted to http/https (`internal/client/client.go`); one 30 s-timeout `http.Client`; JSON request bodies with `Accept: application/vnd.gravitino.v1+json` |
| Provider → OAuth2 token endpoint | Client id and secret, bearer token in the response | Separate 30 s HTTP client; token cached with a refresh margin; refreshes serialised (`internal/client/auth/oauth_credentials.go`) |
| Provider → Kerberos KDC | Principal, keytab material or cached ticket | `gokrb5/v8` SPNEGO; keytab path read from disk; ticket cache optional (`internal/client/auth/kerberos.go`) |
| Provider → local filesystem | The keytab file at the configured path | Read-only access to the keytab path; no other filesystem access, no `os/exec` anywhere in the provider |
| Release supply chain: Actions → GoReleaser → signed checksums + provenance → Terraform Registry | Release artifacts | `create-release-tag.yml` imports the release GPG key from secrets `GPG_PRIVATE_KEY`/`PASSPHRASE` and pushes a **signed** tag; `release.yml` builds with GoReleaser, detach-signs `..._SHA256SUMS`, attests SLSA v1.0 build provenance over the checksum file with an OIDC identity (`actions/attest-build-provenance`), and attaches the SPDX-JSON SBOM and the provenance bundle. The public key is committed at `.github/release-signing-key.asc` and the verification steps are in `README.md#verifying-a-release` |

## Assurance case

Secure-design principles mapped to the code that implements them:

- Complete mediation. Every Gravitino REST call goes through the single
  `doRequest` / `do` pair in `internal/client/client.go`, which builds the
  request, applies the auth provider's `Authorization` header, executes it, and
  decodes the response. No resource or data source constructs its own HTTP
  request, so authentication, URL handling and error decoding cannot be
  bypassed by a caller.
- Fail-safe defaults. `client.New` rejects any URI whose scheme is not http or
  https and any empty host (`internal/client/client.go`). When the server
  version cannot be detected, the fail-closed gates
  (`CheckMetadataObjectTypeSupported`, `CheckPolicyObjectTypesSupported` and
  `CheckIndexesSupported` in `internal/client/version.go`) short-circuit on a
  value that exists only from 1.3.1 on and then return an error, so a
  version-restricted value is never sent to a server of unknown version. The
  deliberately permissive gates (`AtLeast` and `SupportsExternalType`, used by
  `CheckExternalTypesSupported` and `CheckFunctionExternalTypesSupported` in
  `internal/client/server_version.go`) treat an unknown version as recent enough
  so a transient probe failure cannot reject a configuration the server would
  accept; an external data type is the only signal the provider has for such a
  column. The provider has no switch to disable TLS verification.
- Economy of mechanism. One `http.Client` with a 30 s timeout per provider
  instance; no `os/exec` anywhere; no template, SQL or shell evaluation; JSON
  bodies decoded into typed models in `internal/models`.
- Open design. The provider source is public and Apache-2.0 licensed; the threat
  model and limitations below are published rather than implied.

## Common weakness classes

| Class | Position | Where |
|-------|----------|-------|
| Injection (shell, SQL, template) | Not applicable: no shell, SQL or template evaluation anywhere; inputs are decoded into typed `internal/models` structs and sent as JSON | whole repository |
| Sensitive data exposure | 12 `Sensitive` schema attributes cover credential- and secret-bearing values; error bodies are truncated to 2 KiB before being surfaced (`internal/client/client.go`, `maxErrorBodyBytes`, `truncateBody`). The OAuth token-endpoint error path echoes the token server's response body (`internal/client/auth/oauth_credentials.go`) — an accepted limitation below | `internal/provider`, `internal/client` |
| Man-in-the-middle | Standard-library TLS verification with Go's defaults; no option to disable it. Go's `net/http` strips the `Authorization` header on cross-host redirects | `internal/client/client.go` |
| Insecure deserialization | Responses are decoded into typed models of the Gravitino envelope; no arbitrary type resolution | `internal/models`, `internal/client` |
| XXE / XML parsing | Not applicable: the provider has no XML parser | — |
| Unbounded input | Requests time out after 30 s (`defaultTimeout` in `internal/client/client.go`); error bodies are truncated to 2 KiB | `internal/client/client.go` |
| Dependency risk | Dependabot version updates cover gomod, docker-compose and github-actions (daily, 03:00 Europe/Amsterdam, labels `dependencies` and `deps`; the compose stack may only bump `golang`, so the server image stays on the version matrix). OpenSSF Scorecard runs weekly, on `main` and on branch-protection changes, and uploads its SARIF to code scanning. Dependabot security-update alerts, secret scanning and `govulncheck` are still disabled | `.github/dependabot.yml`, `.github/workflows/scorecard.yml` |
| Supply chain | Release tags are GPG-signed, release checksums are GPG-detach-signed with the release key (public half committed), SLSA v1.0 build provenance is attested over the checksum file and attached to the release, and an SPDX-JSON SBOM is attached to the GitHub release | `.github/workflows/release.yml`, `.github/release-signing-key.asc` |

## Hardening in place

- Branch ruleset `main` on the default branch forbids deletions and
  force-pushes and requires a pull request, and it sets
  `require_code_owner_review: true` with `required_approving_review_count: 0`.
  The `CODEOWNERS` entry is `* @caberdo`, which is the *organization*; GitHub
  resolves code owners to users or teams, so the code-owner rule has no
  effective owner today and there is no required status-check rule either. Both
  gaps are recorded in `GOVERNANCE.md` rather than implied as enforced.
- CI runs `bodyclose`, `noctx`, `unconvert` and the rest of the golangci-lint
  set (`.github/golangci.yml`), plus `gosec` whose findings are uploaded to
  code scanning and fail the job.
- `dependency-review` runs on pull requests.
- `docs.yml` runs the markdown link check (`tcort/markdown-link-check`) on pull
  requests.
- Release artifacts are built from a clean `CGO_ENABLED=0`, `-trimpath`, `-s -w`
  GoReleaser configuration with the commit timestamp as `mod_timestamp`; the tag
  is GPG-signed, the checksums are detach-signed, SLSA v1.0 build provenance is
  attested over the checksum file, and the SPDX-JSON SBOM and the provenance
  bundle are attached to the GitHub release.

## Accepted limitations

Stated plainly, because each is a real property of the current release:

- The `gravitino_secrets` and `gravitino_credentials` data sources place
  server-returned secrets in Terraform state. State encryption and remote
  backends are the user's responsibility; the provider marks the attributes
  `Sensitive` but cannot make state secret.
- A plaintext `http://` URI is accepted if the user configures one. TLS is not
  forced; the provider only rejects schemes other than http and https.
- Provider configuration taken from `GRAVITINO_*` environment variables is only
  as private as the environment it is set in.
- The provider does not manage the Gravitino server's own security. Server-side
  authentication, authorisation and audit logging are outside this provider's
  scope.
- The OAuth token-endpoint error path includes the token server's response body
  in the returned error. That path returns a plain error rather than the
  provider's `HTTPError`, so the 2 KiB truncation that applies to Gravitino
  error bodies does not apply here; a token server that reflects credentials in
  its error body could leak them into an error message.
- No dated security review has been performed yet.

## Security review status

No formal, dated security review of this provider has been performed. The
controls in this document are those that are implemented in code and CI today.
OpenSSF Scorecard results are published and CI reports the merged unit +
acceptance coverage; the dependency-scanning gaps listed below remain open.

## Dependency scanning

- Dependabot version updates: enabled for gomod, docker-compose and
  github-actions, daily at 03:00 Europe/Amsterdam, labels `dependencies` and
  `deps`. The compose stack may only bump `golang`, so the Gravitino server
  image stays on the version `.github/gravitino-versions.txt` drives.
- Dependabot security updates: disabled.
- Secret scanning: disabled.
- `govulncheck`: not configured.
- OpenSSF Scorecard: enabled, weekly (Mondays 04:23 UTC), on pushes to `main`
  and whenever branch protection changes; the SARIF result is uploaded to code
  scanning and published for the README badge
  (`.github/workflows/scorecard.yml`).

The dependency surface is direct dependencies on
terraform-plugin-framework v1.19.0, terraform-plugin-framework-validators
v0.19.0, terraform-plugin-go v0.31.0, terraform-plugin-log v0.11.0,
terraform-plugin-testing v1.16.0, terraform-plugin-docs v0.25.0, and gokrb5/v8
v8.4.4; `go list -m all` reports 155 modules. The HashiCorp plugin libraries are
MPL-2.0 and `gokrb5/v8` is Apache-2.0 and is linked into the provider binary.
See `NOTICE` for upstream lineage.

## Security best practices for users

1. Use an `https://` Gravitino `uri`. The provider accepts `http://`, but only
   TLS protects credentials in transit.
2. Do not hardcode credentials in Terraform files. Prefer the provider's
   `GRAVITINO_*` environment variables, and treat those variables as secret.
3. Encrypt Terraform state or use a remote backend with access controls; the
   `gravitino_secrets` and `gravitino_credentials` data sources and any
   credential-bearing attributes end up in state.
4. Grant the Gravitino principal the least privilege it needs; the provider acts
   with exactly the credentials you configure.
5. Rotate Gravitino credentials, OAuth client secrets and Kerberos keytabs
   regularly, and store keytabs so that only the Terraform runner can read them.
6. Upgrade to the latest release before filing a report, so a known fix is not
   re-reported.
