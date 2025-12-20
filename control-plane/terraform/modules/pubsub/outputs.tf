output "topic_id" {
  description = "ID of the Pub/Sub topic"
  value       = google_pubsub_topic.site_events.id
}

output "topic_name" {
  description = "Name of the Pub/Sub topic"
  value       = google_pubsub_topic.site_events.name
}

output "subscription_id" {
  description = "ID of the push subscription"
  value       = google_pubsub_subscription.bridge_push.id
}

output "subscription_name" {
  description = "Name of the push subscription"
  value       = google_pubsub_subscription.bridge_push.name
}

output "dlq_topic_id" {
  description = "ID of the dead letter queue topic (if enabled)"
  value       = var.enable_dead_letter_queue ? google_pubsub_topic.dlq[0].id : null
}

output "dlq_topic_name" {
  description = "Name of the dead letter queue topic (if enabled)"
  value       = var.enable_dead_letter_queue ? google_pubsub_topic.dlq[0].name : null
}
