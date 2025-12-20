/**
 * Site Proxy Cloud Run Service
 *
 * This module creates a Cloud Run service that:
 * - Receives Pub/Sub push notifications for site reconciliation requests
 * - Fans out HTTP calls to site controllers at their external IPs
 * - Deployed in each customer GCP project to access sites in private VPC
 */

# Service account for Cloud Run service
resource "google_service_account" "site_proxy" {
  account_id   = var.service_account_id
  display_name = "Site Proxy Cloud Run Service"
  description  = "Service account for Site Proxy Cloud Run service"
  project      = var.project_id
}

# Grant permissions to call API
resource "google_project_iam_member" "site_proxy_invoker" {
  project = var.project_id
  role    = "roles/run.invoker"
  member  = "serviceAccount:${google_service_account.site_proxy.email}"
}

# Cloud Run service
resource "google_cloud_run_service" "site_proxy" {
  name     = var.service_name
  location = var.region
  project  = var.project_id

  template {
    spec {
      service_account_name = google_service_account.site_proxy.email

      containers {
        image = var.container_image

        env {
          name  = "API_URL"
          value = var.api_url
        }

        env {
          name  = "PORT"
          value = "8080"
        }

        resources {
          limits = {
            cpu    = var.cpu_limit
            memory = var.memory_limit
          }
        }
      }

      container_concurrency = var.container_concurrency
      timeout_seconds       = var.timeout_seconds
    }

    metadata {
      annotations = {
        "autoscaling.knative.dev/minScale" = tostring(var.min_instances)
        "autoscaling.knative.dev/maxScale" = tostring(var.max_instances)
      }
    }
  }

  traffic {
    percent         = 100
    latest_revision = true
  }

  metadata {
    labels = {
      purpose    = "site-reconciliation-proxy"
      managed-by = "terraform"
    }

    annotations = {
      "run.googleapis.com/ingress" = var.ingress_type
    }
  }
}

# Allow Pub/Sub to invoke the Cloud Run service
resource "google_cloud_run_service_iam_member" "pubsub_invoker" {
  service  = google_cloud_run_service.site_proxy.name
  location = google_cloud_run_service.site_proxy.location
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.site_proxy.email}"
}

# Allow unauthenticated invocations (Pub/Sub will use OIDC token)
# Note: In production, you should use authenticated push with OIDC tokens
resource "google_cloud_run_service_iam_member" "public_invoker" {
  count    = var.allow_unauthenticated ? 1 : 0
  service  = google_cloud_run_service.site_proxy.name
  location = google_cloud_run_service.site_proxy.location
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# VPC connector for accessing sites in private VPC
resource "google_vpc_access_connector" "site_proxy" {
  count   = var.create_vpc_connector ? 1 : 0
  name    = var.vpc_connector_name
  region  = var.region
  project = var.project_id

  subnet {
    name       = var.vpc_connector_subnet
    project_id = var.project_id
  }

  machine_type  = var.vpc_connector_machine_type
  min_instances = var.vpc_connector_min_instances
  max_instances = var.vpc_connector_max_instances
}

# Update Cloud Run service with VPC connector
resource "google_cloud_run_service" "site_proxy_with_vpc" {
  count    = var.create_vpc_connector ? 1 : 0
  name     = var.service_name
  location = var.region
  project  = var.project_id

  template {
    spec {
      service_account_name = google_service_account.site_proxy.email

      containers {
        image = var.container_image

        env {
          name  = "API_URL"
          value = var.api_url
        }

        env {
          name  = "PORT"
          value = "8080"
        }

        resources {
          limits = {
            cpu    = var.cpu_limit
            memory = var.memory_limit
          }
        }
      }

      container_concurrency = var.container_concurrency
      timeout_seconds       = var.timeout_seconds
    }

    metadata {
      annotations = {
        "autoscaling.knative.dev/minScale"      = tostring(var.min_instances)
        "autoscaling.knative.dev/maxScale"      = tostring(var.max_instances)
        "run.googleapis.com/vpc-access-connector" = google_vpc_access_connector.site_proxy[0].name
        "run.googleapis.com/vpc-access-egress"   = var.vpc_egress
      }
    }
  }

  traffic {
    percent         = 100
    latest_revision = true
  }

  metadata {
    labels = {
      purpose    = "site-reconciliation-proxy"
      managed-by = "terraform"
    }

    annotations = {
      "run.googleapis.com/ingress" = var.ingress_type
    }
  }

  depends_on = [google_vpc_access_connector.site_proxy]
}
