resource "gravitino_job_template" "daily_etl" {
  metalake   = gravitino_metalake.example.name
  name       = "daily_etl"
  job_type   = "spark"
  comment    = "Daily ETL job template"
  executable = "/opt/spark/bin/spark-submit"

  arguments  = ["--date", "2024-01-01"]
  class_name = "com.example.ETLJob"
  jars       = ["s3://jars/etl-job.jar"]
  configs = {
    "spark.executor.memory" = "2g"
  }
}

resource "gravitino_job_template" "shell_cleanup" {
  metalake   = gravitino_metalake.example.name
  name       = "shell_cleanup"
  job_type   = "shell"
  comment    = "Shell job template"
  executable = "/opt/jobs/cleanup.sh"

  arguments = ["--dry-run"]
  scripts   = ["/opt/jobs/common.sh"]
  environments = {
    "LOG_LEVEL" = "info"
  }
}
