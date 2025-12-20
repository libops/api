# Main terraform configuration for LibOps infrastructure
# Uses for_each to manage multiple organizations, projects, and sites

terraform {
  required_version = ">= 1.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
  backend "gcs" {
    # Bucket configured via -backend-config flag
    prefix = "libops"
  }
}

variable "orchestrator_psc_ip" {
  description = "IP address of the Orchestrator PSC endpoint"
  type        = string
  default     = "10.128.0.99"
}

variable "organizations" {
  description = "Map of organizations keyed by public_id"
  type = map(object({
    name                = string
    gcp_org_id          = string
    gcp_billing_account = string
    gcp_parent          = string
    location            = string
  }))
  default = {}
}

variable "projects" {
  description = "Map of projects keyed by public_id"
  type = map(object({
    name                   = string
    organization_id        = string
    organization_folder_id = optional(string)
    github_repository      = optional(string)
    gcp_billing_account    = string
    machine_type           = string
    disk_size              = number
  }))
  default = {}
}

variable "sites" {
  description = "Map of sites keyed by public_id"
  type = map(object({
    name               = string
    project_id         = string
    gcp_project_id     = string
    gcp_project_number = string
    github_ref         = string
    github_repo        = string
    machine_type       = string
    disk_size          = number
    zone               = string
    firewall_rules = list(object({
      name      = string
      rule_type = string
      cidr      = string
    }))
    members = list(object({
      email = string
      role  = string
    }))
    secrets = list(object({
      name       = string
      vault_path = string
    }))
  }))
  default = {}
}

# Create organizations
module "organizations" {
  source   = "./modules/organization"
  for_each = var.organizations

  public_id           = each.key
  name                = each.value.name
  gcp_org_id          = each.value.gcp_org_id
  gcp_billing_account = each.value.gcp_billing_account
  gcp_parent          = each.value.gcp_parent
  location            = each.value.location
  orchestrator_psc_ip = var.orchestrator_psc_ip
}

# Create projects
module "projects" {
  source   = "./modules/project"
  for_each = var.projects

  public_id              = each.key
  name                   = each.value.name
  organization_id        = each.value.organization_id
  organization_folder_id = try(module.organizations[each.value.organization_id].folder_id, each.value.organization_folder_id)
  github_repository      = each.value.github_repository
  gcp_billing_account    = each.value.gcp_billing_account
  machine_type           = each.value.machine_type
  disk_size              = each.value.disk_size
}

# Create sites
module "sites" {
  source   = "./modules/site"
  for_each = var.sites

  public_id      = each.key
  name           = each.value.name
  project_id     = each.value.project_id
  gcp_project_id = try(module.projects[each.value.project_id].project_id, each.value.gcp_project_id)
  github_ref     = each.value.github_ref
  github_repo    = each.value.github_repo
  machine_type   = each.value.machine_type
  disk_size      = each.value.disk_size
  zone           = each.value.zone
  firewall_rules = each.value.firewall_rules
  members        = each.value.members
  secrets        = each.value.secrets
  users = {
    (each.value.project_id) = []
    (each.key)              = []
  }
}

output "organizations" {
  description = "Organization outputs keyed by public_id"
  value = {
    for k, org in module.organizations : k => {
      folder_id      = org.folder_id
      project_id     = org.project_id
      project_number = org.project_number
    }
  }
}

output "projects" {
  description = "Project outputs keyed by public_id"
  value = {
    for k, proj in module.projects : k => {
      project_id     = proj.project_id
      project_number = proj.project_number
    }
  }
}

output "sites" {
  description = "Site outputs keyed by public_id"
  value = {
    for k, site in module.sites : k => site.site_info
  }
}