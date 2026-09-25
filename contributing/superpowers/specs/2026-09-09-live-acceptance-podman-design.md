# Live Gravitino Acceptance Tests (podman) — Design

> **Date:** 2026-09-09
> **Status:** Approved

## Goal

Run acceptance tests against a **real** Gravitino server (via the `apache/gravitino:1.3.0`
Docker image, orchestrated with **podman** on the developer's laptop), and prove that the
server responses come from a genuine Gravitino instance and not a test mock.

## Problem

The existing `TestAcc*` acceptance tests in `internal/resources/metalake/acceptance_test.go`
and `internal/resources/table/acceptance_test.go` start their own `httptest.NewServer` mock
and override `GRAVITINO_URI` via `t.Setenv(...)` (e.g. `metalake/acceptance_test.go:68`).
This means:

- Even `docker compose run --rm test` (and the CI "Acceptance Tests" job in
  `.github/workflows/test.yml`) only ever talk to mocks.
- No test today verifies the provider's behaviour against a real Gravitino server, and no
  test can prove a response is not a mock.

## Verified facts (spike against `apache/gravitino:1.3.0` via podman)

These were observed by running a real container and issuing raw HTTP requests:

- `GET /api/version` returns `{"code":0,"version":{"version":"1.3.0","compileDate":"...","gitCommit":"..."}}`
  — version is a **nested** object; usable as an explicit "this is Gravitino" marker.
- `GET /api/health`, `/api/health/live`, `/api/health/ready` return `status` in **lowercase**
  (`"up"`).
- Metalake create/read/update/delete all work. The server sets `audit.creator == "anonymous"`
  (no auth configured) and **adds** the reserved property `in-use=true` that is absent from the
  create request.
- Catalog create works with a dummy `metastore.uris` property (no backing HMS needed); the
  server lowercases the `type` to `"relational"` and adds `in-use=true` and
  `gravitino.bypass.hive.metastore.client.capability.check=false`. The provider already drops
  catalogs with `?force=true` (`internal/client/catalog.go:48-53`).
- Tag create/read/delete works fully.
- User / group / role return error code `1006` unless the server is started with
  `gravitino.authorization.enable=true`. These are **out of scope** for this work.

## Podman orchestration

Environment verified on the developer machine:

- `podman` 6.1.1 with a running `podman-machine-default` (applehv, 6 CPUs / 8 GiB).
- `podman-compose` 1.6.0 (`podman compose`).
- `podman-compose` 1.6.0 **ignores** `depends_on: { condition: service_healthy }` — the
  `test` service must poll the Gravitino health endpoint itself before running tests.
- Service-to-service DNS resolution works (`test` can reach `http://gravitino:8090`).
- `resource.Test` runs terraform-core in-process: **no external `terraform` binary is required**,
  so tests run fine inside the `golang:1.26` container.

## The "not a mock" guarantee (three layers)

1. **The live test only runs against a real server.** A `PreCheck` skips the test when there is
   no live server to talk to. In the podman-compose `acc` profile the only reachable server is the
   real Gravitino container.
2. **Explicit version assertion.** The first live test performs `GET /api/version` (via the
   provider client) and fails if the response is not a valid Gravitino version object.
3. **Real-behaviour assertions.** The live tests assert server behaviours that mocks never
   produce: `audit.creator == "anonymous"`, server-added `in-use=true`, lowercase `relational`
   catalog type, lowercase `"up"` health status.

## Test matrix (server-level resources)

| Resource / data source | Coverage |
|------------------------|----------|
| `gravitino_metalake`   | create → read (assert `anonymous` creator + `in-use=true`) → update comment → import → delete |
| `gravitino_catalog`    | create (dummy `metastore.uris`), read, delete (force) |
| `gravitino_tag`        | create / read / delete |
| `gravitino_health` DS  | status `"up"` |
| `gravitino_authentication` DS | principal `"anonymous"` |
| metalake list/get DS   | list + get |

**Out of scope:** user / group / role / owner (require an auth-enabled server), and the full
schema/table/fileset/topic/view hierarchy (requires backing containers).

## Files to change

- **New:** `internal/client/version.go` — `GetVersion()` + `models.VersionResponse`.
- **New:** `internal/acceptance/live_test_util.go` — shared `PreCheck` (checks
  `GRAVITINO_URI` + live `/api/version`), provider factories, unique-name helpers.
- **New:** per-resource live acceptance tests:
  `internal/resources/metalake/live_acceptance_test.go`,
  `internal/resources/catalog/live_acceptance_test.go`,
  `internal/resources/tag/live_acceptance_test.go`,
  `internal/datasources/health/live_acceptance_test.go`,
  `internal/datasources/authentication/live_acceptance_test.go`.
- **Modify:** `docker-compose.yml` — add an `acc` profile with the Gravitino service and a
  golang test service whose entrypoint polls `http://gravitino:8090/health` before running.
- **Modify:** `GNUmakefile` — add a `testacc-podman` target.
- **Docs:** `docs/` (acceptance testing section in README/contributing).

## Design decisions

- **New tests alongside the existing mocks** (no refactor of the mock-based `TestAcc*` files).
- **Live tests are gated** so they never run accidentally against a mock or in normal unit runs:
  they require `TF_ACC=1` **and** a reachable live `GRAVITINO_URI` whose `/api/version` answers.
  The podman-compose `acc` profile provides exactly that.
- Mock-based `TestAcc*` tests that call `t.Setenv("GRAVITINO_URI", server.URL)` must be left
  untouched; the live tests live in separate files/packages so they are not affected.
