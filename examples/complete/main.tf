# Complete Gravitino Deployment Example
# This example shows all resources working together in a realistic data platform setup.
# Provider configuration should be added separately (see examples/provider/).

# ---------------------------------------------------------------------------
# METALAKE — Root namespace
# ---------------------------------------------------------------------------
resource "gravitino_metalake" "platform" {
  name    = "data_platform"
  comment = "Enterprise data platform"
  properties = {
    "environment" = "production"
    "region"      = "eu-west-1"
  }
}

# ---------------------------------------------------------------------------
# CATALOGS — Data sources
# ---------------------------------------------------------------------------
resource "gravitino_catalog" "hive" {
  metalake         = gravitino_metalake.platform.name
  name             = "warehouse"
  type             = "relational"
  catalog_provider = "hive"
  comment          = "Hive warehouse for structured data"
  properties = {
    "metastore.uris" = "thrift://hive-metastore:9083"
  }
}

resource "gravitino_catalog" "iceberg" {
  metalake         = gravitino_metalake.platform.name
  name             = "lakehouse"
  type             = "relational"
  catalog_provider = "lakehouse-iceberg"
  comment          = "Iceberg lakehouse for transactional data lake"
  properties = {
    "warehouse"       = "s3a://iceberg-warehouse"
    "catalog-backend" = "jdbc"
    "uri"             = "jdbc:postgresql://iceberg-metastore:5432/iceberg"
  }
}

resource "gravitino_catalog" "fileset_catalog" {
  metalake = gravitino_metalake.platform.name
  name     = "data_lake"
  type     = "fileset"
  comment  = "Fileset catalog for unstructured data"
}

resource "gravitino_catalog" "kafka" {
  metalake         = gravitino_metalake.platform.name
  name             = "streaming"
  type             = "messaging"
  catalog_provider = "kafka"
  comment          = "Kafka for event streaming"
  properties = {
    "bootstrap.servers" = "kafka-cluster:9092"
  }
}

resource "gravitino_catalog" "ml" {
  metalake = gravitino_metalake.platform.name
  name     = "machine_learning"
  type     = "model"
  comment  = "ML model registry"
}

# ---------------------------------------------------------------------------
# SCHEMAS — Logical grouping per domain
# ---------------------------------------------------------------------------
resource "gravitino_schema" "analytics" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  name     = "analytics"
  comment  = "Analytics and reporting schema"
}

resource "gravitino_schema" "ingestion" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  name     = "ingestion"
  comment  = "Raw data ingestion schema"
  properties = {
    owner = "data-engineering"
  }
}

# ---------------------------------------------------------------------------
# TABLES — With advanced features
# ---------------------------------------------------------------------------
resource "gravitino_table" "customers" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.analytics.name
  name     = "customers"
  comment  = "Customer master data"

  column {
    name     = "customer_id"
    type     = "string"
    nullable = false
  }
  column {
    name     = "name"
    type     = "string"
    nullable = false
  }
  column {
    name = "email"
    type = "string"
  }
  column {
    name = "country"
    type = "string"
  }
  column {
    name = "created_at"
    type = "timestamp"
  }
}

resource "gravitino_table" "orders" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.analytics.name
  name     = "orders"
  comment  = "Customer orders with partitioning and sort order"

  column {
    name     = "order_id"
    type     = "string"
    nullable = false
  }
  column {
    name     = "customer_id"
    type     = "string"
    nullable = false
  }
  column {
    name = "amount"
    type = "double"
  }
  column {
    name = "status"
    type = "string"
  }
  column {
    name = "order_date"
    type = "date"
  }

  partitioning {
    strategy   = "range"
    field_name = ["order_date"]
  }

  distribution {
    strategy = "hash"
    number   = 4
  }

  sort_order {
    field_name = ["order_date"]
    direction  = "desc"
  }
  sort_order {
    field_name = ["amount"]
    direction  = "desc"
  }
}

resource "gravitino_table" "events" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.ingestion.name
  name     = "raw_events"
  comment  = "Raw event data with bucket partitioning"

  column {
    name = "event_id"
    type = "string"
  }
  column {
    name = "event_type"
    type = "string"
  }
  column {
    name = "payload"
    type = "string"
  }
  column {
    name = "event_time"
    type = "timestamp"
  }

  partitioning {
    strategy    = "bucket"
    field_name  = ["event_id"]
    num_buckets = 24
  }
}

# ---------------------------------------------------------------------------
# PARTITIONS — Table partition management
# ---------------------------------------------------------------------------
resource "gravitino_partition" "q1" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.analytics.name
  table    = gravitino_table.orders.name
  type     = "identity"
  # field_names holds a path per field; values are typed literals.
  field_names = [["order_date"]]
  values = [
    {
      data_type = "date"
      value     = "2024-01-01"
    }
  ]
}

resource "gravitino_partition" "q2" {
  metalake    = gravitino_metalake.platform.name
  catalog     = gravitino_catalog.hive.name
  schema      = gravitino_schema.analytics.name
  table       = gravitino_table.orders.name
  type        = "identity"
  field_names = [["order_date"]]
  values = [
    {
      data_type = "date"
      value     = "2024-04-01"
    }
  ]
}

# ---------------------------------------------------------------------------
# VIEWS — Analytics views
# ---------------------------------------------------------------------------
resource "gravitino_view" "customer_orders" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.analytics.name
  name     = "customer_order_summary"
  comment  = "Aggregated customer order metrics"

  representation = [
    {
      type    = "sql"
      dialect = "spark"
      sql     = "SELECT customer_id, COUNT(*) AS orders FROM analytics.orders GROUP BY customer_id"
    }
  ]
}

# ---------------------------------------------------------------------------
# FUNCTIONS — User-defined functions
# ---------------------------------------------------------------------------
resource "gravitino_function" "parse_user_agent" {
  metalake      = gravitino_metalake.platform.name
  catalog       = gravitino_catalog.hive.name
  schema        = gravitino_schema.analytics.name
  name          = "parse_user_agent"
  function_type = "SCALAR"
  deterministic = true
  comment       = "Parse user agent strings into device, browser, OS"

  definitions = [
    {
      parameters = [
        { name = "user_agent", data_type = "string" }
      ]
      return_type = "string"

      impls = [
        {
          language   = "JAVA"
          runtime    = "SPARK"
          class_name = "com.example.udf.ParseUserAgent"
          resources = {
            jars = ["hdfs:///path/to/udf.jar"]
          }
        }
      ]
    }
  ]
}

resource "gravitino_function" "geocode" {
  metalake      = gravitino_metalake.platform.name
  catalog       = gravitino_catalog.hive.name
  schema        = gravitino_schema.analytics.name
  name          = "geocode_address"
  function_type = "SCALAR"
  deterministic = true
  comment       = "Convert address strings to lat/lon coordinates"

  definitions = [
    {
      parameters = [
        { name = "address", data_type = "string" }
      ]
      # Structured types use the JSON document form of the Gravitino data type.
      return_type = jsonencode({
        type = "struct"
        fields = [
          { name = "lat", type = "double" },
          { name = "lon", type = "double" }
        ]
      })

      impls = [
        { language = "SQL", runtime = "SPARK", sql = "geocode(address)" }
      ]
    }
  ]
}

# ---------------------------------------------------------------------------
# TOPICS — Event streaming
# ---------------------------------------------------------------------------
resource "gravitino_topic" "click_events" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.kafka.name
  schema   = gravitino_schema.analytics.name
  name     = "click_events"
  comment  = "User click stream events"
  properties = {
    "retention.ms" = "604800000"
    "partitions"   = "12"
  }
}

resource "gravitino_topic" "order_events" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.kafka.name
  schema   = gravitino_schema.ingestion.name
  name     = "order_events"
  comment  = "Order lifecycle events"
  properties = {
    "retention.ms"   = "2592000000"
    "partitions"     = "6"
    "cleanup.policy" = "compact"
  }
}

# ---------------------------------------------------------------------------
# FILESETS — Data lake storage
# ---------------------------------------------------------------------------
resource "gravitino_fileset" "raw_data" {
  metalake         = gravitino_metalake.platform.name
  catalog          = gravitino_catalog.fileset_catalog.name
  schema           = gravitino_schema.ingestion.name
  name             = "raw_logs"
  type             = "external"
  storage_location = "s3a://datalake/raw/logs"
  comment          = "Raw server logs"
}

resource "gravitino_fileset" "ml_datasets" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.fileset_catalog.name
  schema   = gravitino_schema.analytics.name
  name     = "training_data"
  type     = "managed"
  comment  = "ML training datasets"
  properties = {
    "format" = "parquet"
  }
}

# ---------------------------------------------------------------------------
# MODELS — ML model registry
# ---------------------------------------------------------------------------
resource "gravitino_model" "fraud_detector" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.ml.name
  schema   = gravitino_schema.analytics.name
  name     = "fraud_detection"
  comment  = "ML model for fraud detection"
}

resource "gravitino_model_version" "fraud_v1" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.ml.name
  schema   = gravitino_schema.analytics.name
  model    = gravitino_model.fraud_detector.name
  uri      = "s3://models/fraud_detection/1.0.0"
  aliases  = ["production"]
  comment  = "Initial production release"
  properties = {
    "accuracy"  = "0.97"
    "framework" = "pytorch"
  }
}

resource "gravitino_model_version" "fraud_v2" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.ml.name
  schema   = gravitino_schema.analytics.name
  model    = gravitino_model.fraud_detector.name
  uri      = "s3://models/fraud_detection/2.0.0"
  aliases  = ["staging"]
  comment  = "Improved model with XGBoost"
  properties = {
    "accuracy"  = "0.985"
    "framework" = "xgboost"
  }
}

resource "gravitino_model" "recommendation" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.ml.name
  schema   = gravitino_schema.analytics.name
  name     = "product_recommendations"
  comment  = "Product recommendation engine"
}

# ---------------------------------------------------------------------------
# TAGS — Data classification
# ---------------------------------------------------------------------------
resource "gravitino_tag" "pii" {
  metalake = gravitino_metalake.platform.name
  name     = "PII"
  comment  = "Personally Identifiable Information"
  properties = {
    "classification" = "restricted"
    "retention"      = "7y"
  }
}

resource "gravitino_tag" "sensitive" {
  metalake = gravitino_metalake.platform.name
  name     = "SENSITIVE"
  comment  = "Sensitive business data"
  properties = {
    "classification" = "confidential"
  }
}

resource "gravitino_tag" "public" {
  metalake = gravitino_metalake.platform.name
  name     = "PUBLIC"
  comment  = "Public data, no restrictions"
}

# ---------------------------------------------------------------------------
# POLICIES — Access control rules
# ---------------------------------------------------------------------------
resource "gravitino_policy" "analytics_read" {
  metalake    = gravitino_metalake.platform.name
  name        = "analytics_readonly"
  comment     = "Read-only access to the analytics schema"
  policy_type = "custom"
  enabled     = true

  supported_object_types = ["SCHEMA"]
  custom_rules = {
    "effect"   = "allow"
    "actions"  = "read"
    "subjects" = "analytics-team,reporting-users"
  }
}

resource "gravitino_policy" "ingestion_write" {
  metalake    = gravitino_metalake.platform.name
  name        = "ingestion_writer"
  comment     = "Write access for the ingestion team"
  policy_type = "custom"
  enabled     = true

  supported_object_types = ["SCHEMA"]
  custom_rules = {
    "effect"   = "allow"
    "actions"  = "write,read"
    "subjects" = "data-engineering"
  }
}

resource "gravitino_policy" "pii_restrict" {
  metalake    = gravitino_metalake.platform.name
  name        = "pii_restriction"
  comment     = "Deny guest access to customer PII"
  policy_type = "custom"
  enabled     = true

  supported_object_types = ["TABLE"]
  custom_rules = {
    "effect"    = "deny"
    "actions"   = "read"
    "subjects"  = "guest-user,reporting-users"
    "condition" = "context.role != 'compliance_officer'"
  }
}

# ---------------------------------------------------------------------------
# USERS — Platform users
# ---------------------------------------------------------------------------
resource "gravitino_user" "alice" {
  metalake = gravitino_metalake.platform.name
  name     = "alice"
  roles    = ["admin"]
}

resource "gravitino_user" "bob" {
  metalake = gravitino_metalake.platform.name
  name     = "bob"
  roles    = ["data_engineer"]
}

resource "gravitino_user" "carol" {
  metalake = gravitino_metalake.platform.name
  name     = "carol"
  roles    = ["analyst"]
}

# ---------------------------------------------------------------------------
# GROUPS — User groups
# ---------------------------------------------------------------------------
resource "gravitino_group" "engineering" {
  metalake = gravitino_metalake.platform.name
  name     = "data_engineering"
  roles    = ["data_engineer"]
}

resource "gravitino_group" "analytics_team" {
  metalake = gravitino_metalake.platform.name
  name     = "analytics_team"
  roles    = ["analyst"]
}

# ---------------------------------------------------------------------------
# ROLES — Role definitions with privileges
# ---------------------------------------------------------------------------
resource "gravitino_role" "data_engineer" {
  metalake = gravitino_metalake.platform.name
  name     = "data_engineer"
  securable_objects = [
    {
      full_name = gravitino_schema.ingestion.name
      type      = "SCHEMA"
      privileges = [
        { name = "USE_SCHEMA", condition = "ALLOW" },
        { name = "CREATE_TABLE", condition = "ALLOW" },
      ]
    },
    {
      full_name = gravitino_catalog.hive.name
      type      = "CATALOG"
      privileges = [
        { name = "USE_CATALOG", condition = "ALLOW" },
      ]
    },
  ]
}

resource "gravitino_role" "analyst" {
  metalake = gravitino_metalake.platform.name
  name     = "analyst"
  securable_objects = [
    {
      full_name = gravitino_schema.analytics.name
      type      = "SCHEMA"
      privileges = [
        { name = "USE_SCHEMA", condition = "ALLOW" },
        { name = "SELECT_TABLE", condition = "ALLOW" },
      ]
    },
  ]
}

# ---------------------------------------------------------------------------
# OWNERS — Resource ownership
# ---------------------------------------------------------------------------
resource "gravitino_owner" "catalog_owner" {
  metalake         = gravitino_metalake.platform.name
  object_type      = "CATALOG"
  object_full_name = gravitino_catalog.hive.name
  owner_name       = gravitino_user.alice.name
  owner_type       = "USER"
}

resource "gravitino_owner" "schema_owner" {
  metalake         = gravitino_metalake.platform.name
  object_type      = "SCHEMA"
  object_full_name = "${gravitino_catalog.hive.name}.${gravitino_schema.analytics.name}"
  owner_name       = gravitino_group.engineering.name
  owner_type       = "GROUP"
}

# ---------------------------------------------------------------------------
# JOBS — Scheduled tasks
# ---------------------------------------------------------------------------
resource "gravitino_job_template" "spark_etl" {
  metalake   = gravitino_metalake.platform.name
  name       = "spark_etl_runner"
  job_type   = "spark"
  comment    = "Generic Spark ETL job template"
  executable = "/opt/spark/bin/spark-submit"
  arguments  = ["--env", "production"]
  class_name = "com.platform.etl.Orchestrator"
  jars       = ["s3://jars/etl-framework.jar"]
  configs = {
    "spark.executor.memory" = "2g"
  }
}

resource "gravitino_job" "daily_orders" {
  metalake     = gravitino_metalake.platform.name
  job_template = gravitino_job_template.spark_etl.name
  job_conf = {
    environment = "production"
  }
}

resource "gravitino_job" "hourly_events" {
  metalake     = gravitino_metalake.platform.name
  job_template = gravitino_job_template.spark_etl.name
  job_conf = {
    environment = "production"
  }
}

# ---------------------------------------------------------------------------
# DATA SOURCES — Query existing resources
# ---------------------------------------------------------------------------
data "gravitino_metalakes" "all" {
  depends_on = [gravitino_metalake.platform]
}

data "gravitino_catalogs" "hive_catalogs" {
  metalake   = gravitino_metalake.platform.name
  depends_on = [gravitino_catalog.hive]
}

data "gravitino_tables" "analytics_tables" {
  metalake   = gravitino_metalake.platform.name
  catalog    = gravitino_catalog.hive.name
  schema     = gravitino_schema.analytics.name
  depends_on = [gravitino_table.customers, gravitino_table.orders]
}

data "gravitino_tags" "all_tags" {
  metalake   = gravitino_metalake.platform.name
  depends_on = [gravitino_tag.pii, gravitino_tag.sensitive, gravitino_tag.public]
}
