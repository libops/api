output "service_url" {
  description = "URL of the Cloud Run service"
  value       = google_cloud_run_service.site_proxy.status[0].url
}

output "service_name" {
  description = "Name of the Cloud Run service"
  value       = google_cloud_run_service.site_proxy.name
}

output "service_account_email" {
  description = "Email of the service account"
  value       = google_service_account.site_proxy.email
}

output "vpc_connector_id" {
  description = "ID of the VPC Access Connector (if created)"
  value       = var.create_vpc_connector ? google_vpc_access_connector.site_proxy[0].id : null
}
