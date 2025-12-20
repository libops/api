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
  description = "Public ID (UUID) of the site"
  type        = string
}

variable "name" {
  description = "Name of the site"
  type        = string
}

variable "project_id" {
  description = "Project public ID (LibOps)"
  type        = string
}

variable "gcp_project_id" {
  description = "GCP project ID"
  type        = string
}
variable "gcp_project_number" {
  description = "GCP project number"
  type        = string
}

variable "github_ref" {
  description = "GitHub branch/ref to deploy"
  type        = string
}

variable "github_repo" {
  description = "GitHub repository URL"
  type        = string
}

variable "machine_type" {
  description = "Machine type for compute instance"
  type        = string
  default     = "e2-medium"
}

variable "disk_size" {
  description = "Boot disk size in GB"
  type        = number
  default     = 50
}

variable "zone" {
  description = "GCP zone"
  type        = string
  default     = "us-central1-a"
}

variable "firewall_rules" {
  description = "List of firewall rules"
  type = list(object({
    name      = string
    rule_type = string
    cidr      = string
  }))
  default = []
}

variable "members" {
  description = "List of members with SSH access"
  type = list(object({
    email = string
    role  = string
  }))
  default = []
}

variable "secrets" {
  description = "List of secrets from Vault"
  type = list(object({
    name       = string
    vault_path = string
  }))
  default = []
}

variable "users" {
  description = "Map of users with SSH keys for reconciliation"
  type        = map(list(string))
  default     = {}
}

variable "docker_compose_repo" {
  description = "Docker Compose repository URL"
  type        = string
  default     = "" // Added default to fix missing var issue
}

variable "docker_compose_init" {
  description = "Docker Compose initialization script"
  type        = string
  default     = ""
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "https_allowed_rules" {
  description = "List of CIDR ranges allowed for HTTPS access"
  type        = list(string)
  default     = []
}

locals {
  https_allowed_rules = [
    for rule in var.firewall_rules : rule.cidr
    if rule.rule_type == "https_allowed"
  ]
  ssh_allowed_rules = [
    for rule in var.firewall_rules : rule.cidr
    if rule.rule_type == "ssh_allowed"
  ]
  # Fallback for docker_compose_repo
  final_docker_compose_repo = var.docker_compose_repo != "" ? var.docker_compose_repo : var.github_repo
}

resource "google_compute_firewall" "ssh_allowed" {
  project = var.gcp_project_id
  name    = "libops-ssh"
  network = "default"

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = local.ssh_allowed_rules
  target_tags   = ["libops-${substr(var.public_id, 0, 8)}"]
}

# Service Account for Site VM
resource "google_service_account" "site" {
  project      = var.gcp_project_id
  account_id   = "site-${substr(var.public_id, 0, 8)}"
  display_name = "Site VM ${var.name}"
}

module "machine" {
  source = "git::https://github.com/libops/cloud-compose?ref=0.2.0"

  name                = var.name
  project_id          = var.gcp_project_id
  project_number      = var.gcp_project_number
  docker_compose_repo = local.final_docker_compose_repo
  docker_compose_init = var.docker_compose_init
  region              = var.region
  zone                = var.zone
  run_snapshots       = true
  allowed_ips         = var.https_allowed_rules
  users               = var.users
  rootfs              = "${path.module}/rootfs"
  project_public_id   = var.project_id
  site_public_id      = var.public_id
  service_account_email = google_service_account.site.email
  
  runcmd = [
    "bash /mnt/disks/data/libops/setup-watchers.sh",
    "bash /mnt/disks/data/libops/iptables-smtp.sh",
    "bash /mnt/disks/data/libops/deploy-vm-controller.sh",
    "systemctl start reconcile-ssh-keys.timer",
    "systemctl start reconcile-secrets.service",
  ]
}

output "external_ip" {
  description = "External IP address of the instance"
  value       = module.machine.external_ip
}

output "instance_id" {
  description = "Instance ID"
  value       = module.machine.instance_id
}

output "site_info" {
  value = {
    public_id       = var.public_id
    service_account = google_service_account.site.email
    external_ip     = module.machine.external_ip
  }
}