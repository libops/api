# Project Terraform Module
# Creates GCP project and configures base resources

terraform {
  required_version = ">= 1.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
  }
}

variable "public_id" {
  description = "Public ID (UUID) of the project"
  type        = string
}

variable "name" {
  description = "Name of the project"
  type        = string
}

variable "organization_id" {
  description = "Organization public ID"
  type        = string
}

variable "organization_folder_id" {
  description = "Parent organization folder ID"
  type        = string
}

variable "github_repository" {
  description = "GitHub repository URL"
  type        = string
  default     = null
}

variable "gcp_billing_account" {
  description = "GCP billing account ID"
  type        = string
}

variable "machine_type" {
  description = "Default machine type for sites"
  type        = string
}

variable "disk_size" {
  description = "Default disk size for sites"
  type        = number
}

# Create project
resource "google_project" "project" {
  name            = var.name
  project_id      = "libops-${substr(var.public_id, 0, 8)}"
  folder_id       = var.organization_folder_id
  billing_account = var.gcp_billing_account
}

# Enable APIs
resource "google_project_service" "services" {
  for_each = toset([
    "compute.googleapis.com",
    "container.googleapis.com",
  ])

  project = google_project.project.project_id
  service = each.value
}

output "project_id" {
  value = google_project.project.project_id
}

output "project_number" {
  value = google_project.project.number
}
