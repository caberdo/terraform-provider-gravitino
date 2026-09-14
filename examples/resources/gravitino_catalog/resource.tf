resource "gravitino_catalog" "hive" {
  metalake         = gravitino_metalake.example.name
  name             = "hive_catalog"
  type             = "relational"
  catalog_provider = "hive"
  comment          = "Hive catalog for data warehouse"
  properties = {
    "metastore.uris" = "thrift://localhost:9083"
  }
}

resource "gravitino_catalog" "fileset" {
  metalake = gravitino_metalake.example.name
  name     = "fileset_catalog"
  type     = "fileset"
  comment  = "Fileset catalog for data lake"
}

resource "gravitino_catalog" "lakehouse_iceberg" {
  metalake         = gravitino_metalake.example.name
  name             = "lakehouse_iceberg_catalog"
  type             = "relational"
  catalog_provider = "lakehouse-iceberg"
  comment          = "Lakehouse Iceberg catalog for transactional data lake"
  properties = {
    "warehouse" = "s3a://iceberg-warehouse"
    "catalog-backend" = "jdbc"
    "uri" = "jdbc:postgresql://localhost:5432/iceberg"
  }
}
