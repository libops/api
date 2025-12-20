variable "project_id" {
  description = "GCP project ID where Pub/Sub resources will be created"
  type        = string
}

variable "topic_name" {
  description = "Name of the Pub/Sub topic for site events"
  type        = string
  default     = "site-events"
}

variable "subscription_name" {
  description = "Name of the push subscription to Site Proxy service"
  type        = string
  default     = "site-proxy-push-subscription"
}

variable "proxy_push_endpoint" {
  description = "HTTPS endpoint of the Site Proxy Cloud Run service for push delivery"
  type        = string
}

variable "proxy_service_account" {
  description = "Service account email for Site Proxy Cloud Run service (for OIDC auth)"
  type        = string
  default     = ""
}

variable "enable_dead_letter_queue" {
  description = "Enable dead letter queue for failed message delivery"
  type        = bool
  default     = true
}

variable "enable_monitoring" {
  description = "Enable monitoring alerts for Pub/Sub"
  type        = bool
  default     = true
}

variable "notification_channels" {
  description = "List of notification channel IDs for alerts"
  type        = list(string)
  default     = []
}
