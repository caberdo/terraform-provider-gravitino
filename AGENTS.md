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
| Job                   | `gravitino_job`           | `metalake.job`                 |
| Job Template          | `gravitino_job_template`  | `metalake.job`                 |
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
- **Update pattern**: Compare plan vs state for name/comment/properties, build `[]interface{}` update requests
- **404 handling**: `client.IsNotFoundError(err)` → `resp.State.RemoveResource(ctx)` (all resources)
- **Configure**: Casts `req.ProviderData` to `*client.Client`. For auth, the provider builds an `AuthProvider` via `buildAuthProvider()` and passes it to `client.New(uri, authProvider)`
- **Auth**: Uses `internal/client/auth/` package. The `AuthProvider` interface has `Header(ctx) (string, string, error)`. The `TransportProvider` (optional) has `WrapTransport(base) http.RoundTripper` for Kerberos SPNEGO.
- **Tests**: Use `httptest.NewServer` with custom handlers; test schema, create, delete, import
- **Enum validators**: Always check the Gravitino API docs for possible values and add `stringvalidator.OneOf(...)` to enum fields. Store shared enums as constants in `internal/models/privilege_names.go`.

### Checklist for New Resources

- [ ] Error handling via `client.NewResourceError`
- [ ] 404 via `client.IsNotFoundError`
- [ ] tflog.Debug at start/end of Create/Read/Update/Delete
- [ ] Import with dot-separated ID parsing (valid + invalid test)
- [ ] Unit tests: schema, create, delete, import
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
