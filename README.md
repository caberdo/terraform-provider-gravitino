# Terraform Provider for Apache Gravitino

[![CI](https://img.shields.io/github/actions/workflow/status/caberdo/terraform-provider-gravitino/test.yml?branch=main&label=CI)](https://github.com/caberdo/terraform-provider-gravitino/actions/workflows/test.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/caberdo/terraform-provider-gravitino/badge)](https://scorecard.dev/viewer/?uri=github.com/caberdo/terraform-provider-gravitino)
[![Release](https://img.shields.io/github/v/release/caberdo/terraform-provider-gravitino?label=release)](https://github.com/caberdo/terraform-provider-gravitino/releases)
[![Terraform Registry](https://img.shields.io/badge/registry-caberdo%2Fgravitino-7b42bc)](https://registry.terraform.io/providers/caberdo/gravitino)
[![Go](https://img.shields.io/github/go-mod/go-version/caberdo/terraform-provider-gravitino)](https://golang.org/doc/devel/release.html)
[![License](https://img.shields.io/github/license/caberdo/terraform-provider-gravitino)](LICENSE)
[![Last commit](https://img.shields.io/github/last-commit/caberdo/terraform-provider-gravitino)](https://github.com/caberdo/terraform-provider-gravitino/commits/main)

Manage **[Apache Gravitino](https://gravitino.apache.org/)** — the unified metadata lakehouse —
as code. This provider speaks the Gravitino REST API directly, so metalakes, catalogs, schemas,
tables, views, functions, models, tags, policies, roles, users, groups and jobs live in the same
Terraform state as the rest of your infrastructure.

**21 resources and 50 data sources.**

| | |
|---|---|
| 🎯 **Broad Gravitino coverage** | Metalakes, catalogs, schemas, tables and partitions, views, filesets, topics, functions, models and model versions, tags, policies, roles, users, groups, ownership, job templates and job runs, IDP users and groups |
| 🧪 **Tested against real servers** | 524 tests run by `make test` and 615 in the container suite, which runs against Gravitino `1.3.0` and `1.3.1` (`.github/gravitino-versions.txt`) |
| 🧱 **Version gates, not opaque 400s** | One build serves both server versions: the schema accepts the union and a runtime gate refuses a 1.3.1-only value against an older server with a diagnostic naming the required version (`Gravitino >= 1.3.1`) |
| 🔐 **Verifiable supply chain** | GPG-signed release tags, signed `SHA256SUMS`, SLSA build provenance, SPDX SBOM and the Terraform registry manifest in every release |
| ⚡ **Modern internals** | terraform-plugin-framework v1.19.0 (plugin protocol 6.0), Go 1.27.1, Docker/Podman acceptance harness against a real server |

## Contents

- [Quick start](#quick-start)
- [Requirements and compatibility](#requirements-and-compatibility)
- [Supported Gravitino versions](#supported-gravitino-versions)
- [Provider configuration](#provider-configuration)
- [Authentication](#authentication)
- [Resources](#resources)
- [Data sources](#data-sources)
- [Examples](#examples)
- [Verifying a release](#verifying-a-release)
- [Testing](#testing)
- [Development](#development)
- [Troubleshooting and FAQ](#troubleshooting-and-faq)
- [Project documents](#project-documents)
- [Security, contributing and support](#security-contributing-and-support)
- [License](#license)

## Quick start

```terraform
terraform {
  required_providers {
    gravitino = {
      source  = "caberdo/gravitino"
      version = "~> 0.9"
    }
  }
}

provider "gravitino" {
  uri      = "http://localhost:8090"
  auth     = "basic"
  username = "admin"
  password = "admin"
}

resource "gravitino_metalake" "lake" {
  name    = "analytics"
  comment = "Metadata for the analytics platform"
}

resource "gravitino_catalog" "hive" {
  metalake         = gravitino_metalake.lake.name
  name             = "warehouse"
  type             = "relational"
  catalog_provider = "hive"
  properties = {
    "metastore.uris" = "thrift://localhost:9083"
  }
}

resource "gravitino_schema" "bronze" {
  metalake = gravitino_metalake.lake.name
  catalog  = gravitino_catalog.hive.name
  name     = "bronze"
  comment  = "Raw ingestion layer"
}

resource "gravitino_table" "events" {
  metalake = gravitino_metalake.lake.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.bronze.name
  name     = "events"
  comment  = "Raw event stream"

  column {
    name = "id"
    type = "integer"
  }
  column {
    name = "payload"
    type = "string"
  }
}
```

```bash
terraform init
terraform apply
```

Want a local server to try it against? The repository ships the same Docker Compose set-up that
CI uses for the acceptance suite:

```bash
docker compose up -d gravitino    # Gravitino 1.3.0, REST API on http://localhost:8090
terraform init && terraform apply
```

The environment can carry the connection instead of the provider block:

```bash
export GRAVITINO_URI="http://localhost:8090"
export GRAVITINO_AUTH="basic"
export GRAVITINO_USERNAME="admin"
export GRAVITINO_PASSWORD="admin"
```

## Requirements and compatibility

| | |
|---|---|
| Terraform | 1.0 or later (plugin protocol 6.0, see [`terraform-registry-manifest.json`](./terraform-registry-manifest.json)) |
| Gravitino | API v1.3.0, and v1.3.1 from the same build; acceptance-tested against `1.3.0` and `1.3.1` (`.github/gravitino-versions.txt`) |
| Go (to build from source) | 1.27.1 |

Some capabilities depend on server-side configuration and only work once the matching Gravitino
feature is enabled — roles, owners, users and groups on `gravitino.authorization.enable`, IDP users
and groups on the IDP plugin, jobs on a configured job executor. Until then the server answers with
its own error (HTTP 405 or 404), which the provider surfaces. See
[Troubleshooting and FAQ](#troubleshooting-and-faq).

## Supported Gravitino versions

The provider targets the Apache Gravitino **v1.3.0** REST API and supports **v1.3.1** from the
same build. It asks the server for its version once, when it is configured (`GET /api/version`),
and against a server older than 1.3.1 a value that only exists on 1.3.1 fails with an explicit
diagnostic naming the required version (`requires Gravitino >= 1.3.1`, or `requires Gravitino
1.3.1 or newer` for the metadata object types) instead of the server's opaque `400`.
Version-restricted attributes and data sources say so in their schema description.

| Capability | 1.3.0 | 1.3.1 |
|---|---|---|
| `gravitino_policy.supported_object_types` (`VIEW`, `FUNCTION`) | rejected at create/update | accepted |
| `gravitino_statistics` / `gravitino_credentials` `resource_type` (`VIEW`, `FUNCTION`) | rejected on read | accepted |
| `gravitino_table` index `index_type` (`data_skipping_minmax`, `data_skipping_bloom_filter`, `data_skipping_set`) and index `properties` | rejected at create (an index change replaces the table) | accepted |
| `gravitino_table` / `gravitino_view` column type `external` (`{"type":"external","catalogString":"..."}`) | rejected at create/update | accepted |
| `gravitino_principal.service_admin` | always `false` | reported |
| `gravitino_iceberg_rest_service` data source | endpoint does not exist | returns `uri`, or `null` when the service is unavailable |
| `gravitino_catalog_connection_test`, existing-catalog variant | endpoint does not exist | supported |
| `gravitino_catalog_connection_test`, proposed-configuration variant | supported | supported |

## Provider configuration

The provider supports the following arguments; each one can also be set through its environment
variable, and the environment wins when both are set.

| Attribute | Type | Env Variable | Description |
|-----------|------|-------------|-------------|
| `uri` | `string` | `GRAVITINO_URI` | Gravitino server URI |
| `auth` | `string` | `GRAVITINO_AUTH` | Auth method: `none`, `simple`, `basic`, `oauth`, or `kerberos` |
| `username` | `string` | `GRAVITINO_USERNAME` | Username (simple/basic auth) |
| `password` | `string` (sensitive) | `GRAVITINO_PASSWORD` | Password (basic auth) |
| `oauth_token` | `string` (sensitive) | `GRAVITINO_OAUTH_TOKEN` | Static OAuth2 bearer token |
| `oauth_client_id` | `string` | `GRAVITINO_OAUTH_CLIENT_ID` | OAuth2 client ID (client credentials flow) |
| `oauth_client_secret` | `string` (sensitive) | `GRAVITINO_OAUTH_CLIENT_SECRET` | OAuth2 client secret |
| `oauth_server_uri` | `string` | `GRAVITINO_OAUTH_SERVER_URI` | OAuth2 server URI |
| `oauth_token_path` | `string` | `GRAVITINO_OAUTH_TOKEN_PATH` | OAuth2 token endpoint path |
| `oauth_scope` | `string` | `GRAVITINO_OAUTH_SCOPE` | OAuth2 scope |
| `kerberos_principal` | `string` | `GRAVITINO_KERBEROS_PRINCIPAL` | Kerberos principal |
| `kerberos_keytab` | `string` (sensitive) | `GRAVITINO_KERBEROS_KEYTAB` | Path to keytab file |
| `kerberos_use_ticket_cache` | `bool` | `GRAVITINO_KERBEROS_USE_TICKET_CACHE` | Use OS ticket cache |

## Authentication

The provider supports five authentication methods: `none` (default), `simple`, `basic`, `oauth`,
and `kerberos`. Set the `auth` attribute to choose a method.

### None (no authentication)

```hcl
provider "gravitino" {
  uri  = "http://localhost:8090"
  auth = "none"
}
```

### Simple (OS user)

Uses the current OS user or the `GRAVITINO_USER` environment variable.

```hcl
provider "gravitino" {
  uri  = "http://localhost:8090"
  auth = "simple"
}
```

### Basic (username/password)

```hcl
provider "gravitino" {
  uri      = "http://localhost:8090"
  auth     = "basic"
  username = "admin"
  password = var.gravitino_password
}
```

### OAuth2 static bearer token

```hcl
provider "gravitino" {
  uri         = "http://localhost:8090"
  auth        = "oauth"
  oauth_token = var.gravitino_token
}
```

### OAuth2 client credentials flow (auto-refresh)

```hcl
provider "gravitino" {
  uri                = "http://localhost:8090"
  auth               = "oauth"
  oauth_client_id     = var.oauth_client_id
  oauth_client_secret = var.oauth_client_secret
  oauth_server_uri    = "http://localhost:8177"
  oauth_token_path    = "/oauth2/token"
  oauth_scope         = "test"
}
```

### Kerberos (keytab)

```hcl
provider "gravitino" {
  uri                 = "http://gravitino.example.com"
  auth                = "kerberos"
  kerberos_principal  = "HTTP/gravitino.example.com@EXAMPLE.COM"
  kerberos_keytab     = "/etc/security/gravitino.keytab"
}
```

### Kerberos (ticket cache)

```hcl
provider "gravitino" {
  uri                      = "http://gravitino.example.com"
  auth                     = "kerberos"
  kerberos_principal        = "HTTP/gravitino.example.com@EXAMPLE.COM"
  kerberos_use_ticket_cache = true
}
```

## Resources

Every resource links to its documentation on the Terraform Registry.

| Resource | Manages |
|---|---|
| [`gravitino_metalake`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/metalake) | Metalakes, the top-level namespace of a Gravitino instance |
| [`gravitino_catalog`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/catalog) | Catalogs inside a metalake (relational, fileset, messaging, model) |
| [`gravitino_schema`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/schema) | Schemas inside a catalog |
| [`gravitino_table`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/table) | Tables, with columns, partitioning, distribution, sort order and indexes |
| [`gravitino_partition`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/partition) | Partitions of a table |
| [`gravitino_view`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/view) | Views and their SQL representations |
| [`gravitino_fileset`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/fileset) | Managed and external filesets |
| [`gravitino_topic`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/topic) | Messaging topics |
| [`gravitino_function`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/function) | User-defined functions, their definitions and implementations |
| [`gravitino_model`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/model) | ML models in a model catalog |
| [`gravitino_model_version`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/model_version) | Model versions, URIs and aliases |
| [`gravitino_tag`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/tag) | Tags for classifying metadata objects |
| [`gravitino_policy`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/policy) | Access-control policies and their custom rules |
| [`gravitino_role`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/role) | Roles and the privileges granted on securable objects |
| [`gravitino_user`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/user) | Platform users and their roles |
| [`gravitino_group`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/group) | Groups and their members |
| [`gravitino_owner`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/owner) | The owner of a metadata object |
| [`gravitino_job_template`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/job_template) | Reusable job templates |
| [`gravitino_job`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/job) | Job runs submitted from a template |
| [`gravitino_idp_user`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/idp_user) | Users of the built-in identity provider |
| [`gravitino_idp_group`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/resources/idp_group) | Groups of the built-in identity provider |

## Data sources

Every data source links to its documentation on the Terraform Registry.

### Metalakes

| Data source | Returns |
|---|---|
| [`gravitino_metalakes`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/metalakes) | All metalakes of the server |
| [`gravitino_metalake`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/metalake) | One metalake by name |

### Catalogs and schemas

| Data source | Returns |
|---|---|
| [`gravitino_catalogs`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/catalogs) | Catalogs of a metalake |
| [`gravitino_catalog`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/catalog) | One catalog by name |
| [`gravitino_catalog_connection_test`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/catalog_connection_test) | Connection test result for a proposed configuration, or for an existing catalog on Gravitino 1.3.1+ |
| [`gravitino_schemas`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/schemas) | Schemas of a catalog |
| [`gravitino_schema`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/schema) | One schema by name |

### Tables, partitions, views, filesets, topics and functions

| Data source | Returns |
|---|---|
| [`gravitino_tables`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/tables) | Tables of a schema |
| [`gravitino_table`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/table) | One table, with columns, partitioning and indexes |
| [`gravitino_partitions`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/partitions) | Partitions of a table |
| [`gravitino_partition`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/partition) | One partition |
| [`gravitino_partition_statistics`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/partition_statistics) | Partition statistics of a table |
| [`gravitino_views`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/views) | Views of a schema |
| [`gravitino_view`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/view) | One view |
| [`gravitino_filesets`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/filesets) | Filesets of a schema |
| [`gravitino_fileset`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/fileset) | One fileset |
| [`gravitino_topics`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/topics) | Topics of a schema |
| [`gravitino_topic`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/topic) | One topic |
| [`gravitino_functions`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/functions) | Functions of a schema |
| [`gravitino_function`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/function) | One function |

### Models

| Data source | Returns |
|---|---|
| [`gravitino_models`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/models) | Models of a schema |
| [`gravitino_model`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/model) | One model |
| [`gravitino_model_versions`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/model_versions) | Versions of a model |
| [`gravitino_model_version`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/model_version) | One model version |

### Tags, policies, roles, ownership and statistics

| Data source | Returns |
|---|---|
| [`gravitino_tags`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/tags) | Tags of a metalake |
| [`gravitino_tag`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/tag) | One tag |
| [`gravitino_policies`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/policies) | Policies of a metalake |
| [`gravitino_policy`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/policy) | One policy |
| [`gravitino_roles`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/roles) | Roles granted on a metadata object |
| [`gravitino_roles_list`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/roles_list) | Roles defined in a metalake |
| [`gravitino_role`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/role) | One role, with its privileges |
| [`gravitino_owner`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/owner) | The owner of a metadata object |
| [`gravitino_credentials`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/credentials) | Credentials of a metadata object |
| [`gravitino_secrets`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/secrets) | Resolved secrets of a metadata object (Gravitino 1.4+) |
| [`gravitino_statistics`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/statistics) | Statistics of a metadata object |

### Users, groups, jobs and server state

| Data source | Returns |
|---|---|
| [`gravitino_users`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/users) | Users of a metalake |
| [`gravitino_user`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/user) | One user |
| [`gravitino_groups`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/groups) | Groups of a metalake |
| [`gravitino_group`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/group) | One group |
| [`gravitino_idp_user`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/idp_user) | A user of the built-in identity provider |
| [`gravitino_idp_group`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/idp_group) | A group of the built-in identity provider |
| [`gravitino_jobs`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/jobs) | Job runs of a metalake |
| [`gravitino_job`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/job) | One job run by id |
| [`gravitino_job_templates`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/job_templates) | Job templates of a metalake |
| [`gravitino_job_template`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/job_template) | One job template |
| [`gravitino_principal`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/principal) | The authenticated principal (and `service_admin` on Gravitino 1.3.1+) |
| [`gravitino_health`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/health) | Aggregate server health |
| [`gravitino_liveness`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/liveness) | Server liveness |
| [`gravitino_readiness`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/readiness) | Server readiness |
| [`gravitino_iceberg_rest_service`](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs/data-sources/iceberg_rest_service) | The advertised Iceberg REST endpoint (Gravitino 1.3.1+) |

## Examples

| Path | What it shows |
|---|---|
| [`examples/complete`](./examples/complete) | A full data-platform configuration: metalake, five catalogs, schemas, tables with partitioning, views, functions, topics, filesets, models, tags, policies, users, groups, roles, owners and jobs |
| [`examples/resources/<resource>`](./examples/resources) | One `resource.tf` per resource, embedded verbatim in the generated docs |
| [`examples/data-sources/<data source>`](./examples/data-sources) | One `data-source.tf` per data source |
| [`examples/provider`](./examples/provider) | Provider configuration |

Example files are published to the Registry as provider documentation, so an example that
references a removed attribute ships a broken snippet to users. `make validate-examples` builds
the provider from the working tree, points Terraform at it through `dev_overrides`, and runs
`terraform validate` on every example; CI runs it on every pull request.

## Verifying a release

Every release is built by [GoReleaser](https://goreleaser.com/) in GitHub Actions from a GPG-signed
tag, and ships:

| Asset | Purpose |
|---|---|
| `terraform-provider-gravitino_<version>_<os>_<arch>.zip` | The provider binaries for linux, darwin and windows, amd64 and arm64 |
| `terraform-provider-gravitino_<version>_SHA256SUMS` | Checksums for all artifacts, including the registry manifest |
| `terraform-provider-gravitino_<version>_SHA256SUMS.sig` | GPG signature over the checksums |
| `terraform-provider-gravitino_<version>_manifest.json` | The Terraform registry manifest (plugin protocol versions) |
| `sbom.spdx.json` | SPDX software bill of materials |
| `terraform-provider-gravitino_<version>_provenance.intoto.jsonl` | SLSA v1.0 build provenance over every checksummed artifact |

The release signing key is committed as
[`.github/release-signing-key.asc`](https://github.com/caberdo/terraform-provider-gravitino/blob/main/.github/release-signing-key.asc):

```bash
gpg --import .github/release-signing-key.asc     # fingerprint B6E21D8E7BE46C88492A7516BAD489AD0A93C1F6
gpg --verify terraform-provider-gravitino_0.9.0_SHA256SUMS.sig \
             terraform-provider-gravitino_0.9.0_SHA256SUMS
```

The checksum signature proves the checksums are ours; the build provenance proves *where the
binaries come from*. It is generated in the release workflow by
[`actions/attest-build-provenance`](https://github.com/actions/attest-build-provenance) and stored
in the repository's attestation record, so it is checkable with the GitHub CLI:

```bash
gh attestation verify terraform-provider-gravitino_0.9.0_linux_amd64.zip \
    --repo caberdo/terraform-provider-gravitino
```

That answers *which repository, tag, commit and workflow produced this archive*, and it fails if
the file was rebuilt or modified after the fact. The same provenance is attached to the release as
`terraform-provider-gravitino_<version>_provenance.intoto.jsonl`, so a release you already
downloaded verifies without a lookup in the attestation record:

```bash
gh attestation verify terraform-provider-gravitino_0.9.0_linux_amd64.zip \
    --repo caberdo/terraform-provider-gravitino \
    --bundle terraform-provider-gravitino_0.9.0_provenance.intoto.jsonl
```

Terraform itself verifies the checksums when it installs a provider and records them in
`.terraform.lock.hcl`; commit that file so every `terraform init` re-checks the same artifacts.
The release tag carries the **Verified** badge on GitHub because it is signed by the key
registered to the releasing account.

## Testing

| Command | What it runs |
|---|---|
| `make test` | The unit and schema suite plus the mock acceptance tests, which skip themselves without `TF_ACC`: 524 tests run, 91 skipped |
| `make testacc` | The same suite with `TF_ACC=1`, so the `TestAcc*` mock acceptance tests run too |
| `make testacc-docker` | The full 615-test suite in a Go container against Gravitino in Docker on `GRAVITINO_VERSION` (default `1.3.0`); the 1.3.1-only tests skip themselves on a 1.3.0 server |
| `make testacc-matrix` | `testacc-docker` once per supported server version (`.github/gravitino-versions.txt`, currently `1.3.0` and `1.3.1`) |
| `make testacc-live` | Starts Gravitino with `podman compose` and runs the 26 `TestLiveAcc*` tests against it — a real server, no HTTP mock |
| `make testacc-live-filter F=<TestName>` | One live test, e.g. `F=TestLiveAccMetalakeResource` |

CI runs the unit suite with `-race` on every pull request, and the acceptance matrix on every push
to `main`.

The suite is 615 tests in total:

| Group | Count | Needs |
|---|---|---|
| Unit and schema tests | 523 | Go only |
| Mock acceptance tests (`TestAcc*`) | 66 | `TF_ACC=1`; the server is an `httptest` instance |
| Live acceptance tests (`TestLiveAcc*`) | 26 | `GRAVITINO_URI` pointing at a real server, started by `make testacc-live` |

The live tests do not trust that address blindly: `acceptance.LivePreCheck` refuses to run unless
`GET /api/version` answers with a non-empty version and it matches `GRAVITINO_EXPECT_VERSION`
(defaulting to `GRAVITINO_VERSION`), so a mock cannot be mistaken for a server.

```bash
make test                                        # unit + mock suite
make testacc-docker                              # container suite, Gravitino 1.3.0
GRAVITINO_VERSION=1.3.1 make testacc-docker      # same suite, Gravitino 1.3.1
make testacc-matrix                              # every supported version
make testacc-live                                # live suite via podman
make testacc-live-filter F=TestLiveAccRoleResource
```

### Coverage

`make coverage` runs the unit suite alone (`TestAcc*` and `TestLiveAcc*` skip themselves), writes
`coverage.out` and prints a per-package table, weakest package first; `make coverage-html` opens
the same profile as HTML. Because packages whose behaviour is asserted end-to-end read low on the
unit suite alone, the acceptance container also writes `coverage-acceptance.out`
(`-covermode=atomic -coverpkg=./internal/...`), and `make coverage-merge` merges both profiles into
`coverage-union.out` and prints the same table.

```sh
make coverage             # unit profile + per-package table
make coverage-html        # same run, then the HTML report
make testacc-docker       # acceptance container, writes coverage-acceptance.out
make coverage-merge       # merged profile + per-package table
```

The `test` job publishes the unit table in its job summary and the `acceptance` job the merged one.

`-coverpkg` merges coverage across packages; a bare `-coverprofile` reports the owning package only,
which understates the numbers badly (the client reads 14% there because its callers live in other
packages). Measured at revision `9dbf159`:

| Command | Statement coverage |
|---|---|
| `go test -coverpkg=./... -coverprofile=c.out ./internal/...` | 75.5% (unit only) |
| `TF_ACC=1 go test -coverpkg=./... -coverprofile=c.out ./internal/...` | 78.0% (unit + acceptance) |
| `go test -coverprofile=c.out ./internal/...` | 59.8% (per package) |
| `TF_ACC=1 go test -coverprofile=c.out ./internal/...` | 61.6% (per package, unit + acceptance) |

Statement counts come from the cover profile, so they depend on the Go patch release that produced
it — `go 1.27.1` (the `go.mod` toolchain, which CI installs) sums `./internal/...` to 9766
statements, `go 1.27.2` to 7661 for the same tree. On the committed tooling CI reports 65.8%
(6424/9766) for the unit profile and 82.1% (8021/9766) with the acceptance profile merged.

## Development

```bash
make build              # build ./bin/terraform-provider-gravitino
make install            # install into ~/.terraform.d/plugins for local dev_overrides
make test               # unit and mock tests
make testacc-docker     # acceptance suite in Docker
make lint vet fmt       # golangci-lint (.github/golangci.yml), go vet, gofmt
make generate           # regenerate docs/ with terraform-plugin-docs
make validate-examples  # check examples/ against the provider schema
```

To use a local build with Terraform, add to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "caberdo/gravitino" = "/path/to/terraform-provider-gravitino/bin"
  }
  direct {}
}
```

[`AGENTS.md`](./AGENTS.md) describes the repository conventions: the schema rules, the error and
logging patterns, the version-gate shapes, and the checklist for adding a resource.

## Troubleshooting and FAQ

**A role, owner, user or group call fails with HTTP 405 `UnsupportedOperationException`.**
Those endpoints need `gravitino.authorization.enable=true` on the server. The provider does not
paper over it; it reports the server's response.

**`gravitino_idp_user` or `gravitino_idp_group` returns 404.**
`/api/idp/*` only exists when the built-in IDP plugin is loaded
(`gravitino.server.rest.extensionPackages=org.apache.gravitino.idp.web.rest.feature`) and the
`basic` authenticator is configured. On a default server the route is not there.

**A 1.3.1-only value is rejected with a diagnostic naming the required version.**
That is the version gate, not the server. The schema accepts every supported value and the runtime
compares the server version, so the diagnostic names the version instead of passing the value
through for an opaque `400`. Upgrade the server, or drop the value — see
[Supported Gravitino versions](#supported-gravitino-versions).

**`gravitino_secrets` fails on my server.**
The secrets API only exists on Gravitino 1.4 or newer; the data source needs that server.

**Table, view, function, partition or statistics operations are rejected by a fileset catalog.**
Those need a lakehouse catalog (Hive or Iceberg); a fileset catalog rejects those operations.

**A job fails to submit, or a job template cannot be deleted.**
Jobs need a job executor configured on the server. A job template can only be deleted once none of
its job runs is active.

**`gravitino_iceberg_rest_service` fails on Gravitino 1.3.0.**
`GET /api/system/iceberg-rest` does not exist before 1.3.1. The data source turns the endpoint's
`404` into the version diagnostic, so the failure names the required server version.

**Deleting something that is already gone does not fail.**
Read and delete are idempotent: an object that disappeared out of band is removed from state
instead of erroring.

**Which server version should I run?**
CI exercises `1.3.0` and `1.3.1`; `.github/gravitino-versions.txt` is the single source of truth
and is read by both the CI matrix and `make testacc-matrix`.

## Project documents

- [`ARCHITECTURE.md`](./ARCHITECTURE.md) — package layout, request flow, authentication, the
  Gravitino version gates, the test layers and the release pipeline.
- [`GOVERNANCE.md`](./GOVERNANCE.md) — who maintains the provider, how decisions are made, what CI
  enforces and how continuity is handled.
- [`ROADMAP.md`](./ROADMAP.md) — what is planned next, and what is explicitly out of scope.
- [`.github/OPENSSF-BEST-PRACTICES.md`](./.github/OPENSSF-BEST-PRACTICES.md) — the OpenSSF Best
  Practices evidence pack, criterion by criterion, with the command behind every figure.
- [`NOTICE`](./NOTICE) — the upstream works this provider derives from and their licences.

## Security, contributing and support

- [`SECURITY.md`](./.github/SECURITY.md) — how to report a vulnerability.
- [`CONTRIBUTING.md`](./.github/CONTRIBUTING.md) — how to get a change merged.
- [`SUPPORT.md`](./SUPPORT.md) — where to ask questions.
- [`CODE_OF_CONDUCT.md`](./CODE_OF_CONDUCT.md) — the ground rules for the community.
- [`CHANGELOG.md`](./CHANGELOG.md) — what changed in every release.
- [Provider documentation](https://registry.terraform.io/providers/caberdo/gravitino/latest/docs)
  — the generated resource and data source reference.

## License

Apache 2.0 — see the [LICENSE](LICENSE) file.
