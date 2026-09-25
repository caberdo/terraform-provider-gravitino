# Terraform Provider for Apache Gravitino

A Terraform provider for managing [Apache Gravitino](https://gravitino.apache.org) resources — the unified metadata lakehouse service.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.26.4

## Using the Provider

### Provider Configuration

```hcl
terraform {
  required_providers {
    gravitino = {
      source = "gravitino/gravitino"
    }
  }
}

provider "gravitino" {
  uri      = "http://localhost:8090"
  auth     = "basic"
  username = "admin"
  password = "admin"
}
```

Or configure via environment variables:

```sh
export GRAVITINO_URI="http://localhost:8090"
export GRAVITINO_AUTH="oauth"
export GRAVITINO_OAUTH_TOKEN="eyJ..."
```

The provider supports the following arguments:

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

### Creating a Metalake

```hcl
resource "gravitino_metalake" "example" {
  name    = "my_metalake"
  comment = "My first metalake"
  properties = {
    env = "production"
  }
}
```

### Creating a Catalog

```hcl
resource "gravitino_catalog" "hive" {
  metalake = gravitino_metalake.example.name
  name     = "my_hive_catalog"
  type             = "relational"
  catalog_provider = "hive"
  properties = {
    "metastore.uris" = "thrift://localhost:9083"
  }
}
```

### Creating a Schema and Table

```hcl
resource "gravitino_schema" "example" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.hive.name
  name     = "my_schema"
}

resource "gravitino_table" "example" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.example.name
  name     = "my_table"

  column {
    name = "id"
    type = "integer"
  }
  column {
    name    = "name"
    type    = "varchar"
    length  = 255
  }
}
```

### Data Sources

```hcl
data "gravitino_metalakes" "all" {}

data "gravitino_metalake" "example" {
  name = "my_metalake"
}
```

## Resources

| Resource                 | Description                                      |
|--------------------------|--------------------------------------------------|
| `gravitino_metalake`     | Manage a Gravitino metalake.                     |
| `gravitino_catalog`      | Manage a catalog within a metalake.              |
| `gravitino_schema`       | Manage a schema within a catalog.                |
| `gravitino_table`        | Manage a table within a schema.                  |
| `gravitino_fileset`      | Manage a fileset.                                |
| `gravitino_topic`        | Manage a messaging topic.                        |
| `gravitino_view`         | Manage a view.                                   |
| `gravitino_function`     | Manage a function.                               |
| `gravitino_model`        | Manage a model.                                  |
| `gravitino_model_version`| Manage a model version.                          |
| `gravitino_partition`    | Manage a table partition.                        |
| `gravitino_tag`          | Manage a tag.                                    |
| `gravitino_policy`       | Manage an access control policy.                 |
| `gravitino_role`         | Manage a role and its privileges.                |
| `gravitino_user`         | Manage a user.                                   |
| `gravitino_group`        | Manage a group.                                  |
| `gravitino_owner`        | Manage the owner of a metadata object.           |
| `gravitino_job`          | Run a job template.                              |
| `gravitino_job_template` | Manage a job template.                           |
| `gravitino_idp_user`     | Manage an identity provider user.                |
| `gravitino_idp_group`    | Manage an identity provider group.               |

## Data Sources

### Metalakes

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_metalakes`     | List all metalakes.            |
| `gravitino_metalake`      | Get a specific metalake.       |

### Catalogs, schemas, tables, filesets, topics, views

| Data Source                 | Description                                     |
|-----------------------------|-------------------------------------------------|
| `gravitino_catalogs`        | List catalogs of a metalake.                     |
| `gravitino_catalog`         | Get a specific catalog.                          |
| `gravitino_schemas`         | List schemas of a catalog.                       |
| `gravitino_schema`          | Get a specific schema.                           |
| `gravitino_tables`          | List tables of a schema.                         |
| `gravitino_table`           | Get a specific table.                            |
| `gravitino_partitions`      | List partitions of a table.                      |
| `gravitino_partition`       | Get a specific partition.                        |
| `gravitino_filesets`        | List filesets of a schema.                       |
| `gravitino_fileset`         | Get a specific fileset.                          |
| `gravitino_topics`          | List topics of a schema.                         |
| `gravitino_topic`           | Get a specific topic.                            |
| `gravitino_views`           | List views of a schema.                          |
| `gravitino_view`            | Get a specific view.                             |
| `gravitino_functions`       | List functions of a schema.                      |
| `gravitino_function`        | Get a specific function.                         |
| `gravitino_models`          | List models of a schema.                         |
| `gravitino_model`           | Get a specific model.                            |
| `gravitino_model_versions`  | List versions of a model.                        |
| `gravitino_model_version`   | Get a specific model version.                    |

### Tags, policies, roles, statistics, credentials

| Data Source                     | Description                                        |
|---------------------------------|----------------------------------------------------|
| `gravitino_tags`                | List tags of a metalake.                            |
| `gravitino_tag`                 | Get a specific tag.                                 |
| `gravitino_policies`            | List policies of a metalake.                        |
| `gravitino_roles`               | List roles of a metadata object.                    |
| `gravitino_roles_list`          | List roles of a metalake.                           |
| `gravitino_role`                | Get a specific role.                                |
| `gravitino_owner`               | Get the owner of a metadata object.                 |
| `gravitino_credentials`         | Get the credentials of a metadata object.           |
| `gravitino_secrets`             | Get the resolved secrets of a metadata object (Gravitino 1.4+). |
| `gravitino_statistics`          | Get the statistics of a metadata object.            |
| `gravitino_partition_statistics`| Get the partition statistics of a metadata object.  |

### Users, groups, jobs and health

| Data Source                 | Description                                     |
|-----------------------------|-------------------------------------------------|
| `gravitino_users`           | List users of a metalake.                        |
| `gravitino_user`            | Get a specific user.                             |
| `gravitino_groups`          | List groups of a metalake.                       |
| `gravitino_group`           | Get a specific group.                            |
| `gravitino_idp_user`        | Get an identity provider user.                   |
| `gravitino_idp_group`       | Get an identity provider group.                  |
| `gravitino_jobs`            | List job runs of a metalake.                     |
| `gravitino_job`             | Get a specific job run.                          |
| `gravitino_job_templates`   | List job templates of a metalake.                |
| `gravitino_job_template`    | Get a specific job template.                     |
| `gravitino_principal`       | Get the authenticated principal.                 |
| `gravitino_health`          | Get the aggregate server health.                 |
| `gravitino_liveness`        | Get the server liveness.                         |
| `gravitino_readiness`       | Get the server readiness.                        |

## Testing

Unit and mock-based tests:

```sh
make test
```

### Live acceptance tests against a real Gravitino (podman)

The repository contains `TestLiveAcc*` acceptance tests that run against a **real**
Gravitino server (image `apache/gravitino:1.3.0`) and prove the responses are not mocks:
`acceptance.LivePreCheck` refuses to run unless `GRAVITINO_URI` answers `GET /api/version`
with a matching Gravitino version.

Run them with podman (needs a running `podman machine` and `podman-compose`):

```bash
make testacc-live                # all live acceptance tests
make testacc-live-filter F=TestLiveAccMetalakeResource  # one test
```

Unlike the mock-based `TestAcc*` tests (which spin up an `httptest` server), these tests
target `http://gravitino:8090` inside the podman-compose network.

## Building the Provider

```sh
git clone https://github.com/gravitino/terraform-provider-gravitino
cd terraform-provider-gravitino
make build
make test
```

To install the provider locally:

```sh
make install
```

## Publishing

To generate Terraform documentation:

```sh
make generate
```

## License

Apache 2.0 — see the [LICENSE](LICENSE) file.
