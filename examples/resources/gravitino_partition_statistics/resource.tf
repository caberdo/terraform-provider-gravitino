# Partition statistics are keyed by partition name, then by statistic name.
# Like gravitino_statistics, the resource owns every custom-* entry it manages.
resource "gravitino_partition_statistics" "orders" {
  metalake = gravitino_metalake.example.name
  catalog  = gravitino_catalog.hive.name
  schema   = gravitino_schema.example.name
  table    = gravitino_table.partitioned.name

  partitions = {
    "dt=2026-10-01" = {
      "custom-row-count" = "500"
    }
    "dt=2026-10-02" = {
      "custom-row-count" = "750"
      "custom-size"      = "1024"
    }
  }
}
