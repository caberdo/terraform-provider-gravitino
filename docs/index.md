---
page_title: "Gravitino Provider"
subcategory: ""
description: |-
  Terraform provider for Apache Gravitino, the unified metadata lakehouse: metalakes, catalogs, schemas, tables, views, filesets, topics, functions, models, tags, policies, roles, users, groups, ownership and jobs - 23 resources and 54 data sources.
---

# Gravitino Provider

Terraform provider for [Apache Gravitino](https://gravitino.apache.org/), the unified metadata
lakehouse. It speaks the Gravitino REST API directly, so metadata and access control live in the
same Terraform state as the rest of your infrastructure.

**23 resources and 54 data sources**, covering:

- **Metadata** - metalakes, catalogs (relational, fileset, messaging, model), schemas, tables with
  columns, partitioning and indexes, partitions, views, filesets, topics and functions
- **ML assets** - models and model versions
- **Governance** - tags, policies and their object associations, roles and privileges, ownership,
  users and groups
- **Jobs** - job templates and job runs
- **Identity provider** - IDP users and groups
- **Server state** - the authenticated principal, health, liveness, readiness, the Iceberg REST
  endpoint, statistics and credentials

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
}

resource "gravitino_table" "events" {
  metalake = gravitino_metalake.lake.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.bronze.name
  name     = "events"

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

The [README](https://github.com/caberdo/terraform-provider-gravitino#readme) documents every
resource, the complete resource and data source tables, release verification and a troubleshooting
FAQ.

## Requirements and compatibility

- Terraform 1.0 or later is required (plugin protocol 6.0).
- One provider build serves Apache Gravitino **v1.3.0** and **v1.3.1**. The provider asks the
  server for its version once, when it is configured (`GET /api/version`); CI runs the acceptance
  suite against both, from [`.github/gravitino-versions.txt`](https://github.com/caberdo/terraform-provider-gravitino/blob/main/.github/gravitino-versions.txt).
- The schema accepts the union of both versions and a runtime gate refuses a 1.3.1-only value
  against an older server with a diagnostic naming the required version (`Gravitino >= 1.3.1`)
  instead of the server's opaque `400`. Restricted attributes say so in their schema description.
- Some resources need server-side configuration: `gravitino_role`, `gravitino_owner`,
  `gravitino_user` and `gravitino_group` require `gravitino.authorization.enable=true`;
  `gravitino_idp_user` and `gravitino_idp_group` require the built-in IDP plugin with the `basic`
  authenticator; jobs require a configured job executor; tables, views, functions, partitions and
  statistics require a lakehouse catalog (Hive or Iceberg).

### Gravitino 1.3.0 vs 1.3.1

| Capability | 1.3.0 | 1.3.1 |
|---|---|---|
| `gravitino_policy.supported_object_types` (`VIEW`, `FUNCTION`) | rejected at create/update | accepted |
| `gravitino_statistics` / `gravitino_credentials` `resource_type`, and `gravitino_tag_association` / `gravitino_policy_association` / `gravitino_object_tags` / `gravitino_object_policies` `object_type` (`VIEW`, and `FUNCTION` for policies) | rejected with an explicit diagnostic | accepted |
| `gravitino_table` index `index_type` (`data_skipping_minmax`, `data_skipping_bloom_filter`, `data_skipping_set`) and index `properties` | rejected at create/update | accepted |
| `gravitino_table` / `gravitino_view` column type `external` (`{"type":"external","catalogString":"..."}`) | rejected at create/update | accepted |
| `gravitino_principal.service_admin` | always `false` | reported |
| `gravitino_iceberg_rest_service` data source | endpoint does not exist | returns `uri`, or `null` when the service is unavailable |
| `gravitino_catalog_connection_test` existing-catalog variant | endpoint does not exist | supported |
| `gravitino_catalog_connection_test` proposed-configuration variant | supported | supported |

The `gravitino_secrets` data source needs Gravitino 1.4 or newer.

## Verifying a release

Releases are built from GPG-signed tags and ship checksums signed with the
[release signing key](https://github.com/caberdo/terraform-provider-gravitino/blob/main/.github/release-signing-key.asc)
(fingerprint `B6E21D8E7BE46C88492A7516BAD489AD0A93C1F6`), SLSA build provenance, an SPDX SBOM and
the Terraform registry manifest:

```bash
gpg --verify terraform-provider-gravitino_<version>_SHA256SUMS.sig \
             terraform-provider-gravitino_<version>_SHA256SUMS
gh attestation verify terraform-provider-gravitino_<version>_linux_amd64.zip \
  --repo caberdo/terraform-provider-gravitino
```

## Authentication

The provider supports five authentication methods: `none` (default), `simple`, `basic`, `oauth`, and `kerberos`. Set the `auth` attribute to choose a method.

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

## Community

- [Issues and feature requests](https://github.com/caberdo/terraform-provider-gravitino/issues)
- [Changelog](https://github.com/caberdo/terraform-provider-gravitino/blob/main/CHANGELOG.md)
- [Security policy](https://github.com/caberdo/terraform-provider-gravitino/blob/main/.github/SECURITY.md)
- [Contributing](https://github.com/caberdo/terraform-provider-gravitino/blob/main/.github/CONTRIBUTING.md)
- [Support](https://github.com/caberdo/terraform-provider-gravitino/blob/main/SUPPORT.md)

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `auth` (String) Authentication method: 'none', 'simple', 'basic', 'oauth', or 'kerberos'. 'simple' authenticates as the OS user (honouring the GRAVITINO_USER environment variable). Can also be set via GRAVITINO_AUTH environment variable.
- `kerberos_keytab` (String, Sensitive) Path to Kerberos keytab file. Can also be set via GRAVITINO_KERBEROS_KEYTAB environment variable.
- `kerberos_principal` (String) Kerberos principal (e.g. HTTP/server@REALM). Can also be set via GRAVITINO_KERBEROS_PRINCIPAL environment variable.
- `kerberos_use_ticket_cache` (Boolean) Use Kerberos ticket cache instead of keytab. Can also be set via GRAVITINO_KERBEROS_USE_TICKET_CACHE environment variable.
- `oauth_client_id` (String) OAuth2 client ID for client credentials flow. Can also be set via GRAVITINO_OAUTH_CLIENT_ID environment variable.
- `oauth_client_secret` (String, Sensitive) OAuth2 client secret for client credentials flow. Can also be set via GRAVITINO_OAUTH_CLIENT_SECRET environment variable.
- `oauth_scope` (String) OAuth2 scope for client credentials flow. Can also be set via GRAVITINO_OAUTH_SCOPE environment variable.
- `oauth_server_uri` (String) OAuth2 server URI for client credentials flow. Can also be set via GRAVITINO_OAUTH_SERVER_URI environment variable.
- `oauth_token` (String, Sensitive) OAuth2 bearer token. Can also be set via GRAVITINO_OAUTH_TOKEN environment variable.
- `oauth_token_path` (String) OAuth2 token endpoint path (e.g. /oauth2/token). Can also be set via GRAVITINO_OAUTH_TOKEN_PATH environment variable.
- `password` (String, Sensitive) Password for basic authentication. Can also be set via GRAVITINO_PASSWORD environment variable.
- `uri` (String) The URI of the Gravitino server. Can also be set via GRAVITINO_URI environment variable.
- `username` (String) Username for simple/basic authentication (simple falls back to GRAVITINO_USER). Can also be set via GRAVITINO_USERNAME environment variable.
