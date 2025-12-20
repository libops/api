package reconciler

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// WebSocket connection metrics
var (
	// Connection lifecycle metrics
	websocketConnectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "libops_websocket_connections_total",
			Help: "Total number of WebSocket connection attempts",
		},
		[]string{"status"}, // success, rejected, failed
	)

	websocketConnectionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "libops_websocket_connections_active",
			Help: "Current number of active WebSocket connections",
		},
	)

	websocketConnectionDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "libops_websocket_connection_duration_seconds",
			Help: "Duration of WebSocket connections in seconds",
			Buckets: []float64{
				60,    // 1 minute
				300,   // 5 minutes
				900,   // 15 minutes
				1800,  // 30 minutes
				3600,  // 1 hour
				7200,  // 2 hours
				14400, // 4 hours
				28800, // 8 hours
				86400, // 24 hours
			},
		},
	)

	// Message metrics
	websocketMessagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "libops_websocket_messages_total",
			Help: "Total number of WebSocket messages processed",
		},
		[]string{"type", "direction"}, // type: ping/reconciliation_complete/etc, direction: inbound/outbound
	)

	websocketMessageSizeBytes = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "libops_websocket_message_size_bytes",
			Help: "Size of WebSocket messages in bytes",
			Buckets: []float64{
				64,   // 64 B
				256,  // 256 B
				512,  // 512 B
				1024, // 1 KB
				2048, // 2 KB
				4096, // 4 KB (our limit)
			},
		},
	)

	// Security event metrics
	websocketSecurityEventsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "libops_websocket_security_events_total",
			Help: "Total number of security events detected",
		},
		[]string{"event_type"}, // read_timeout, message_too_large, invalid_message_type, etc.
	)

	websocketReadTimeoutsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "libops_websocket_read_timeouts_total",
			Help: "Total number of read timeout events (Slowloris detection)",
		},
	)

	websocketMessageSizeErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "libops_websocket_message_size_errors_total",
			Help: "Total number of messages rejected due to size limit",
		},
	)

	websocketDuplicateConnectionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "libops_websocket_duplicate_connections_total",
			Help: "Total number of duplicate connection attempts (ghost connection prevention)",
		},
	)

	// JWT validation metrics
	websocketJWTValidationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "libops_websocket_jwt_validation_total",
			Help: "Total number of JWT validation attempts",
		},
		[]string{"status"}, // success, invalid_signature, invalid_email, invalid_audience, expired
	)

	// Heartbeat metrics
	websocketHeartbeatIntervalSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "libops_websocket_heartbeat_interval_seconds",
			Help: "Time between consecutive heartbeat messages",
			Buckets: []float64{
				10, // 10 seconds
				20, // 20 seconds
				30, // 30 seconds (expected)
				40, // 40 seconds
				60, // 60 seconds
				90, // 90 seconds (timeout threshold)
			},
		},
	)

	websocketHeartbeatJitterSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "libops_websocket_heartbeat_jitter_seconds",
			Help: "Deviation from expected 30-second heartbeat interval",
			Buckets: []float64{
				1,  // 1 second
				2,  // 2 seconds
				5,  // 5 seconds
				10, // 10 seconds (threshold)
				20, // 20 seconds
				30, // 30 seconds
			},
		},
	)

	// Reconciliation metrics
	websocketReconciliationTriggersTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "libops_websocket_reconciliation_triggers_total",
			Help: "Total number of reconciliation requests sent to VMs",
		},
		[]string{"target"}, // ssh_keys, secrets, firewall
	)

	websocketReconciliationCompletionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "libops_websocket_reconciliation_completions_total",
			Help: "Total number of reconciliation completions received",
		},
		[]string{"target", "status"}, // target: ssh_keys/secrets/firewall, status: success/error
	)

	websocketReconciliationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "libops_websocket_reconciliation_duration_seconds",
			Help: "Duration of reconciliation operations in seconds",
			Buckets: []float64{
				1,   // 1 second
				5,   // 5 seconds
				10,  // 10 seconds
				30,  // 30 seconds
				60,  // 1 minute
				120, // 2 minutes
				300, // 5 minutes (timeout threshold)
			},
		},
		[]string{"target"}, // ssh_keys, secrets, firewall
	)

	// Site connection status
	websocketSiteConnectionStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "libops_websocket_site_connection_status",
			Help: "Current connection status per site (1=connected, 0=disconnected)",
		},
		[]string{"site_id"},
	)

	websocketSiteLastPingTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "libops_websocket_site_last_ping_timestamp",
			Help: "Unix timestamp of last ping received from site",
		},
		[]string{"site_id"},
	)
)

// Metric helper functions

// RecordConnectionAttempt records a WebSocket connection attempt
func RecordConnectionAttempt(status string) {
	websocketConnectionsTotal.WithLabelValues(status).Inc()
}

// RecordConnectionEstablished records a successful connection
func RecordConnectionEstablished() {
	websocketConnectionsActive.Inc()
}

// RecordConnectionClosed records a closed connection with duration
func RecordConnectionClosed(durationSeconds float64) {
	websocketConnectionsActive.Dec()
	websocketConnectionDuration.Observe(durationSeconds)
}

// RecordMessage records a WebSocket message
func RecordMessage(messageType, direction string, sizeBytes int) {
	websocketMessagesTotal.WithLabelValues(messageType, direction).Inc()
	websocketMessageSizeBytes.Observe(float64(sizeBytes))
}

// RecordSecurityEvent records a security event
func RecordSecurityEvent(eventType string) {
	websocketSecurityEventsTotal.WithLabelValues(eventType).Inc()
}

// RecordReadTimeout records a read timeout event
func RecordReadTimeout() {
	websocketReadTimeoutsTotal.Inc()
	RecordSecurityEvent("read_timeout")
}

// RecordMessageSizeError records a message size limit violation
func RecordMessageSizeError() {
	websocketMessageSizeErrorsTotal.Inc()
	RecordSecurityEvent("message_size_limit")
}

// RecordDuplicateConnection records a duplicate connection attempt
func RecordDuplicateConnection() {
	websocketDuplicateConnectionsTotal.Inc()
	RecordSecurityEvent("duplicate_connection")
}

// RecordJWTValidation records a JWT validation attempt
func RecordJWTValidation(status string) {
	websocketJWTValidationTotal.WithLabelValues(status).Inc()
}

// RecordHeartbeat records heartbeat timing metrics
func RecordHeartbeat(intervalSeconds, jitterSeconds float64) {
	websocketHeartbeatIntervalSeconds.Observe(intervalSeconds)
	websocketHeartbeatJitterSeconds.Observe(jitterSeconds)
}

// RecordReconciliationTrigger records a reconciliation request
func RecordReconciliationTrigger(target string) {
	websocketReconciliationTriggersTotal.WithLabelValues(target).Inc()
}

// RecordReconciliationCompletion records a reconciliation completion
func RecordReconciliationCompletion(target, status string, durationSeconds float64) {
	websocketReconciliationCompletionsTotal.WithLabelValues(target, status).Inc()
	websocketReconciliationDuration.WithLabelValues(target).Observe(durationSeconds)
}

// SetSiteConnectionStatus sets the connection status for a site
func SetSiteConnectionStatus(siteID string, connected bool) {
	value := float64(0)
	if connected {
		value = 1
	}
	websocketSiteConnectionStatus.WithLabelValues(siteID).Set(value)
}

// SetSiteLastPingTimestamp sets the last ping timestamp for a site
func SetSiteLastPingTimestamp(siteID string, timestamp int64) {
	websocketSiteLastPingTimestamp.WithLabelValues(siteID).Set(float64(timestamp))
}
