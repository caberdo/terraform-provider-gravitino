# AGENTS.md

## Project: terraform-provider-gravitino

A Terraform provider for [Apache Gravitino](https://gravitino.apache.org/), built with the Plugin Framework (v1.19.0).

## Documentation map

Read these before changing behaviour; each states a constraint this file only summarises:

| Document | Contents |
|---|---|
| `ARCHITECTURE.md` | Package layout, request flow, auth, version gating, test layers |
| `GOVERNANCE.md` | Roles, review and merge requirements, release authority |
| `ROADMAP.md` | Supported Gravitino versions, planned surface, open work |
| `.github/OPENSSF-BEST-PRACTICES.md` | OpenSSF Best Practices criteria and the evidence for each |
| `.github/CONTRIBUTING.md` | Development setup, conventions, pull request process |
| `.github/SECURITY.md` | Supported versions, vulnerability reporting, security model |
| `CHANGELOG.md` | Release history; add an entry under `## Unreleased` |
| `NOTICE` | Upstream works and their licences |

`docs/` is **generated** by tfplugindocs from `templates/` plus `examples/`
(`make generate`, i.e. `go generate ./...`). Never hand-edit `docs/`; edit the template
or the example and regenerate, or CI's docs-drift gate (`git diff --exit-code -- docs/`)
fails. Resource and data source schema descriptions are the source of the reference docs.

## Provider surface

23 resources, 54 data sources, 12 schema attributes marked `Sensitive` (credentials,
tokens and keytab material). Registration lives in `internal/provider/provider.go`;
schemas are the source of `docs/`.

## Resources Implemented (all complete with CRUD + Import + tests)

| Resource              | TF Name                   | Hierarchy                    |
|-----------------------|---------------------------|------------------------------|
| Metalake              | `gravitino_metalake`      | `metalake`                   |
| Catalog               | `gravitino_catalog`       | `metalake.catalog`           |
| Schema                | `gravitino_schema`        | `metalake.catalog.schema`    |
| Fileset               | `gravitino_fileset`       | `metalake.catalog.schema.fs` |
| Topic                 | `gravitino_topic`         | `metalake.catalog.schema.t`  |
| Table                 | `gravitino_table`         | `metalake.catalog.schema.tb` |
| View                  | `gravitino_view`          | (implemented)                |
| Model                 | `gravitino_model`         | (implemented)                |
| Model Version         | `gravitino_model_version` | (implemented)                |
| Function              | `gravitino_function`      | (implemented)                |
| Partition             | `gravitino_partition`     | `metalake.catalog.schema.tb.pt` |
| Tag                   | `gravitino_tag`           | `metalake.tag`                 |
| Tag Association       | `gravitino_tag_association` | `metalake.{obj}.tags`        |
| Policy                | `gravitino_policy`        | `metalake.{obj}.policy`        |
| Policy Association    | `gravitino_policy_association` | `metalake.{obj}.policies`  |
| Job                   | `gravitino_job`           | `metalake.job_id` (job run)    |
| Job Template          | `gravitino_job_template`  | `metalake.job_template`        |
| User                  | `gravitino_user`          | `metalake.user`                |
| Group                 | `gravitino_group`         | `metalake.group`               |
| Role                  | `gravitino_role`          | `metalake.role`                |
| Owner                 | `gravitino_owner`         | `metalake.{obj}`               |
| IdP User              | `gravitino_idp_user`      | `metalake.idp_user`            |
| IdP Group             | `gravitino_idp_group`     | `metalake.idp_group`           |

All resources also have corresponding data sources (list + get) and documentation under `docs/`.

## Architecture

```
main.go                        # Entry point; serves protocol 6
internal/
├── client/                    # 35 files: HTTP client + per-resource REST methods
│   ├── client.go              # Core client: doRequest, Get, Post, Put, Delete
│   ├── auth/                  # 12 files: auth providers (AuthProvider interface)
│   │   ├── provider.go        # AuthProvider + TransportProvider interfaces
│   │   ├── simple.go          # Simple (OS user) auth
│   │   ├── basic.go           # HTTP Basic auth
│   │   ├── oauth_static.go    # OAuth2 static bearer token
│   │   ├── oauth_credentials.go # OAuth2 client credentials flow (auto-refresh)
│   │   └── kerberos.go        # Kerberos SPNEGO auth
│   ├── authentication.go      # Principal endpoint
│   ├── server_version.go      # DetectServerVersion, ResolveServerVersion, AtLeast
│   ├── version.go             # GetVersion + the fail-closed Check* version gates
│   ├── metalake.go, catalog.go, schema.go, fileset.go, topic.go, ...
├── models/                    # 39 files: JSON-annotated structs (requests & responses)
│   ├── common.go              # ErrorResponse, Audit, NameIdentifier
│   ├── metalake.go, catalog.go, schema.go, fileset.go, topic.go, ...
├── provider/
│   └── provider.go            # Provider schema + resource/data source registration
├── resources/                 # one package per resource + shared helpers
│   ├── metalake/resource.go   # Schema + CRUD + Import
│   ├── catalog/resource.go
│   ├── schema/resource.go
│   ├── resourceutil/          # Shared import-ID parsing helpers
│   ├── resourcetest/          # Shared test harness for resource packages
│   └── ...
├── datasources/               # 27 dirs: data source implementations
│   └── ...
├── acceptance/                # Live pre-check + protocol v6 server factories
├── tfutil/                    # Plan/state and types helpers shared by both layers
```

Tests live beside the code: 295 Go files, 131 of them `*_test.go`; 58 packages under
`./internal/...`, 56 with tests.

## API compatibility target

Resources are aligned against the Apache Gravitino **v1.3.0** OpenAPI specification
(`docs/open-api/` in the Gravitino repository, e.g.
`https://raw.githubusercontent.com/apache/gravitino/v1.3.0/docs/open-api/tables.yaml`).
Always check that spec before adding or changing a field: field names, enum values and
update request types are not guessable.

### Gravitino 1.3.0 vs 1.3.1

A single provider build supports **v1.3.0 and v1.3.1** servers. Schema validators accept the
union of both versions; the runtime then gates 1.3.1-only values on the detected server
version. Compare a spec such as
`https://github.com/apache/gravitino/compare/v1.3.0...v1.3.1` before adding a
version-restricted field.

| Capability (Gravitino API) | Provider surface | 1.3.0 | 1.3.1 |
|---|---|---|---|
| `PolicyContentBase.supportedObjectTypes` gains `VIEW`/`FUNCTION` (`policies.yaml`) | `gravitino_policy.supported_object_types`, `gravitino_policies` | rejected at create/update with an explicit diagnostic | accepted |
| `metadataObjectType` gains `VIEW`/`FUNCTION` (`openapi.yaml`) | `gravitino_statistics.resource_type`, `gravitino_credentials.resource_type`, `gravitino_tag_association.object_type`, `gravitino_policy_association.object_type`, `gravitino_object_tags.object_type`, `gravitino_object_policies.object_type` | rejected with an explicit diagnostic | accepted |
| `IndexSpec.properties` and the `data_skipping_*` `indexType` values (`indexes.yaml`) | `gravitino_table.index[].properties`, `index[].index_type` | rejected at create with an explicit diagnostic (an index change forces a replacement) | accepted |
| `ExternalType` restored to `DataType.oneOf` (`datatype.yaml`) | column `type` of `gravitino_table`/`gravitino_view` as `{"type":"external","catalogString":"..."}` | rejected at create/update with an explicit diagnostic | accepted |
| `AuthMeResponse.serviceAdmin` (`authn.yaml`) | `gravitino_principal.service_admin` | absent: always `false` | reported |
| `GET /system/iceberg-rest` (`getIcebergRestServiceUri`, `system.yaml`) | `gravitino_iceberg_rest_service` | endpoint does not exist: explicit diagnostic | `uri` set, or `null` when the service is unavailable |
| `POST /catalogs/{catalog}/testConnection` (`testExistingCatalogConnection`, `catalogs.yaml`) | existing-catalog variant of `gravitino_catalog_connection_test` | endpoint does not exist: explicit diagnostic | supported |
| `POST /catalogs/testConnection` (`testConnection`, `catalogs.yaml`) | proposed-configuration variant of `gravitino_catalog_connection_test` | supported | supported |

The `gravitino_secrets` API only exists on `main` and is documented in its schema as requiring
Gravitino 1.4 or newer.

**Version detection.** The provider detects the server version once in
`provider.Configure` (`client.DetectServerVersion`, best effort) and keeps it on the client.
`client.ResolveServerVersion` returns that version and only probes `GET /api/version` when the
probe failed, so a gate normally costs no request of its own; every comparison goes through
`models.ServerVersionAtLeast` (an unparseable version counts as "too old"). A restricted value
MUST fail with a diagnostic naming the required version, never with the server's opaque `400`.
Four shapes are in use:

- `client.CheckMetadataObjectTypeSupported`, `client.CheckPolicyObjectTypesSupported` and
  `client.CheckIndexesSupported` fail closed: they short-circuit on the 1.3.1-only values
  (`models.ObjectTypeRequiresGravitino131`, `models.IndexTypeRequiresGravitino131`), so a value
  that exists since 1.3.0 costs no version comparison, and they return the version-lookup error
  when the version cannot be read, so a restricted value is never sent to a server of unknown
  version.
- `client.CheckExternalTypesSupported` short-circuits the same way (only columns whose type
  actually uses `external` are gated) but stays permissive when the version is unknown
  (`client.SupportsExternalType` is `client.AtLeast(1, 3, 1)`, which treats an unknown version as
  current). This is deliberate: an external column is the only signal the provider has, and a
  server whose version endpoint is blocked must not lose the ability to manage such a column.
- `gravitino_iceberg_rest_service` fails open on an unknown version: it logs a failed version
  lookup with `tflog.Warn`, calls the 1.3.1-only endpoint and maps that endpoint's 404 to the
  version diagnostic. When the detected version is already known to predate 1.3.1 it reports the
  same diagnostic without calling the endpoint. Use this shape only when the endpoint response
  is itself the authoritative signal.
- `gravitino_catalog_connection_test` resolves the version with `client.ResolveServerVersion` and
  fails closed when the existing-catalog variant is used against a server older than 1.3.1; the
  proposed-configuration variant works on 1.3.0 as well.

Version-restricted API surface therefore follows a **union schema + runtime version gate**: the
schema validator accepts the union of every supported server version and a `client.Check*` method
refuses a 1.3.1-only value against an older server with an explicit "requires Gravitino >= 1.3.1"
diagnostic instead of the endpoint's opaque 400.

**Marking restricted fields.** Every attribute, data source or endpoint that needs a newer
server MUST carry the restriction in its schema description — `Requires Gravitino >= 1.3.1`
for rejected values/endpoints, `Always false on Gravitino versions before 1.3.1` for fields the
older server simply omits — and MUST fail with a diagnostic naming the required version, never
an opaque server `400`.

## Conventions

### Quick Reference

**Error handling (mandatory):**

```go
resp.Diagnostics.Append(client.NewResourceError("creating catalog", name, err)...)
```

**404 check (mandatory):**

```go
if client.IsNotFoundError(err) {
    resp.State.RemoveResource(ctx)
    return
}
```

`IsNotFoundError` inspects the real HTTP status of the response (`client.HTTPError`); it does
not look at the Gravitino error payload, whose `code` field is an application code in the
1000-1100 range and therefore never 404. Delete must treat a 404 as success (the object is
already gone).

**Client calls (mandatory):** every client method takes the request context first:

```go
result, err := r.client.GetCatalog(ctx, metalake, name)
```

**Logging (mandatory):**

```go
tflog.Debug(ctx, "Creating catalog", map[string]interface{}{"metalake": m, "name": n})
tflog.Debug(ctx, "Created catalog", map[string]interface{}{"metalake": m, "name": n})
```

**Never:**
- `resp.Diagnostics.AddError("msg", err.Error())` directly
- `strings.Contains(err.Error(), "404")` — use `client.IsNotFoundError(err)`
- `fmt.Println` / `println` / `log.Printf` in resources

### Patterns
- **ID format**: dot-separated hierarchy (`metalake`, `metalake.catalog`, etc.)
- **"id" attribute**: Always `Computed: true` with `stringplanmodifier.UseStateForUnknown()`
- **Audit**: `types.Object` with `AuditAttrTypes` (creator, create_time, last_modifier, last_modified_time)
- **Properties**: `types.Map` with `ElementType: types.StringType`
- **Update pattern**: Compare plan vs state and send the update requests the API supports. An attribute that cannot be updated in place MUST carry `RequiresReplace()`; never accept a change and silently drop it (that yields a perpetual diff).
- **Computed attributes**: must be set on *every* code path, or carry `UseStateForUnknown()`. Leaving an unknown value in state fails the apply with "Provider produced inconsistent result after apply".
- **404 handling**: `client.IsNotFoundError(err)` → `resp.State.RemoveResource(ctx)` (all resources)
- **Configure**: Casts `req.ProviderData` to `*client.Client`. For auth, the provider builds an `AuthProvider` via `buildAuthProvider()` and passes it to `client.New(uri, authProvider)`
- **Auth**: Uses `internal/client/auth/` package. The `AuthProvider` interface has `Header(ctx) (string, string, error)`. The `TransportProvider` (optional) has `WrapTransport(base) http.RoundTripper` for Kerberos SPNEGO.
- **Tests**: Use `httptest.NewServer` with custom handlers; test schema, create, update, delete, import. Mock payloads MUST be copied from the `examples:` section of the matching OpenAPI spec — hand-written payloads invented from the same Go structs only prove that the struct marshals, not that the API agrees.
- **End-to-end tests**: for CRUD correctness add a `TF_ACC=1` test using `resource.Test` with `providerserver.NewProtocol6WithError(provider.New("test")())`; that path goes through Terraform Core and catches unknown/inconsistent values that direct framework calls miss.
- **Enum validators**: Always check the Gravitino API docs for possible values and add `stringvalidator.OneOf(...)` to enum fields. Store shared enums as constants in `internal/models/privilege_names.go`.

### Checklist for New Resources

- [ ] Error handling via `client.NewResourceError`
- [ ] 404 via `client.IsNotFoundError`
- [ ] tflog.Debug at start/end of Create/Read/Update/Delete
- [ ] Import with dot-separated ID parsing (valid + invalid test)
- [ ] Unit tests: schema, create, update, delete, import (spec-faithful payloads)
- [ ] Resource registered in `internal/provider/provider.go`
- [ ] Enum fields have `stringvalidator.OneOf` with all possible values from API docs
- [ ] Docs generated via `make generate` (`go generate ./...`) from `templates/` + `examples/`; never edit `docs/` by hand

### Commands
- **Build**: `make build` (`go build ./...` for a compile check)
- **Test (unit)**: `make test` (`go test -v -cover ./internal/...`)
- **Test (acceptance)**: `make testacc` (`TF_ACC=1`), `make testacc-docker` (Gravitino 1.3.0 in Docker; `GRAVITINO_VERSION=1.3.1` for the other), `make testacc-matrix` (every version in `.github/gravitino-versions.txt`)
- **Live acceptance (real server via podman)**: `make testacc-live` / `make testacc-live-filter F=<TestLiveAcc...>`
- **Coverage**: `make coverage`, `make coverage-merge`, `make coverage-gate` (fails when the merged profile is below `.github/coverage-baseline.txt`); see [Coverage](#coverage)
- **Lint**: `golangci-lint run --config .github/golangci.yml ./...`
- **Lint fix**: `make lint-fix`
- **Vet / fmt**: `make vet`, `make fmt`
- **Docs**: `make generate` (`go generate ./...`, uses `github.com/hashicorp/terraform-plugin-docs`) — never hand-edit `docs/`
- **Examples**: `make validate-examples` (CI runs it with `REQUIRE_TERRAFORM=1`)

### Testing

**Unit tests** (`httptest.NewServer`):
```bash
go test -v -cover ./internal/...
```
Each resource package has `resource_test.go` with: schema, create, delete, import (valid + invalid).

**Acceptance tests** (real Gravitino via Docker):
```bash
docker compose run --rm test
# Or filter by pattern:
TEST_PATTERN=TestAccCatalog docker compose run --rm -e TEST_PATTERN=TestAccCatalog test
```

**Live acceptance tests (real server, not mocks):** `TestLiveAcc*` tests (see
`internal/resources/*/live_test.go`, `internal/datasources/*/live_test.go`) only run against a
real Gravitino via `make testacc-live` (podman) — the `acceptance.LivePreCheck` gate skips them
unless `GRAVITINO_URI` answers `/api/version`. They never start their own HTTP mock.

Write `Test*` for mocks, `TestAcc*` when the path must go through Terraform Core
(`providerserver.NewProtocol6WithError`), and `TestLiveAcc*` only for behaviour that a mock
cannot prove. 700 `Test*` functions today, 66 of them `TestAcc*` and 26 `TestLiveAcc*`.

#### Coverage

`make coverage` runs the unit suite alone (`TestAcc*` and `TestLiveAcc*` skip themselves, so the
profile cannot depend on the environment), writes `coverage.out` and prints a per-package table,
weakest first (`scripts/coverage-summary.sh`); `make coverage-html` opens the same profile as HTML.
Because packages such as `internal/resources/*` assert most of their behaviour end-to-end, the
acceptance container also writes `coverage-acceptance.out` (`-covermode=atomic
-coverpkg=./internal/...`), and `make coverage-merge` merges both profiles into `coverage-union.out`
and prints the same table. `merge-coverage.sh` keys blocks by position, so the union has one line
per block however the inputs were instrumented; `coverage-summary.sh` sums the statement column and
therefore rejects a profile that repeats a block (a raw `-coverpkg` profile) with exit 2 instead of
printing a wrong number.

**The floor.** `make coverage-gate` (`scripts/coverage-gate.sh`) compares `coverage-union.out`
against [`.github/coverage-baseline.txt`](.github/coverage-baseline.txt) — the same file
`scripts/coverage-readme.sh` renders README.md's badge and block from — and exits non-zero for
every scope below its `floor_*`. The `Coverage` job runs it on every pull request, after running
the acceptance container on the newest supported server (`GRAVITINO_VERSION` from
`.github/gravitino-versions.txt`), so the gate never sees a unit-only number or a 1.3.0 run, and it
also runs `coverage-readme.sh --check` so the published block cannot drift. A scope named in the
baseline that drops out of the profile, a missing, misspelled or stray `floor_*`, and an absent
profile are configuration errors (exit 2), never a pass. On `main` the same script runs again
inside the newest (`1.3.1`) acceptance-matrix cell — `latest`, read from
`.github/gravitino-versions.txt` — and the older cells are deliberately not gated because they skip
the version-gated tests. Raise a floor in the same pull request that adds the tests and run
`make coverage-readme`. The `test` job publishes the unit table in its job summary, the
`acceptance` job the merged one.

Measured on the committed tooling at revision `c2fc783` (merged profile: Gravitino 1.3.0,
`go 1.27.2`, one line per block) and reproduced on Gravitino 1.3.1, the server the `Coverage` job
runs. Always use a distinct profile per run; `-coverpkg=./...` instruments every package, a bare
`-coverprofile` reports the owning package only. The unit-only row below is that local run; CI's
`test` job installs the `go.mod` toolchain (1.27.1) and prints a different denominator in its own
summary — the gated number is always the merged one.

| Command | Coverage |
|---|---|
| `make coverage` (unit only, per package) | 64.2% (4918/7661) |
| `make coverage-merge` (unit + acceptance, the profile `make coverage-gate` enforces) | 83.4% (6387/7661) |

Scopes the gate enforces, with the floors currently committed: `internal/client` 79.7% (floor
79.0), `internal/models` 86.3 (85.5), `internal/provider` 94.6 (94.0), `internal/client/auth` 76.7
(76.0), `internal/datasources/schema` 94.6 (94.0), `internal/datasources/policy` 86.1 (85.5),
`internal/datasources/role` 85.6 (85.0), `internal/datasources/fileset` 86.5 (86.0),
`internal/datasources/partition` 92.2 (91.5), `internal/resources/partition` 92.4 (91.5), `total`
83.4 (82.5). `internal/datasources/secrets` is deliberately not a scope: it wraps the Gravitino 1.4
secrets API, which the supported 1.3.0/1.3.1 servers cannot serve.

Statement counts come straight from the cover profile, so they depend on the Go patch release that
produced it: for this tree `go 1.27.1` — the `go.mod` toolchain, which CI installs — sums
`./internal/...` to 9766 statements and `go 1.27.2` to 7661 (identical block set, different
statement counts per block). Compare numbers only within one toolchain, and note that a merged
profile takes its statement counts from the profile `merge-coverage.sh` reads last (the acceptance
container's, in CI).

### Logging

Use `tflog` from `github.com/hashicorp/terraform-plugin-log/tflog`:
- `tflog.Debug(ctx, msg, fields...)` — CRUD boundaries, before API calls
- `tflog.Warn(ctx, msg, fields...)` — non-fatal issues
- `tflog.Error(ctx, msg, fields...)` — errors returned as diagnostics

Fields use `map[string]interface{}` format. Logs respect `TF_LOG`, `TF_LOG_PATH`, `TF_LOG_PROVIDER` env vars.

### Dependencies
- Go 1.27.1
- `terraform-plugin-framework` v1.19.0
- `terraform-plugin-framework-validators` v0.19.0
- `terraform-plugin-testing` v1.16.0
- `github.com/jcmturner/gokrb5/v8` v8.4.4 (Kerberos auth)
