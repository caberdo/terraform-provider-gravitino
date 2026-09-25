# Live acceptance test cases (against a real Gravitino)

These tests run against a **real** Gravitino server (`apache/gravitino:1.3.0`) via podman and
never start an HTTP mock of their own. They are recognisable by the `TestLiveAcc` prefix.

## Running them

```bash
make testacc-live                                       # all live tests
make testacc-live-filter F=TestLiveAccMetalakeResource  # a single test
```

`scripts/testacc-live.sh` discovers every package that contains `TestLiveAcc` tests, so a new
live test is picked up without editing the script.

**Guarantee that the responses do not come from a mock** (`acceptance.LivePreCheck` in
`internal/acceptance/live.go`):

1. The test skips when `GRAVITINO_URI` is not set.
2. The test fails when `GET /api/version` does not return a valid Gravitino version (empty
   version, or a version differing from `GRAVITINO_EXPECT_VERSION`).
3. The tests run inside the podman-compose network; the only reachable server on
   `http://gravitino:8090` is the real container.

## Test cases

| Test (function) | Resource / data source | Steps against a real server | Key assertions |
|---|---|---|---|
| `TestLiveAccMetalakeResource` | `gravitino_metalake` | create → update → import → (teardown: delete) | name/comment/properties; `audit.creator` == `gravitino_principal`; `audit.create_time` set; no drift after refresh (properties contain only the configured keys; the provider filters `in-use` out) |
| `TestLiveAccCatalogResource`, `...UpdateProperties` | `gravitino_catalog` (+ metalake) | create (hive, dummy `metastore.uris`) → read → update properties → (teardown: delete with `force=true`) | name/type `relational`/`catalog_provider` `hive`/comment/id; `properties.metastore.uris`; server-added properties (`in-use`, `gravitino.bypass.*`) do not reach state; `audit.creator` == principal |
| `TestLiveAccSchemaResourceProperties`, `...CommentReplaces` | `gravitino_schema` (+ metalake, catalog) | create → update properties → a comment change forces replacement | only `setProperty`/`removeProperty` are sent; `name`/`comment` → replace |
| `TestLiveAccFilesetResourceProperties`, `...Rename` | `gravitino_fileset` (+ metalake, catalog, schema) | create → update properties (in place) → rename (in place) | the server-only property `default-location-name` does not reach state; rename sends `rename` and updates the id |
| `TestLiveAccTagResource` | `gravitino_tag` (+ metalake) | create → read → (teardown: delete) | name/comment; `audit.creator` == principal; `audit.create_time` set |
| `TestLiveAccRoleResource` | `gravitino_role` (+ metalake, catalog) | create with one securable object → add a second one through the privilege override → (teardown: delete) | `securable_objects` holds METALAKE/CATALOG; the server serialises lower case while state shows the canonical upper case; `securable_objects[].fullName` is **relative** to the metalake |
| `TestLiveAccPolicyResource` | `gravitino_policy` (+ metalake) | create → update (comment/enabled/supported_object_types/custom_rules) | `supported_object_types` is serialised lower case and returned upper case; `custom_rules`/`enabled` converge without drift |
| `TestLiveAccUserResource`, `TestLiveAccGroupResource` | `gravitino_user`, `gravitino_group` (+ metalake) | create → refresh → (teardown: delete) | name/id/audit; requires authorization on the server |
| `TestLiveAccOwnerResource` | `gravitino_owner` (+ metalake, catalog, group) | create (owner of a catalog) → refresh | `object_full_name` is relative (`my_catalog`); `owner_type` is upper case in state despite the lower-case server response; id `metalake.CATALOG.my_catalog` |
| `TestLiveAccJobTemplateResource`, `...Spark`, `...InvalidCombination` | `gravitino_job_template` (+ metalake) | create (shell) → update (comment + arguments) → rename → spark variant; plus a combination that config validation must reject | exact register payload (`{jobTemplate:{…}}`), read-back for the audit information, rename targets the old name and updates the id |
| `TestLiveAccJobResource`, `...UnknownTemplate` | `gravitino_job` | create (run) → refresh → (teardown: cancel) | `job_id`/`status`/`audit`; id `metalake.job-…`; an unknown template produces a clear API error |
| `TestLiveAccModelResource` | `gravitino_model` (+ model catalog) | create → rename/comment/properties update → import | `latest_version` is computed; an in-place rename does not produce an inconsistent result |
| `TestLiveAccModelVersionResource` | `gravitino_model_version` (+ model) | link → read → import | the version number is assigned by the server (computed); `uris` is a map |
| `TestLiveAccHealthDataSources` | `gravitino_health`, `gravitino_liveness`, `gravitino_readiness` | read | status == `"up"` (the real server uses lower case, unlike many mocks) |
| `TestLiveAccPrincipalDataSource` | `gravitino_principal` | read | name == `"anonymous"` (the default principal of a real server without authentication) |
| `TestLiveAccMetalakeDataSources` | `gravitino_metalakes` (list), `gravitino_metalake` (get) + `gravitino_metalake` resource | create → get through the data source → list → (teardown: delete) | data source name/comment match the metalake that was created |

## Known server quirks around jobs and templates

- `DELETE /metalakes/{m}/jobs/templates/{t}` answers HTTP 409 `InUseException` as long as job
  runs are associated with the template, and a non-existent template answers HTTP 200
  `{"dropped":false}` (not 404).
- A job that is cancelled while queued stays in `cancelling` until a job executor picks up the
  cancellation, which keeps the template in use. The job live test therefore manages the
  template outside Terraform, and `gravitino_job`'s destroy waits at most 10s (best effort) for a
  terminal status.
- `DELETE /metalakes/{m}/jobs/runs/{jobId}` does not exist (HTTP 405): destroy cancels instead.

## Why this exposed real bugs

On its first runs the suite found two deviations between the assumptions in the mock tests and
the behaviour of the real server, both fixed since:

- **`gravitino_principal`**: `GET /api/authn/me` returns `principal` as a string
  (`"anonymous"`), not as an object with `name`/`roles`. (fix: commit `5d48ad7`)
- **`gravitino_catalog`**: the server adds `in-use` and `gravitino.bypass.*` to the catalog
  properties itself; the resource wrote those into state → drift
  `.properties: new element "in-use" has appeared`. (fix: commit `2505383`)

## What a stock 1.3.0 server does and does not support

Measured against `apache/gravitino:1.3.0` (default configuration, `auth=none`). This determines
which resources can be tested live and with which catalog provider:

| Catalog provider | Supports | Does **not** support |
|---|---|---|
| `fileset` (`type = "fileset"`) | catalogs, schemas, filesets, tags, policies, job templates/jobs, users/groups/roles/owners (with authorization) | tables, views, functions, partitions, statistics, topics |
| `model` (`type = "model"`, `properties = { uri = "file:/..." }`) | models and model versions | everything else |

Server configuration requirements:

- **roles / owners / users / groups**: require `gravitino.authorization.enable=true`; without that
  flag those endpoints answer HTTP 405 `UnsupportedOperationException` (error code `1006`).
  `docker-compose.yml` therefore enables the flag. Note that users/groups depend on
  authorization, not on a backing service.
- **idp_user / idp_group**: require the `idp-basic` plugin plus `gravitino.authenticators = basic`
  (incompatible with the default `simple`), `gravitino.server.rest.extensionPackages =
  org.apache.gravitino.idp.web.rest.feature` and an initial admin password. On a default server
  `/api/idp/*` answers an empty 404.
- **secrets**: the secrets API only exists from Gravitino 1.4 onwards; on 1.3.0 that route
  answers 404.

## Case sensitivity of enum values (important)

Gravitino accepts enum values **case-insensitively on input**, but always serialises them in
**lower case**. Measured examples:

```
POST /api/metalakes/authz_ml/roles   {"securableObjects":[{"type":"METALAKE",
     "privileges":[{"name":"CREATE_CATALOG","condition":"ALLOW"}]}]}   -> 200
GET  /api/metalakes/authz_ml/roles/r1                                   -> "type":"metalake",
     "name":"create_catalog", "condition":"allow"
PUT  /api/metalakes/authz_ml/owners/CATALOG/c1  {"name":"anonymous","type":"USER"} -> 200
GET  /api/metalakes/authz_ml/owners/CATALOG/c1  -> {"owner":{"name":"anonymous","type":"user"}}
POST /api/metalakes/{ml}/policies  content.supportedObjectTypes ["CATALOG"] -> ["catalog"]
```

Providers must therefore normalise the server value to the canonical (upper case) form before it
reaches state; otherwise every apply fails with "Provider produced inconsistent result after
apply".

Also measured: the `metadataObjectType` path segment is **upper case and singular** —
`/objects/CATALOG/...` answers 200, `/objects/catalogs/...` answers 400 `IllegalArgumentException`.

## Verifying payloads against a running server

A quick way to check whether the real server accepts a request body (this caught several
mock-versus-real deviations):

```bash
podman compose up -d gravitino
curl -s http://localhost:8090/api/version
curl -s -X POST http://localhost:8090/api/metalakes -H 'Content-Type: application/json' \
  -d '{"name":"probe_ml"}'
curl -s -X POST http://localhost:8090/api/metalakes/probe_ml/jobs/templates \
  -H 'Content-Type: application/json' \
  -d '{"jobTemplate":{"name":"t","jobType":"shell","executable":"/bin/echo"}}'
```

Unknown JSON fields are **not** ignored: you get
`UnrecognizedPropertyException: Unrecognized field "x" (class ...)`. That is exactly why mock
payloads must come from the OpenAPI specification and not from the Go structs.

## Out of scope (server level, no backing services)

- table / view / function / partition / statistics: require a lakehouse catalog (hive/iceberg);
  a `fileset` catalog answers `Catalog does not support table operations`. There is therefore no
  live coverage for them in CI — changes to those resources stay based on the OpenAPI
  specification and mock tests.
- topic: requires a Kafka catalog.
- idp_user / idp_group: require the `idp-basic` configuration described above.
