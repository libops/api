# Organization Terraform Module
# Creates GCP folder and project for organization management

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
  description = "Public ID (UUID) of the organization"
  type        = string
}

variable "name" {
  description = "Name of the organization"
  type        = string
}

variable "gcp_org_id" {
  description = "GCP organization ID"
  type        = string
}

variable "gcp_billing_account" {
  description = "GCP billing account ID"
  type        = string
}

variable "gcp_parent" {
  description = "Parent resource (folders/XXX or organizations/XXX)"
  type        = string
}

variable "location" {
  description = "Default location for resources"
  type        = string
  default     = "us-central1"
}

variable "orchestrator_psc_ip" {
  description = "IP address of the Orchestrator PSC endpoint"
  type        = string
}

# Create folder for organization
resource "google_folder" "org_folder" {
  display_name = var.name
  parent       = var.gcp_parent
}

# Create project for organization management
# Use public_id to ensure unique project IDs
resource "google_project" "org_project" {
  name            = "${var.name}-mgmt"
  project_id      = "libops-${substr(var.public_id, 0, 8)}-mgmt"
  folder_id       = google_folder.org_folder.name
  billing_account = var.gcp_billing_account
}

# Enable required APIs
resource "google_project_service" "services" {
  for_each = toset([
    "compute.googleapis.com",
    "container.googleapis.com",
    "cloudbilling.googleapis.com",
    "cloudresourcemanager.googleapis.com",
    "iam.googleapis.com",
  ])

  project = google_project.org_project.project_id
  service = each.value

  disable_on_destroy = false
}

# Network
resource "google_compute_network" "org_network" {
  project                 = google_project.org_project.project_id
  name                    = "org-network"
  auto_create_subnetworks = false
  depends_on              = [google_project_service.services]
}

resource "google_compute_subnetwork" "org_subnet" {
  project       = google_project.org_project.project_id
  name          = "org-subnet"
  ip_cidr_range = "10.128.0.0/20"
  region        = var.location
  network       = google_compute_network.org_network.id
}

# Cloud NAT
resource "google_compute_router" "router" {
  project = google_project.org_project.project_id
  name    = "org-router"
  region  = var.location
  network = google_compute_network.org_network.id
}

resource "google_compute_router_nat" "nat" {
  project                            = google_project.org_project.project_id
  name                               = "org-nat"
  router                             = google_compute_router.router.name
  region                             = var.location
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
}

# Service Account
resource "google_service_account" "org_vm" {
  project      = google_project.org_project.project_id
  account_id   = "org-vm"
  display_name = "Customer Org VM"
}

# IAM
resource "google_project_iam_member" "vm_owner" {
  project = google_project.org_project.project_id
  role    = "roles/owner"
  member  = "serviceAccount:${google_service_account.org_vm.email}"
}

# GCS Bucket for State
resource "google_storage_bucket" "terraform_state" {
  project       = google_project.org_project.project_id
  name          = "libops-org-${substr(var.public_id, 0, 8)}-tfstate"
  location      = var.location
  force_destroy = false
  versioning {
    enabled = true
  }
}

# VM
resource "google_compute_instance" "org_vm" {
  project      = google_project.org_project.project_id
  name         = "org-vm"
  machine_type = "e2-medium"
  zone         = "${var.location}-a"

  boot_disk {
    initialize_params {
      image = "cos-cloud/cos-stable"
      size  = 50
    }
  }

  network_interface {
    network    = google_compute_network.org_network.id
    subnetwork = google_compute_subnetwork.org_subnet.id
  }

  service_account {
    email  = google_service_account.org_vm.email
    scopes = ["cloud-platform"]
  }

  metadata = {
    startup-script = templatefile("${path.module}/startup.sh", {
      orchestrator_psc_ip = var.orchestrator_psc_ip
      organization_id     = var.public_id
      gcs_bucket          = google_storage_bucket.terraform_state.name
    })
  }
  
  tags = ["customer-org-vm"]
  
  depends_on = [google_compute_router_nat.nat]
}

output "folder_id" {
  description = "The folder ID"
  value       = google_folder.org_folder.name
}

output "project_id" {
  description = "The project ID"
  value       = google_project.org_project.project_id
}

output "project_number" {
  description = "The project number"
  value       = google_project.org_project.number
}