## 0.7.0 (2026-09-25)

_Releases 0.5.0 through 0.6.2 were tagged without changelog entries._

BREAKING CHANGES:
- **Order-insensitive collections are now `set` attributes.** Attributes that
  Gravitino treats as unordered collections were modeled as lists, causing
  "inconsistent result after apply" when the server returned them in a different
  order than configured (e.g. role privileges on update). The following are now
  sets: `gravitino_role` `securable_objects`/`privileges`, `gravitino_user` and
  `gravitino_group` `roles`, `gravitino_idp_user` `groups`,
  `gravitino_idp_group` `users`, `gravitino_policy` `supported_object_types`,
  `gravitino_model_version` `aliases` (resources and data sources).
- **`gravitino_credentials` now models the real credential list.**
  `GET /metalakes/{metalake}/objects/{type}/{name}/credentials` returns
  `credentials`, a list of `{credentialType, expireTimeInMs, credentialInfo}`
  objects. The data source read a single `credential` object with
  `type`/`value`/`expireTime`, so against a real Gravitino every attribute came
  back empty. The data source now exposes `credentials` (list of objects) with
  `credential_type`, `expire_time_in_ms` (number, `0` means no expiry) and
  `credential_info` (sensitive map of credential type specific key/values). The
  attributes `type`, `value` and `expire_time` no longer exist.
- **`gravitino_principal` no longer exposes `roles`.** `GET /api/authn/me` returns
  `{code, principal}` only, so `roles` could never be populated (it was always an
  empty list). Use `data.gravitino_roles` for role information.
- **`gravitino_roles` now exposes `names`.** `GET
  /metalakes/{metalake}/objects/{metadataObjectType}/{metadataObjectFullName}/roles`
  returns a `NameListResponse` (`{code, names}`), not role objects. The data source
  decoded a non-existent `roles` list of `{name, privileges, securable_object}`
  objects, so against a real Gravitino every result was empty. The `roles` attribute
  no longer exists; use `names` (list of role names).
- **`gravitino_role` `properties` and `name` force replacement.** Gravitino v1.3.0
  has no role rename or role property update endpoint (the only role write endpoint
  is `PUT /metalakes/{metalake}/permissions/roles/{role}` for a privilege override),
  so changing `name` or `properties` now destroys and recreates the role. Previously
  a `properties` change was silently dropped, which produced a perpetual diff and
  failed applies.

- **`gravitino_job` now models a job *run*.** Gravitino models jobs as job runs of a
  job template: `POST /metalakes/{metalake}/jobs/runs` starts one and the server
  assigns the id, so a user-supplied job name cannot exist. The resource is configured
  with `metalake`, `job_template` and `job_conf`, exposes `job_id`, `status`,
  `queued_at`, `started_at`, `finished_at` and `audit` as computed values, and uses
  `metalake.<job_id>` as its id (import with `metalake.job_id`); `name`, `template`,
  `parameters` and `schedule` no longer exist. Every configured attribute forces a
  replacement (a new run) and destroy *cancels* the run, because the API has no delete
  (`DELETE /jobs/runs/{jobId}` answers 405).
- **`gravitino_job_template` now models the real template object.** Registering sends
  `{"jobTemplate": {...}}` with `job_type` (`shell`/`spark`) plus that variant's fields
  (`executable`, `arguments`, `environments`, `custom_fields`, `scripts` for shell;
  `class_name`, `jars`, `files`, `archives`, `configs` for spark). The attributes
  `template`, `parameters` and `properties` are gone: the previous body was rejected by
  a real server with `Unrecognized field "name" (class ...JobTemplateRegisterRequest)`,
  and the register response is a bare `{code}`, so create now reads the template back.
  Updates use rename/updateComment/updateTemplate and `job_type` forces replacement
  (the variants are distinct object types). Gravitino refuses to delete a template that
  still has active job runs (HTTP 409 `InUseException`), so destroy the jobs first.
- **`gravitino_job`/`gravitino_jobs` data sources follow the run model:** the get data
  source takes `job_id` instead of `name`; the list data source returns job runs.
- **Internal design notes moved out of `docs/`** (`docs/superpowers/**`,
  `docs/live-acceptance-tests.md` → `contributing/`), because the Terraform Registry
  publishes everything under `docs/` as provider documentation.
- **Dead `/bulk` client and models removed** (no corresponding endpoint in the Gravitino
  v1.3.0 API and no callers).

FIXES:
- **Fix `Malformed json request` when updating schema, topic, view or function.**
  These resources sent update `@type` values that Gravitino does not support for
  the entity, so the server rejected the request with
  `{"code":1001,"type":"IllegalArgumentException","message":"Malformed json request"}`
  (Jackson `InvalidTypeIdException`). Per Gravitino's `*UpdateRequest` DTOs:
  - `gravitino_schema`: dropped `rename`/`updateComment`; `name` and `comment` now
    force replacement (Gravitino only supports `setProperty`/`removeProperty` for
    schemas).
  - `gravitino_topic`: dropped `rename`; `name` now forces replacement (topics have
    no rename).
  - `gravitino_view`: dropped `updateComment`; `comment` now forces replacement
    (views have no comment update; use rename/properties/`replaceView`).
  - `gravitino_function`: dropped `rename`/`setProperty`/`removeProperty`; `name`
    and `properties` now force replacement (functions only support comment and
    definition/implementation updates).
  Also added `UseStateForUnknown` plan modifiers on `id`/`audit` for these
  resources to avoid spurious `-> (known after apply)` diffs. Covered by new
  acceptance tests that assert no unsupported `@type` is ever sent.
- **Fix "inconsistent result after apply" for optional `comment` attributes.**
  `gravitino_schema`, `gravitino_view`, `gravitino_function`, `gravitino_model`, and
  `gravitino_topic` set `comment` to an empty string in state when the server returned
  no comment, while the config had `comment` omitted (null). The read functions now
  normalize an empty server comment to `null`, so apply no longer fails with
  `.comment: was null, but now cty.StringVal("")`. Covered by new acceptance tests.
- **Fix `gravitino_principal` decode against real Gravitino.** `GET /api/authn/me`
  returns `principal` as a plain string (e.g. `"anonymous"`), not an object with
  `name`/`roles`; the data source now decodes the real response shape.
- **Fix property drift on `gravitino_catalog`.** Real Gravitino adds reserved/derived
  catalog properties (`in-use`, `gravitino.bypass.*`) that are absent from the
  config. The resource now keeps only configured/known properties in state, so
  apply no longer fails with `.properties: new element "in-use" has appeared`.
- **`gravitino_credentials` object type enum is credential-specific.** The
  `resource_type` validator borrowed the statistics object type list; it now uses
  the exact `metadataObjectType` enum of the credentials endpoint
  (`models.CredentialObjectTypes`: METALAKE, CATALOG, SCHEMA, TABLE, COLUMN,
  FILESET, TOPIC, MODEL, ROLE), so future changes to the statistics list can no
  longer silently change which object types the data source accepts. Both
  `gravitino_credentials` and `gravitino_principal` now report failures through
  `client.NewResourceError`, so a Gravitino 404/401 diagnostic carries the real
  HTTP status plus the server error type and message.
- **Fix `gravitino_role` against real Gravitino.** The Go model decoded a role body
  with a singular `securableObject` plus a role level `privileges` array, which the
  API never returns: every securable object and privilege was lost (empty state, no
  drift detection). `gravitino_role` and `data.gravitino_role` now use the real
  `Role` model (`name`, `properties`, `securableObjects[].{fullName, type,
  privileges[].{name, condition}}`) plus the `audit` block the server always returns.
  Also:
  - the create request always contains `securableObjects` (verified against
    Gravitino: a request without it is rejected with HTTP 400
    `"securableObjects" can't null`), and may be an empty array;
  - privilege overrides send the complete desired list of securable objects, since
    `PUT /permissions/roles/{role}` replaces the whole set (verified live: an object
    omitted from `overrides` is removed from the role);
  - the values the server returns are always lower case (`metalake`,
    `create_catalog`, `allow`), although input is case-insensitive; the provider
    upper-cases them into state, which removes the "Provider produced inconsistent
    result after apply" failure for real servers;
  - `properties` uses `UseStateForUnknown` plus `RequiresReplaceIfConfigured`, so an
    unconfigured `properties` no longer turns any privilege update into a
    destroy/recreate cycle;
  - `Read` removes the role from state on a Gravitino 404 and `Delete` treats a 404
    as success, both reporting through `client.NewResourceError`; import guards
    against malformed `metalake.role` identifiers instead of panicking;
  - `securable_objects[].full_name` is documented as relative to the metalake
    (verified live: `my_catalog` is accepted for a CATALOG, while the prefixed
    `my_metalake.my_catalog` is rejected with HTTP 400
    `IllegalNamespaceException`), and the example plus a unit test pin the fact
    that the provider never adds a metalake prefix;
  - the object type validators (`gravitino_role.securable_objects[].type` and
    `data.gravitino_roles.resource_type`) use the exact `metadataObjectType` enum
    (`METALAKE`, `CATALOG`, `SCHEMA`, `TABLE`, `FILESET`, `TOPIC`, `ROLE`, `MODEL`,
    `FUNCTION`, `TAG`, `POLICY`, `JOB_TEMPLATE`). Verified against a live server:
    any casing of these values is accepted, while the plural form is rejected with
    HTTP 400 `No enum constant org.apache.gravitino.MetadataObject.Type.CATALOGS`.
    The description of both attributes no longer suggests `catalogs`/`schemas`/
    `tables`. Covered by new unit tests (create body equals the spec example, exact
    override payload, 404 handling, import guards) and acceptance tests against a
    spec-faithful fake server that returns lower case enums like the real one.

- **404 handling now works against a real Gravitino server.** `client.IsNotFoundError`
  matched the substrings `"404"`/`"Not Found"` in the error text, but a Gravitino error
  payload carries an application code in the 1000-1100 range and a message such as
  `Failed to operate metalake(s) [x] operation [LOAD], reason [NoSuchMetalakeException]`,
  which contains neither. A resource deleted out of band therefore produced a hard error
  instead of being removed from state (and any message containing "404" would have
  removed the resource from state by accident). Errors are now typed
  (`client.HTTPError`) and carry the real HTTP status; reads drop the resource from
  state, deletes treat 404 as success. Regression tests use the error payload from
  Gravitino's own OpenAPI examples.
- **Server errors keep their status, type and stack trace.** `NewResourceError` called
  `errors.As` with a value target while the client returns a pointer, so the structured
  branch was dead code and every failure degraded to `message (type)`. Diagnostics now
  include the HTTP status, the Gravitino exception type, the message and the server stack
  trace, plus a bounded body excerpt for non-JSON error responses (proxies, gateways).
- **The request context is no longer discarded.** Client methods take `ctx` and pass it
  to the HTTP request and to the auth provider, so a cancelled provider run aborts
  in-flight requests instead of waiting for the 30s client timeout.
- **Path segments are escaped consistently.** The metalake/user/group/role/owner and
  job-template request paths were built by string interpolation, so a name containing
  `/`, a space or `?` corrupted the request URL — owner and role use dotted object full
  names, which are especially likely to contain such characters.
- **Kerberos: the SPNEGO retry works again.** `hasNegotiateChallenge` only matched a bare
  `Negotiate` header while real servers send `Negotiate <token>`, and the retry reused an
  already consumed request body, sending an empty body with the original
  `Content-Length` on POST/PUT. Both fixed; the body is rewound via `req.GetBody`.
- **`GRAVITINO_KERBEROS_USE_TICKET_CACHE` reports invalid values** instead of silently
  falling back to `false`.
- **Releases ship the Terraform Registry manifest.** `.github/goreleaser.yml` did not
  include `terraform-registry-manifest.json` in the checksums or release assets, so the
  Registry could not read `protocol_versions`; it is now published as
  `terraform-provider-gravitino_<version>_manifest.json`.
- **The release workflow no longer runs twice per tag.** `create-release-tag.yml` pushed
  a `v*` tag (triggering `release.yml`) *and* called it via `workflow_call`, so two
  GoReleaser runs raced on the same release assets.
- **CI and tooling hardening.** `make lint`/`make lint-fix` use `.github/golangci.yml`
  (they previously ran with golangci-lint's default set, so local lint was weaker than
  CI); the test job runs `go build ./...` and `go vet ./...` over the whole module;
  gosec no longer excludes hardcoded credentials (G101); the compose test service fails
  fast (`set -e`) and enforces `GRAVITINO_EXPECT_VERSION`; the Gravitino service enables
  authorization so roles, owners, users and groups are covered by live tests;
  `.gitignore` covers key material (`*.pem`, `*.key`, `*.keytab`, …) and any
  `*.tfstate`; dependabot tracks the pinned container images.
- **Documentation corrections.** `README.md` claimed Go >= 1.22 (go.mod requires 1.26.4),
  used the reserved `provider` argument in its catalog example instead of
  `catalog_provider`, listed 13 of 21 resources and 34 of 47 data sources, and omitted
  the `none` auth method.

ENHANCEMENTS:
- Live acceptance tests (`TestLiveAcc*`) that run against a **real** Gravitino
  server via podman (`make testacc-live` / `make testacc-live-filter F=<test>`),
  covering metalake, catalog, tag, and the health/principal/metalake data sources.
  `acceptance.LivePreCheck` requires `GRAVITINO_URI` to answer `GET /api/version`
  so the tests never silently pass against a mock.
- **Example validation in CI.** `scripts/validate-examples.sh` (also
  `make validate-examples`) builds the provider, points Terraform at it through
  `dev_overrides` and runs `terraform validate` in every module under `examples/`.
  The resource snippets are embedded verbatim in the generated Registry documentation,
  so an example referencing a removed attribute published a broken snippet to users —
  and the script also surfaces provider schema errors. It runs in the test job.
- **Every resource ships an `import.sh`**, so the generated pages document import for
  all of them instead of silently omitting the section.
- **The provider index documents the server requirements per resource**:
  `gravitino_role`/`gravitino_owner`/`gravitino_user`/`gravitino_group` need
  `gravitino.authorization.enable=true`; `gravitino_idp_user`/`gravitino_idp_group` need
  the built-in IDP plugin plus the `basic` authenticator; `gravitino_secrets` needs
  Gravitino 1.4 or newer (the secrets API does not exist in 1.3.x); tables, views,
  functions, partitions and statistics need a lakehouse catalog.
- **Live coverage extended to 23 `TestLiveAcc*` tests.** New live tests cover
  `gravitino_job`/`gravitino_job_template` (shell + spark variants, rename, payload
  assertions), `gravitino_model`/`gravitino_model_version`, `gravitino_role`,
  `gravitino_policy`, `gravitino_user`, `gravitino_group` and `gravitino_owner`, next to
  the existing metalake/catalog/schema/fileset/tag/health/principal tests. They run
  against a real Gravitino 1.3.0 (`make testacc-live`), which now discovers the packages
  containing live tests instead of using a fixed list, so new live tests run in CI
  automatically; `docker-compose.yml` enables
  authorization so the role/owner/user/group endpoints are reachable. The suite already
  caught behaviour the mocks missed: Gravitino serialises enum values in lower case, and
  `securable_objects[].fullName` / the owner `object_full_name` are relative to the
  metalake.
- **Example validation is exact.** `scripts/validate-examples.sh` (Python helper
  `scripts/validate-examples.py`) validates the complete example modules in place and
  concatenates the per-resource snippets into one synthetic module, so every reference
  resolves and Terraform performs its full validation on each snippet. Validating the
  snippets on their own silently skipped "missing required argument" checks — that is how
  a `gravitino_partition` example shipped without the required `type` and a
  `gravitino_table` example kept using the removed `columns` argument.

- **Live test matrix documented** in `contributing/live-acceptance-tests.md`, including
  which catalog provider can host which resource, the measured case handling of enum
  values (accepted case-insensitively, always serialised lower case, so providers must
  canonicalise what they read) and short curl recipes to verify a payload against a
  running server.

- `client.GetVersion()` and `models.VersionResponse` for server-version verification.

## 0.4.6 (2026-09-09)

FIXES:
- **Fix "inconsistent result after apply" for `gravitino_role` securable objects.**
  Gravitino returns privilege names, conditions, and securable object types in
  lowercase (`create_catalog`, `allow`, `metalake`) even though the API accepts
  and the config uses uppercase (`CREATE_CATALOG`, `ALLOW`, `METALAKE`). The
  provider now normalizes these values to uppercase when reading them into state,
  matching the configured values and preventing drift.
- Applied the same normalization to the role data sources for consistency.

ENHANCEMENTS:
- Acceptance test `TestAccRoleResource_CreateWithLowercaseServerValues` and unit
  test `TestSecurableObjectsToTF_NormalizesToUppercase` guarding against the
  case-mismatch regression.

## 0.4.5 (2026-09-09)

FIXES:
- **Fix update failure "Property in-use is immutable or reserved, cannot be
  deleted"** for `gravitino_metalake` (and `gravitino_table`). Gravitino treats
  `in-use` as a reserved property managed via a dedicated endpoint; sending it as
  a regular `removeProperty`/`setProperty` update is rejected. The provider now
  filters reserved properties (`in-use`) out of create requests, state mapping,
  and update diffs so they are never touched as normal properties.

ENHANCEMENTS:
- Unit and acceptance tests covering reserved-property filtering and ensuring
  updates never emit `removeProperty("in-use")`.
- Documentation notes that the reserved `in-use` property is managed by
  Gravitino and filtered out.

## 0.4.4 (2026-09-09)

FIXES:
- **Fix provider panic "assignment to entry in nil map"** in `gravitino_metalake`
  Read. When the metalake had no configured properties (or was imported) and the
  server returned properties, the state merge wrote to a nil map. The merge now
  starts from an initialized map.

ENHANCEMENTS:
- Unit tests covering the nil-map case for `metalakeToState` and
  `mapTableResponseToState`, plus an import acceptance test with server-only
  properties.

## 0.4.3 (2026-09-09)

FIXES:
- **Fix perpetual drift on `properties` when the server drops a configured key**
  (e.g. the reserved `in-use` property). State now merges configured properties
  with server-returned ones, so keys Gravitino does not echo back are preserved
  instead of being dropped. Applies to `gravitino_metalake` and
  `gravitino_table`; both `properties` attributes are now `Optional + Computed`.
- **Fix "inconsistent result: .properties was cty.MapValEmpty, but now null" for
  `gravitino_table`** when `properties = {}` is configured and the server returns
  no properties (metalake already handled in 0.4.2).

ENHANCEMENTS:
- Acceptance tests that reproduce and guard against properties drift and empty
  maps: `TestAccMetalakeResource_NoDriftWithServerDroppedProperty` (incl.
  property removal on update), `TestAccTableResource_NoDriftWithServerDroppedProperty`,
  and `TestAccTableResource_CreateWithEmptyProperties`.

## 0.4.2 (2026-09-09)

FIXES:
- **Fix "Provider produced inconsistent result after apply: .audit was absent,
  but now present"** for `gravitino_metalake` and `gravitino_table`. The `audit`
  attribute was declared as a `SingleNestedBlock` (which cannot be marked
  `Computed`), so Terraform rejected the server-populated audit object in state.
  It is now a `Computed` object attribute, matching every other resource.
- **Fix "inconsistent result: .properties was cty.MapValEmpty, but now null"**
  for `gravitino_metalake` and `gravitino_table`. When `properties = {}` is
  configured (or the server returns no properties), the provider no longer
  overwrites the planned empty map with `null` in state.
- **Centralized audit conversion:** New shared `AuditAttrTypes` and
  `AuditToObjectValue` helpers in `internal/models/audit.go`, replacing
  per-package duplication and the root cause of the drift.
- **Consistent table column/distribution state:** `column` length/precision/scale
  and `comment` now produce state values matching their schema defaults instead
  of `null`, and `distribution.func_args` emits `null` instead of an empty list,
  eliminating further "inconsistent result after apply" errors.
- **Metalake data sources** (`gravitino_metalake`, `gravitino_metalakes`) now use
  the same `Computed` object `audit` attribute for consistency.

ENHANCEMENTS:
- Acceptance tests that reproduce and guard against the "inconsistent result
  after apply" bug: `TestAccMetalakeResource_CreateWithAudit`,
  `TestAccMetalakeResource_CreateWithEmptyProperties`, and
  `TestAccTableResource_CreateWithAudit`.

## 0.4.0 (2026-09-02)

BREAKING CHANGES:
- **Policy resource rewritten** for the Gravitino 1.3.0 policy model. The old
  `effect`, `actions`, `subjects`, `condition`, and `object` fields are replaced
  by the new `policy_type`, `enabled`, `supported_object_types`, `properties`,
  and `custom_rules` schema.

FEATURES:
- **Built-in IDP management:** New `gravitino_idp_user` and `gravitino_idp_group`
  resources and data sources for local authentication (password-based users,
  group membership, enable/disable).
- **Model versions:** New `gravitino_model_version` resource with `gravitino_model_version`
  and `gravitino_model_versions` data sources (link, list, get, update, delete,
  aliases, and URI resolution — 10 endpoints).
- **Job templates:** New `gravitino_job_template` resource with `gravitino_job_template`
  and `gravitino_job_templates` data sources (register/get/update/delete).
- **Secrets:** New `gravitino_secrets` data source exposing resolved plaintext
  secrets for a metadata object.
- **Bulk operations:** Client support for bulk user/group add/remove endpoints.

ENHANCEMENTS:
- **Access control coverage:** `gravitino_user`, `gravitino_group`, `gravitino_role`,
  and `gravitino_owner` resources with get/list data sources.
- **Centralized error handling:** New `client.NewResourceError` and
  `client.IsNotFoundError` helpers used consistently across all resources.
- **Structured logging:** `tflog.Debug` at all CRUD boundaries.
- **Enum validators:** `stringvalidator.OneOf` for all enum fields, backed by
  shared constants in `internal/models/privilege_names.go`.
- **Docker Compose acceptance tests** plus CI acceptance job and Makefile targets.

FIXES:
- Corrected REST paths to match the Gravitino 1.3.0 OpenAPI specification:
  - `/principal` → `/authn/me`
  - `/health/liveness` → `/health/live`, `/health/readiness` → `/health/ready`
  - credentials and statistics now use the `/objects/{type}/{fullName}` path
  - job runs moved to `/jobs/runs`, removed obsolete pause/resume
  - catalog `testConnection` path corrected
  - role privilege override now a single bulk call at `/permissions/roles/{role}`
- Added the `COLUMN` metadata object type.

## 0.3.1 (2026-07-27)

BREAKING CHANGES: None

ENHANCEMENTS:
- **Documentation:** Provider docs now show all 7 auth method examples with HCL snippets
- **Resource examples:** Enriched with more realistic configurations
  - Table: added `sort_orders` and `distribution` example
  - View: added `view_def` (SQL) and properties
  - Function: added `function_body` and properties
  - Partition: multiple partitions across different tables
- **Complete deployment example:** New `examples/complete/main.tf` showing all resources working together in a realistic data platform scenario
- **AGENTS.md:** Updated with auth architecture, missing resources (model_version, job_template), and gokrb5 dependency

## 0.3.0 (2026-07-27)

BREAKING CHANGES: None

FEATURES:
- **Provider authentication:** Full support for all 4 Gravitino auth methods
  - `simple` — OS user or `GRAVITINO_USER` environment variable
  - `basic` — HTTP Basic authentication (unchanged)
  - `oauth` — Static bearer token AND client credentials flow with auto-refresh
  - `kerberos` — SPNEGO authentication via keytab or ticket cache

ENHANCEMENTS:
- Provider config expanded with 8 new attributes: `oauth_client_id`, `oauth_client_secret`, `oauth_server_uri`, `oauth_token_path`, `oauth_scope`, `kerberos_principal`, `kerberos_keytab`, `kerberos_use_ticket_cache`
- Modular auth architecture for easier extensibility
- OAuth2 token refresh with thread-safe caching (90% expiry margin)
- Kerberos SPNEGO with 401 challenge-response retry handling
- New `none` auth option for explicit no-authentication configuration

NOTES:
- `auth = "basic"` and `auth = "oauth"` with `oauth_token` remain fully backward compatible
- Empty or unset `auth` still works (no authentication)
- `gokrb5/v8` added as dependency for Kerberos support

## 0.1.0 (Unreleased)

BREAKING CHANGES: None

FEATURES:
- **New Resource:** `gravitino_metalake`
- **New Resource:** `gravitino_catalog`
- **New Resource:** `gravitino_schema`
- **New Resource:** `gravitino_table` with full column, sort order, distribution, partitioning, and index support
- **New Resource:** `gravitino_tag`
- **New Resource:** `gravitino_fileset`
- **New Resource:** `gravitino_topic`
- **New Resource:** `gravitino_view`
- **New Resource:** `gravitino_function`
- **New Resource:** `gravitino_model`
- **New Resource:** `gravitino_partition`
- **New Resource:** `gravitino_policy`
- **New Resource:** `gravitino_job`
- Provider supports OAuth2 and HTTP Basic authentication
- All resources support import
- Enum validators for constrained string attributes
