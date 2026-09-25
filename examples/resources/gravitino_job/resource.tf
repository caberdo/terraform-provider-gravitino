resource "gravitino_job" "daily_etl" {
  metalake     = gravitino_metalake.example.name
  job_template = gravitino_job_template.daily_etl.name

  job_conf = {
    "date" = "2024-01-01"
  }
}

output "daily_etl_job_id" {
  description = "The server-generated identifier of the job run."
  value       = gravitino_job.daily_etl.job_id
}

output "daily_etl_status" {
  description = "The status of the job run."
  value       = gravitino_job.daily_etl.status
}
