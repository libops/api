variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region for Cloud Run service"
  type        = string
  default     = "us-central1"
}

variable "service_name" {
  description = "Name of the Cloud Run service"
  type        = string
  default     = "site-proxy"
}

variable "service_account_id" {
  description = "Service account ID for Cloud Run"
  type        = string
  default     = "site-proxy-sa"
}

variable "container_image" {
  description = "Container image for Site Proxy service (e.g., gcr.io/project/site-proxy:latest)"
  type        = string
}

variable "api_url" {
  description = "Base URL for the control plane API"
  type        = string
}

variable "cpu_limit" {
  description = "CPU limit for each container instance"
  type        = string
  default     = "1000m"
}

variable "memory_limit" {
  description = "Memory limit for each container instance"
  type        = string
  default     = "512Mi"
}

variable "container_concurrency" {
  description = "Maximum number of concurrent requests per container"
  type        = number
  default     = 80
}

variable "timeout_seconds" {
  description = "Request timeout in seconds"
  type        = number
  default     = 60
}

variable "min_instances" {
  description = "Minimum number of instances"
  type        = number
  default     = 1
}

variable "max_instances" {
  description = "Maximum number of instances"
  type        = number
  default     = 10
}

variable "ingress_type" {
  description = "Ingress type (all, internal, internal-and-cloud-load-balancing)"
  type        = string
  default     = "all"
}

variable "allow_unauthenticated" {
  description = "Allow unauthenticated invocations (required for Pub/Sub push without OIDC)"
  type        = bool
  default     = false
}

variable "create_vpc_connector" {
  description = "Create VPC Access Connector for accessing sites in private VPC"
  type        = bool
  default     = true
}

variable "vpc_connector_name" {
  description = "Name of the VPC Access Connector"
  type        = string
  default     = "site-proxy-connector"
}

variable "vpc_connector_subnet" {
  description = "Subnet for VPC Access Connector"
  type        = string
  default     = ""
}

variable "vpc_connector_machine_type" {
  description = "Machine type for VPC connector instances"
  type        = string
  default     = "e2-micro"
}

variable "vpc_connector_min_instances" {
  description = "Minimum instances for VPC connector"
  type        = number
  default     = 2
}

variable "vpc_connector_max_instances" {
  description = "Maximum instances for VPC connector"
  type        = number
  default     = 10
}

variable "vpc_egress" {
  description = "VPC egress setting (private-ranges-only or all-traffic)"
  type        = string
  default     = "private-ranges-only"
}
