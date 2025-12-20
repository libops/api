/**
 * Pub/Sub Topic and Subscription for Control Plane Events
 *
 * This module creates:
 * - Pub/Sub topic for site reconciliation requests
 * - Push subscription to Site Proxy Cloud Run service with 60s ack deadline
 */

resource "google_pubsub_topic" "site_events" {
  name    = var.topic_name
  project = var.project_id

  # Message retention: 7 days (default)
  message_retention_duration = "604800s"

  labels = {
    purpose = "control-plane-reconciliation"
    managed-by = "terraform"
  }
}

resource "google_pubsub_subscription" "site_proxy_push" {
  name    = var.subscription_name
  topic   = google_pubsub_topic.site_events.id
  project = var.project_id

  # Push configuration to Site Proxy Cloud Run service
  push_config {
    push_endpoint = var.proxy_push_endpoint

    # OIDC token for authentication
    dynamic "oidc_token" {
      for_each = var.proxy_service_account != "" ? [1] : []
      content {
        service_account_email = var.proxy_service_account
        audience             = var.proxy_push_endpoint
      }
    }

    attributes = {
      x-goog-version = "v1"
    }
  }

  # Ack deadline: 60 seconds (sufficient for site proxy fan-out processing)
  ack_deadline_seconds = 60

  # Message retention: 7 days
  message_retention_duration = "604800s"

  # Retry policy: exponential backoff
  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  # Dead letter policy: send to DLQ after 5 failed delivery attempts
  dynamic "dead_letter_policy" {
    for_each = var.enable_dead_letter_queue ? [1] : []
    content {
      dead_letter_topic     = google_pubsub_topic.dlq[0].id
      max_delivery_attempts = 5
    }
  }

  labels = {
    purpose    = "site-proxy-push-subscription"
    managed-by = "terraform"
  }
}

# Dead Letter Queue topic (optional)
resource "google_pubsub_topic" "dlq" {
  count   = var.enable_dead_letter_queue ? 1 : 0
  name    = "${var.topic_name}-dlq"
  project = var.project_id

  labels = {
    purpose    = "dead-letter-queue"
    managed-by = "terraform"
  }
}

# Grant Pub/Sub SA permission to publish to DLQ
resource "google_pubsub_topic_iam_member" "dlq_publisher" {
  count   = var.enable_dead_letter_queue ? 1 : 0
  topic   = google_pubsub_topic.dlq[0].id
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

# Grant Pub/Sub SA permission to subscribe to main topic
resource "google_pubsub_subscription_iam_member" "proxy_subscriber" {
  subscription = google_pubsub_subscription.site_proxy_push.id
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

# Get current project details
data "google_project" "current" {
  project_id = var.project_id
}

# Monitoring: Alert on DLQ messages
resource "google_monitoring_alert_policy" "dlq_messages" {
  count        = var.enable_dead_letter_queue && var.enable_monitoring ? 1 : 0
  display_name = "Pub/Sub DLQ Messages Detected"
  project      = var.project_id

  conditions {
    display_name = "Messages in DLQ"

    condition_threshold {
      filter          = "resource.type=\"pubsub_topic\" AND resource.labels.topic_id=\"${google_pubsub_topic.dlq[0].name}\" AND metric.type=\"pubsub.googleapis.com/topic/send_message_operation_count\""
      duration        = "60s"
      comparison      = "COMPARISON_GT"
      threshold_value = 0

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  notification_channels = var.notification_channels

  alert_strategy {
    auto_close = "1800s"
  }

  documentation {
    content = <<-EOT
      Messages are being sent to the Dead Letter Queue for topic ${var.topic_name}.
      This indicates that the Site Proxy service is failing to process events after 5 retry attempts.
      Investigate Site Proxy Cloud Run logs for errors.
    EOT
  }
}

# Monitoring: Alert on subscription backlog
resource "google_monitoring_alert_policy" "subscription_backlog" {
  count        = var.enable_monitoring ? 1 : 0
  display_name = "Pub/Sub Subscription Backlog"
  project      = var.project_id

  conditions {
    display_name = "Unacknowledged messages > 100"

    condition_threshold {
      filter          = "resource.type=\"pubsub_subscription\" AND resource.labels.subscription_id=\"${google_pubsub_subscription.site_proxy_push.name}\" AND metric.type=\"pubsub.googleapis.com/subscription/num_undelivered_messages\""
      duration        = "300s"
      comparison      = "COMPARISON_GT"
      threshold_value = 100

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_MAX"
      }
    }
  }

  notification_channels = var.notification_channels

  alert_strategy {
    auto_close = "3600s"
  }

  documentation {
    content = <<-EOT
      Pub/Sub subscription ${var.subscription_name} has more than 100 unacknowledged messages.
      This indicates that the Site Proxy service is not processing events fast enough.
      Check Site Proxy Cloud Run service for scaling issues or errors.
    EOT
  }
}
