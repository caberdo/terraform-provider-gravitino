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
    "warehouse"      = "s3a://iceberg-warehouse"
    "catalog-backend" = "jdbc"
    "uri"            = "jdbc:postgresql://iceberg-metastore:5432/iceberg"
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
  columns = [
    { "name" = "customer_id", "type" = "string", "nullable" = false },
    { "name" = "name",        "type" = "string", "nullable" = false },
    { "name" = "email",       "type" = "string" },
    { "name" = "country",     "type" = "string" },
    { "name" = "created_at",  "type" = "timestamp" },
  ]
}

resource "gravitino_table" "orders" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.analytics.name
  name     = "orders"
  comment  = "Customer orders with partitioning and sort order"
  columns = [
    { "name" = "order_id",    "type" = "string", "nullable" = false },
    { "name" = "customer_id", "type" = "string", "nullable" = false },
    { "name" = "amount",      "type" = "double" },
    { "name" = "status",      "type" = "string" },
    { "name" = "order_date",  "type" = "date" },
  ]
  partition_strategy = "RANGE"
  partition_buckets  = 12
  distribution = {
    strategy = "HASH"
    buckets  = 4
    partition_keys = [
      { "name" = "customer_id", "type" = "string" },
    ]
  }
  sort_orders = [
    { "name" = "order_date", "direction" = "DESC" },
    { "name" = "amount",     "direction" = "DESC" },
  ]
}

resource "gravitino_table" "events" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.ingestion.name
  name     = "raw_events"
  comment  = "Raw event data with hash partitioning"
  columns = [
    { "name" = "event_id",   "type" = "string" },
    { "name" = "event_type", "type" = "string" },
    { "name" = "payload",    "type" = "string" },
    { "name" = "event_time", "type" = "timestamp" },
  ]
  partition_strategy = "HASH"
  partition_buckets  = 24
}

# ---------------------------------------------------------------------------
# PARTITIONS — Table partition management
# ---------------------------------------------------------------------------
resource "gravitino_partition" "q1" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.analytics.name
  table    = gravitino_table.orders.name
  name     = "2024_q1"
}

resource "gravitino_partition" "q2" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.analytics.name
  table    = gravitino_table.orders.name
  name     = "2024_q2"
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
}

# ---------------------------------------------------------------------------
# FUNCTIONS — User-defined functions
# ---------------------------------------------------------------------------
resource "gravitino_function" "parse_user_agent" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.analytics.name
  name     = "parse_user_agent"
  comment  = "Parse user agent strings into device, browser, OS"
}

resource "gravitino_function" "geocode" {
  metalake = gravitino_metalake.platform.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.analytics.name
  name     = "geocode_address"
  comment  = "Convert address strings to lat/lon coordinates"
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
    "retention.ms" = "2592000000"
    "partitions"   = "6"
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
  version  = "1.0.0"
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
  version  = "2.0.0"
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
  metalake   = gravitino_metalake.platform.name
  name       = "PII"
  comment    = "Personally Identifiable Information"
  properties = {
    "classification" = "restricted"
    "retention"      = "7y"
  }
}

resource "gravitino_tag" "sensitive" {
  metalake   = gravitino_metalake.platform.name
  name       = "SENSITIVE"
  comment    = "Sensitive business data"
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
  metalake      = gravitino_metalake.platform.name
  resource_type = "SCHEMAS"
  resource      = gravitino_schema.analytics.name
  name          = "analytics_readonly"
  effect        = "allow"
  actions       = ["read"]
  subjects      = ["analytics-team", "reporting-users"]
}

resource "gravitino_policy" "ingestion_write" {
  metalake      = gravitino_metalake.platform.name
  resource_type = "SCHEMAS"
  resource      = gravitino_schema.ingestion.name
  name          = "ingestion_writer"
  effect        = "allow"
  actions       = ["write", "read"]
  subjects      = ["data-engineering"]
}

resource "gravitino_policy" "pii_restrict" {
  metalake      = gravitino_metalake.platform.name
  resource_type = "TABLES"
  resource      = gravitino_table.customers.name
  name          = "pii_restriction"
  effect        = "deny"
  actions       = ["read"]
  subjects      = ["guest-user", "reporting-users"]
  condition     = "context.role != 'compliance_officer'"
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
        { name = "USE_SCHEMA",   condition = "ALLOW" },
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
        { name = "USE_SCHEMA",   condition = "ALLOW" },
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
  metalake = gravitino_metalake.platform.name
  name     = "spark_etl_runner"
  template = "spark"
  parameters = {
    "main_class" = "com.platform.etl.Orchestrator"
    "jar"        = "s3://jars/etl-framework.jar"
    "args"       = "--env=${environment} --date=${date}"
  }
  comment = "Generic Spark ETL job template"
}

resource "gravitino_job" "daily_orders" {
  metalake   = gravitino_metalake.platform.name
  name       = "daily_orders_etl"
  template   = gravitino_job_template.spark_etl.name
  schedule   = "0 2 * * *"
  parameters = {
    "environment" = "production"
  }
  comment = "Daily orders ETL pipeline"
}

resource "gravitino_job" "hourly_events" {
  metalake   = gravitino_metalake.platform.name
  name       = "hourly_event_processing"
  template   = gravitino_job_template.spark_etl.name
  schedule   = "0 * * * *"
  parameters = {
    "environment" = "production"
  }
  comment = "Hourly raw event processing"
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
