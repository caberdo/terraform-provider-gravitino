# AGENTS.md

## Project: terraform-provider-gravitino

A Terraform provider for [Apache Gravitino](https://gravitino.apache.org/), built with the Plugin Framework (v1.19.0).

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
| Policy                | `gravitino_policy`        | `metalake.{obj}.policy`        |
| Job                   | `gravitino_job`           | `metalake.job_id` (job run)    |
| Job Template          | `gravitino_job_template`  | `metalake.job_template`        |
| User                  | `gravitino_user`          | `metalake.user`                |
| Group                 | `gravitino_group`         | `metalake.group`               |
| Role                  | `gravitino_role`          | `metalake.role`                |
| Owner                 | `gravitino_owner`         | `metalake.{obj}`               |

All resources also have corresponding data sources (list + get) and documentation under `docs/`.

## Architecture

```
main.go                        # Entry point
internal/
├── client/                    # HTTP client + per-resource REST methods
│   ├── client.go              # Core client: doRequest, Get, Post, Put, Delete
│   ├── auth/                  # Auth providers (AuthProvider interface)
│   │   ├── provider.go        # AuthProvider + TransportProvider interfaces
│   │   ├── simple.go          # Simple (OS user) auth
│   │   ├── basic.go           # HTTP Basic auth
│   │   ├── oauth_static.go    # OAuth2 static bearer token
│   │   ├── oauth_credentials.go # OAuth2 client credentials flow (auto-refresh)
│   │   └── kerberos.go        # Kerberos SPNEGO auth
│   ├── authentication.go      # Principal endpoint
│   ├── metalake.go, catalog.go, schema.go, fileset.go, topic.go, ...
├── models/                    # JSON-annotated structs (requests & responses)
│   ├── common.go              # ErrorResponse, Audit, NameIdentifier
│   ├── metalake.go, catalog.go, schema.go, fileset.go, topic.go, ...
├── provider/
│   └── provider.go            # Provider schema + resource/data source registration
├── resources/                 # Terraform Plugin Framework resources
│   ├── metalake/resource.go   # Schema + CRUD + Import
│   ├── catalog/resource.go
│   ├── schema/resource.go
│   └── ...
├── datasources/               # Data source implementations
│   └── ...
```

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
| `metadataObjectType` gains `VIEW`/`FUNCTION` (`openapi.yaml`) | `gravitino_statistics.resource_type`, `gravitino_credentials.resource_type` | rejected on read with an explicit diagnostic | accepted |
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
- [ ] Docs generated via `go generate ./...`

### Commands
- **Build**: `go build ./...`
- **Test (unit)**: `go test -v -cover ./internal/...`
- **Test (acceptance via Docker)**: `make testacc-docker`
- **Live acceptance (real server via podman)**: `make testacc-live` / `make testacc-live-filter F=<TestLiveAcc...>`
- **Lint**: `golangci-lint run --config .github/golangci.yml ./...`
- **Lint fix**: `make lint-fix`
- **Docs**: `go generate ./...` (uses `github.com/hashicorp/terraform-plugin-docs`)

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

### Logging

Use `tflog` from `github.com/hashicorp/terraform-plugin-log/tflog`:
- `tflog.Debug(ctx, msg, fields...)` — CRUD boundaries, before API calls
- `tflog.Warn(ctx, msg, fields...)` — non-fatal issues
- `tflog.Error(ctx, msg, fields...)` — errors returned as diagnostics

Fields use `map[string]interface{}` format. Logs respect `TF_LOG`, `TF_LOG_PATH`, `TF_LOG_PROVIDER` env vars.

### Dependencies
- Go 1.26.4
- `terraform-plugin-framework` v1.19.0
- `terraform-plugin-framework-validators` v0.19.0
- `terraform-plugin-testing` v1.16.0
- `github.com/jcmturner/gokrb5/v8` v8.4.4 (Kerberos auth)
