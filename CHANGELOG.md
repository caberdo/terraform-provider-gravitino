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
