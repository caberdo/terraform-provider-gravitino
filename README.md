# Terraform Provider for Apache Gravitino

A Terraform provider for managing [Apache Gravitino](https://gravitino.apache.org) resources — the unified metadata lakehouse service.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.22

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
| `auth` | `string` | `GRAVITINO_AUTH` | Auth method: `simple`, `basic`, `oauth`, or `kerberos` |
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
  type     = "relational"
  provider = "hive"
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

| Resource                | Description                                      |
|-------------------------|--------------------------------------------------|
| `gravitino_metalake`    | Manage a Gravitino metalake.                     |
| `gravitino_catalog`     | Manage a catalog within a metalake.              |
| `gravitino_schema`      | Manage a schema within a catalog.                |
| `gravitino_table`       | Manage a table within a schema.                  |
| `gravitino_tag`         | Manage a tag.                                    |
| `gravitino_fileset`     | Manage a fileset.                                |
| `gravitino_topic`       | Manage a messaging topic.                        |
| `gravitino_view`        | Manage a view.                                   |
| `gravitino_function`    | Manage a function.                               |
| `gravitino_model`       | Manage a model.                                  |
| `gravitino_partition`   | Manage a table partition.                        |
| `gravitino_policy`      | Manage an access control policy.                 |
| `gravitino_job`         | Manage a job.                                    |

## Data Sources

### Metalakes

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_metalakes`     | List all metalakes.            |
| `gravitino_metalake`      | Get a specific metalake.       |

### Catalogs

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_catalogs`      | List all catalogs.             |
| `gravitino_catalog`       | Get a specific catalog.        |

### Schemas

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_schemas`       | List all schemas.              |
| `gravitino_schema`        | Get a specific schema.         |

### Tables

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_tables`        | List all tables.               |
| `gravitino_table`         | Get a specific table.          |

### Tags

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_tags`          | List all tags.                 |
| `gravitino_tag`           | Get a specific tag.            |

### Filesets

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_filesets`      | List all filesets.             |
| `gravitino_fileset`       | Get a specific fileset.        |

### Topics

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_topics`        | List all topics.               |
| `gravitino_topic`         | Get a specific topic.          |

### Views

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_views`         | List all views.                |
| `gravitino_view`          | Get a specific view.           |

### Functions

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_functions`     | List all functions.            |
| `gravitino_function`      | Get a specific function.       |

### Models

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_models`        | List all models.               |
| `gravitino_model`         | Get a specific model.          |

### Partitions

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_partitions`    | List all partitions.           |
| `gravitino_partition`     | Get a specific partition.      |

### Policies

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_policies`      | List all access control policies. |

### Jobs

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_jobs`          | List all jobs.                 |
| `gravitino_job`           | Get a specific job.            |

### Health

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_health`        | Get server health status.      |
| `gravitino_liveness`      | Get server liveness status.    |
| `gravitino_readiness`     | Get server readiness status.   |

### Credentials

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_credentials`   | Get credential information.    |

### Roles

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_roles`         | List all roles.                |

### Statistics

| Data Source                        | Description                      |
|------------------------------------|----------------------------------|
| `gravitino_statistics`             | Get table statistics.            |
| `gravitino_partition_statistics`   | Get partition-level statistics.  |

### Principal

| Data Source               | Description                    |
|---------------------------|--------------------------------|
| `gravitino_principal`     | Get the current principal.     |

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
