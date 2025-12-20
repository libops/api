package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/api/idtoken"

	"github.com/libops/api/db"
)

// SecurityConfig holds security parameters
type SecurityConfig struct {
	// Maximum message size (10KB should be enough for all messages)
	MaxMessageSize int64

	// Rate limiting: max messages per second per connection
	MaxMessagesPerSecond int

	// Rate limiting: burst capacity
	MessageBurstCapacity int

	// Maximum connections per site
	MaxConnectionsPerSite int

	// Allowed message types from clients
	AllowedClientMessageTypes map[string]bool

	// Heartbeat validation
	MaxHeartbeatJitter time.Duration // Max deviation from expected interval
}

// DefaultSecurityConfig returns secure defaults
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		MaxMessageSize:        10 * 1024, // 10KB
		MaxMessagesPerSecond:  5,         // 5 messages/sec (generous for ping + occasional completion)
		MessageBurstCapacity:  10,        // Allow bursts of 10
		MaxConnectionsPerSite: 1,         // Only 1 connection per site
		AllowedClientMessageTypes: map[string]bool{
			"ping":                    true,
			"reconciliation_complete": true,
			"reconciliation_error":    true,
		},
		MaxHeartbeatJitter: 10 * time.Second,
	}
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	tokens         int
	maxTokens      int
	refillRate     int // tokens per second
	lastRefill     time.Time
	mu             sync.Mutex
	violationCount int
}

// NewRateLimiter creates a rate limiter
func NewRateLimiter(maxTokens, refillRate int) *RateLimiter {
	return &RateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if request is allowed
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	tokensToAdd := int(elapsed.Seconds()) * rl.refillRate

	if tokensToAdd > 0 {
		rl.tokens = min(rl.tokens+tokensToAdd, rl.maxTokens)
		rl.lastRefill = now
	}

	// Check if we have tokens
	if rl.tokens > 0 {
		rl.tokens--
		return true
	}

	rl.violationCount++
	return false
}

// GetViolationCount returns number of rate limit violations
func (rl *RateLimiter) GetViolationCount() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.violationCount
}

// PendingReconciliation tracks reconciliations we're expecting responses for
type PendingReconciliation struct {
	RequestID   string
	Target      string
	TriggeredAt time.Time
	Timeout     time.Time
}

// ConnectionManager manages WebSocket connections from customer VMs
type ConnectionManager struct {
	db                     db.Querier
	connections            sync.Map // map[int64]*SiteConnection (key: site.ID)
	lastPing               sync.Map // map[int64]time.Time
	upgrader               websocket.Upgrader
	apiAudience            string // Expected JWT audience (e.g., "https://api.libops.io")
	config                 *SecurityConfig
	rateLimiters           sync.Map // map[int64]*RateLimiter (siteID -> limiter)
	pendingReconciliations sync.Map // map[string]*PendingReconciliation (requestID -> pending)
	connectionCounts       sync.Map // map[int64]int (siteID -> count)
}

// SiteConnection represents a connected site VM
type SiteConnection struct {
	SiteID      int64
	SiteUUID    string
	ProjectID   string
	Conn        *websocket.Conn
	ConnectedAt time.Time
	mu          sync.Mutex
}

// Message represents a WebSocket message
type Message struct {
	Type      string          `json:"type"`
	Timestamp int64           `json:"timestamp,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// ReconciliationRequest sent from API to VM
type ReconciliationRequest struct {
	Type      string `json:"type"`   // "reconcile"
	Target    string `json:"target"` // "ssh_keys", "secrets", "firewall", "general"
	RequestID string `json:"request_id"`
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(querier db.Querier, apiAudience string) *ConnectionManager {
	config := DefaultSecurityConfig()

	cm := &ConnectionManager{
		db:          querier,
		apiAudience: apiAudience,
		config:      config,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins since we validate via JWT
				return true
			},
			HandshakeTimeout: 10 * time.Second,
		},
	}

	// Start background tasks
	go cm.monitorHeartbeats()
	go cm.cleanupPendingReconciliations()

	return cm
}

// HandleConnect handles WebSocket upgrade and connection management
func (cm *ConnectionManager) HandleConnect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract and validate JWT token
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		slog.Warn("connection attempt without authorization header",
			"remote_addr", r.RemoteAddr)
		http.Error(w, "Missing authorization", http.StatusUnauthorized)
		RecordConnectionAttempt("missing_auth")
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")

	// Validate JWT using Google's public keys
	// The VM generates this JWT using its service account via the metadata server
	// JWT structure:
	// - Header: {"alg":"RS256","typ":"JWT"}
	// - Claims: {
	//     "iss": "vm-{site_uuid}@{project}.iam.gserviceaccount.com",
	//     "sub": "...",
	//     "email": "vm-{site_uuid}@{project}.iam.gserviceaccount.com",
	//     "aud": "https://api.libops.io",
	//     "exp": ...,
	//     "iat": ...
	//   }
	payload, err := idtoken.Validate(ctx, token, cm.apiAudience)
	if err != nil {
		slog.Warn("invalid JWT token",
			"error", err,
			"remote_addr", r.RemoteAddr)
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		RecordJWTValidation("invalid_signature")
		RecordConnectionAttempt("invalid_token")
		return
	}

	// SECURITY: Extract and validate service account email from JWT claims
	// This is the critical identity field - VMs authenticate using their service account
	email, ok := payload.Claims["email"].(string)
	if !ok || email == "" {
		slog.Warn("JWT missing or empty email claim",
			"remote_addr", r.RemoteAddr,
			"claims", payload.Claims)
		http.Error(w, "Invalid token claims", http.StatusForbidden)
		RecordJWTValidation("invalid_email")
		RecordConnectionAttempt("missing_email")
		return
	}

	// SECURITY: Validate email is a well-formed service account
	// Must end with .iam.gserviceaccount.com to be a valid GCP service account
	if !strings.HasSuffix(email, ".iam.gserviceaccount.com") {
		slog.Warn("JWT email is not a service account",
			"email", email,
			"remote_addr", r.RemoteAddr)
		http.Error(w, "Invalid service account", http.StatusForbidden)
		RecordJWTValidation("not_service_account")
		RecordConnectionAttempt("not_service_account")
		return
	}

	// SECURITY: Verify JWT audience matches our expected audience
	// This prevents token reuse from other services
	if payload.Audience != cm.apiAudience {
		slog.Warn("JWT audience mismatch",
			"expected", cm.apiAudience,
			"got", payload.Audience,
			"email", email)
		http.Error(w, "Invalid token audience", http.StatusForbidden)
		RecordJWTValidation("invalid_audience")
		RecordConnectionAttempt("audience_mismatch")
		return
	}

	// JWT validation successful
	RecordJWTValidation("success")

	// Parse site identity from service account email
	siteUUID, projectID, err := parseSiteFromServiceAccount(email)
	if err != nil {
		slog.Warn("invalid service account format",
			"email", email,
			"error", err)
		http.Error(w, "Invalid service account", http.StatusForbidden)
		RecordConnectionAttempt("malformed_sa")
		return
	}

	// Look up site in database
	site, err := cm.lookupSite(ctx, siteUUID)
	if err != nil {
		slog.Warn("unknown site attempted connection",
			"site_uuid", siteUUID,
			"service_account", email,
			"error", err)
		http.Error(w, "Site not found", http.StatusForbidden)
		RecordConnectionAttempt("site_not_found")
		return
	}

	// Verify site is active
	if site.Status.SitesStatus != db.SitesStatusActive {
		slog.Warn("inactive site attempted connection",
			"site_id", site.ID,
			"site_uuid", siteUUID,
			"status", site.Status.SitesStatus)
		http.Error(w, "Site not active", http.StatusForbidden)
		RecordConnectionAttempt("site_inactive")
		return
	}

	// Note: Project ID validation removed - site.ID is sufficient for authentication
	// The service account is tied to the specific site UUID, which is validated above

	// Upgrade to WebSocket
	conn, err := cm.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("failed to upgrade to websocket",
			"site_id", site.ID,
			"error", err)
		return
	}

	// SECURITY: Close any existing connection (prevent ghost connections)
	// This handles the "last write wins" case properly
	if oldInterface, loaded := cm.connections.LoadAndDelete(site.ID); loaded {
		oldConn := oldInterface.(*SiteConnection)
		slog.Warn("closing previous connection for site (duplicate connection)",
			"site_id", site.ID,
			"old_connected_at", oldConn.ConnectedAt)
		oldConn.Conn.Close() // Force close the old connection
		RecordDuplicateConnection()
	}

	// Create site connection
	siteConn := &SiteConnection{
		SiteID:      site.ID,
		SiteUUID:    siteUUID,
		ProjectID:   projectID,
		Conn:        conn,
		ConnectedAt: time.Now(),
	}

	// Store new connection
	cm.connections.Store(site.ID, siteConn)
	cm.lastPing.Store(site.ID, time.Now())

	// Record successful connection
	RecordConnectionAttempt("success")
	RecordConnectionEstablished()
	SetSiteConnectionStatus(fmt.Sprintf("%d", site.ID), true)
	SetSiteLastPingTimestamp(fmt.Sprintf("%d", site.ID), time.Now().Unix())

	slog.Info("site connected",
		"site_id", site.ID,
		"site_uuid", siteUUID,
		"project_id", projectID)

	// Trigger initial reconciliation
	go cm.triggerInitialReconciliation(siteConn)

	// Handle messages from this connection
	go cm.handleMessages(siteConn)
}

// parseSiteFromServiceAccount extracts site UUID and project ID from service account email
// Expected format: vm-{site_uuid_short}@{project_id}.iam.gserviceaccount.com
func parseSiteFromServiceAccount(email string) (siteUUID, projectID string, err error) {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid email format")
	}

	localPart := parts[0]
	domainPart := parts[1]

	// Verify it's a VM service account
	if !strings.HasPrefix(localPart, "vm-") {
		return "", "", fmt.Errorf("not a VM service account")
	}

	// Extract short UUID (first 8 chars of site UUID)
	siteUUIDShort := strings.TrimPrefix(localPart, "vm-")
	if len(siteUUIDShort) != 8 {
		return "", "", fmt.Errorf("invalid UUID short format")
	}

	// Extract project ID from domain
	if !strings.HasSuffix(domainPart, ".iam.gserviceaccount.com") {
		return "", "", fmt.Errorf("invalid service account domain")
	}
	projectID = strings.TrimSuffix(domainPart, ".iam.gserviceaccount.com")

	// Return short UUID - we'll use it to lookup the full UUID in database
	return siteUUIDShort, projectID, nil
}

// lookupSite finds site by short UUID (first 8 chars)
func (cm *ConnectionManager) lookupSite(ctx context.Context, siteUUIDShort string) (*db.GetSiteByShortUUIDRow, error) {
	// Use the SQLC-generated query to find site by short UUID
	site, err := cm.db.GetSiteByShortUUID(ctx, siteUUIDShort)
	if err != nil {
		return nil, fmt.Errorf("site not found: %w", err)
	}

	return &site, nil
}

// handleMessages processes incoming messages from a site connection
func (cm *ConnectionManager) handleMessages(siteConn *SiteConnection) {
	defer func() {
		// Clean up on disconnect
		cm.connections.Delete(siteConn.SiteID)
		cm.lastPing.Delete(siteConn.SiteID)
		cm.rateLimiters.Delete(siteConn.SiteID)
		cm.decrementConnectionCount(siteConn.SiteID)
		siteConn.Conn.Close()

		// Record connection closed with duration
		duration := time.Since(siteConn.ConnectedAt)
		RecordConnectionClosed(duration.Seconds())
		SetSiteConnectionStatus(fmt.Sprintf("%d", siteConn.SiteID), false)

		slog.Info("site disconnected",
			"site_id", siteConn.SiteID,
			"duration", duration)
	}()

	// Set read limits
	siteConn.Conn.SetReadLimit(cm.config.MaxMessageSize)

	// SECURITY: Set initial read deadline (prevent Slowloris attacks)
	_ = siteConn.Conn.SetReadDeadline(time.Now().Add(90 * time.Second))

	// SECURITY: Set pong handler to extend deadline automatically
	siteConn.Conn.SetPongHandler(func(string) error {
		_ = siteConn.Conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})

	// Get or create rate limiter for this site
	limiter := cm.getRateLimiter(siteConn.SiteID)

	// Track last ping time for jitter detection
	var lastPingTime time.Time

	for {
		var msg Message
		err := siteConn.Conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Warn("unexpected websocket close",
					"site_id", siteConn.SiteID,
					"error", err)
			}
			// Check if it's a read deadline error (Slowloris attack or dead connection)
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				slog.Warn("read deadline exceeded - connection timeout",
					"site_id", siteConn.SiteID)
				RecordReadTimeout()
			}
			return
		}

		// Record inbound message (approximate size based on message type)
		msgSize := len(msg.Data) + len(msg.Type) + 20 // rough estimate
		RecordMessage(msg.Type, "inbound", msgSize)

		// SECURITY 1: RATE LIMITING
		if !limiter.Allow() {
			slog.Warn("rate limit exceeded",
				"site_id", siteConn.SiteID,
				"violations", limiter.GetViolationCount())

			// Close connection after too many violations
			if limiter.GetViolationCount() > 100 {
				slog.Error("excessive rate limit violations, closing connection",
					"site_id", siteConn.SiteID)
				cm.trackSecurityEvent(siteConn.SiteID, "rate_limit_abuse")
				return
			}
			continue
		}

		// SECURITY 2: MESSAGE TYPE VALIDATION
		if !cm.config.AllowedClientMessageTypes[msg.Type] {
			slog.Warn("invalid message type from client",
				"site_id", siteConn.SiteID,
				"type", msg.Type)
			cm.trackSecurityEvent(siteConn.SiteID, "invalid_message_type")
			RecordSecurityEvent("invalid_message_type")
			continue
		}

		// PROCESS ALLOWED MESSAGE TYPES
		switch msg.Type {
		case "ping":
			now := time.Now()

			// SECURITY: Check for heartbeat jitter (too fast or too slow)
			if !lastPingTime.IsZero() {
				interval := now.Sub(lastPingTime)
				expectedInterval := 30 * time.Second
				jitter := interval - expectedInterval

				if jitter < 0 {
					jitter = -jitter
				}

				if jitter > cm.config.MaxHeartbeatJitter {
					slog.Warn("unusual heartbeat timing",
						"site_id", siteConn.SiteID,
						"interval", interval,
						"jitter", jitter)
					cm.trackSecurityEvent(siteConn.SiteID, "heartbeat_jitter")
				}
			}

			lastPingTime = now

			// SECURITY: Validate timestamp is reasonable (within 5 minutes of now)
			if msg.Timestamp != 0 {
				msgTime := time.Unix(msg.Timestamp, 0)
				timeDiff := now.Sub(msgTime)
				if timeDiff < 0 {
					timeDiff = -timeDiff
				}

				if timeDiff > 5*time.Minute {
					slog.Warn("ping timestamp too far from current time",
						"site_id", siteConn.SiteID,
						"timestamp", msg.Timestamp,
						"diff", timeDiff)
					cm.trackSecurityEvent(siteConn.SiteID, "bad_timestamp")
				}
			}

			// Update last ping time and metrics
			cm.lastPing.Store(siteConn.SiteID, now)
			SetSiteLastPingTimestamp(fmt.Sprintf("%d", siteConn.SiteID), now.Unix())

			// Extend read deadline on successful application ping
			_ = siteConn.Conn.SetReadDeadline(now.Add(90 * time.Second))

			// Send pong
			siteConn.mu.Lock()
			err := siteConn.Conn.WriteJSON(Message{
				Type:      "pong",
				Timestamp: now.Unix(),
			})
			siteConn.mu.Unlock()

			if err != nil {
				slog.Warn("failed to send pong",
					"site_id", siteConn.SiteID,
					"error", err)
				return
			}

			// Record outbound pong message
			RecordMessage("pong", "outbound", 50)

		case "reconciliation_complete", "reconciliation_error":
			// SECURITY: Validate reconciliation response
			var data map[string]interface{}
			if err := json.Unmarshal(msg.Data, &data); err != nil {
				slog.Warn("invalid reconciliation response data",
					"site_id", siteConn.SiteID,
					"error", err)
				cm.trackSecurityEvent(siteConn.SiteID, "invalid_response_data")
				continue
			}

			requestID, ok := data["request_id"].(string)
			if !ok || requestID == "" {
				slog.Warn("reconciliation response missing request_id",
					"site_id", siteConn.SiteID)
				cm.trackSecurityEvent(siteConn.SiteID, "missing_request_id")
				continue
			}

			// SECURITY: Verify we actually triggered this reconciliation
			pendingInterface, exists := cm.pendingReconciliations.Load(requestID)
			if !exists {
				slog.Warn("received response for unknown reconciliation",
					"site_id", siteConn.SiteID,
					"request_id", requestID)
				cm.trackSecurityEvent(siteConn.SiteID, "unknown_reconciliation")
				continue
			}

			pending := pendingInterface.(*PendingReconciliation)

			// Remove from pending
			cm.pendingReconciliations.Delete(requestID)

			// SECURITY: Validate target matches
			target, _ := data["target"].(string)
			if target != "" && target != pending.Target {
				slog.Warn("reconciliation target mismatch",
					"site_id", siteConn.SiteID,
					"expected", pending.Target,
					"got", target)
				cm.trackSecurityEvent(siteConn.SiteID, "target_mismatch")
			}

			// Calculate duration
			duration := time.Since(pending.TriggeredAt)

			// Record reconciliation completion
			status := "success"
			if msg.Type == "reconciliation_error" {
				status = "error"
			}
			RecordReconciliationCompletion(pending.Target, status, duration.Seconds())

			// Log completion
			if msg.Type == "reconciliation_complete" {
				slog.Info("reconciliation completed",
					"site_id", siteConn.SiteID,
					"request_id", requestID,
					"target", pending.Target,
					"duration", duration)
			} else {
				errorMsg, _ := data["error"].(string)
				slog.Error("reconciliation failed on VM",
					"site_id", siteConn.SiteID,
					"request_id", requestID,
					"target", pending.Target,
					"error", errorMsg,
					"duration", duration)
			}
		}
	}
}

// monitorHeartbeats runs in background and closes stale connections
func (cm *ConnectionManager) monitorHeartbeats() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		staleThreshold := 90 * time.Second

		cm.lastPing.Range(func(key, value interface{}) bool {
			siteID := key.(int64)
			lastPing := value.(time.Time)

			if now.Sub(lastPing) > staleThreshold {
				slog.Warn("closing stale connection",
					"site_id", siteID,
					"last_ping_ago", now.Sub(lastPing))

				// Close connection
				if connInterface, ok := cm.connections.Load(siteID); ok {
					conn := connInterface.(*SiteConnection)
					conn.Conn.Close()
				}

				// Clean up
				cm.connections.Delete(siteID)
				cm.lastPing.Delete(siteID)
			}

			return true
		})
	}
}

// cleanupPendingReconciliations removes expired pending reconciliations
func (cm *ConnectionManager) cleanupPendingReconciliations() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		cm.pendingReconciliations.Range(func(key, value interface{}) bool {
			pending := value.(*PendingReconciliation)
			if now.After(pending.Timeout) {
				slog.Warn("reconciliation timed out",
					"request_id", pending.RequestID,
					"target", pending.Target,
					"triggered_at", pending.TriggeredAt)
				cm.pendingReconciliations.Delete(key)

				// Record timeout
				duration := time.Since(pending.TriggeredAt)
				RecordReconciliationCompletion(pending.Target, "timeout", duration.Seconds())
			}
			return true
		})
	}
}

// TriggerReconciliation sends a reconciliation request to a connected site
func (cm *ConnectionManager) TriggerReconciliation(siteID int64, reconciliationType string) error {
	connInterface, ok := cm.connections.Load(siteID)
	if !ok {
		return fmt.Errorf("site %d not connected", siteID)
	}

	siteConn := connInterface.(*SiteConnection)

	requestID := generateRequestID()

	// Track this as pending
	pending := &PendingReconciliation{
		RequestID:   requestID,
		Target:      reconciliationType,
		TriggeredAt: time.Now(),
		Timeout:     time.Now().Add(5 * time.Minute), // 5 minute timeout
	}
	cm.pendingReconciliations.Store(requestID, pending)

	request := ReconciliationRequest{
		Type:      "reconcile",
		Target:    reconciliationType,
		RequestID: requestID,
	}

	siteConn.mu.Lock()
	err := siteConn.Conn.WriteJSON(request)
	siteConn.mu.Unlock()

	if err != nil {
		cm.pendingReconciliations.Delete(requestID)
		return fmt.Errorf("failed to send reconciliation request: %w", err)
	}

	// Record reconciliation trigger
	RecordReconciliationTrigger(reconciliationType)
	RecordMessage("reconcile", "outbound", 100)

	slog.Info("triggered reconciliation",
		"site_id", siteID,
		"type", reconciliationType,
		"request_id", requestID)

	return nil
}

// triggerInitialReconciliation triggers all reconciliation types when a site first connects
func (cm *ConnectionManager) triggerInitialReconciliation(siteConn *SiteConnection) {
	// Give the connection a moment to stabilize
	time.Sleep(1 * time.Second)

	for _, reconciliationType := range []string{"ssh_keys", "secrets", "firewall"} {
		if err := cm.TriggerReconciliation(siteConn.SiteID, reconciliationType); err != nil {
			slog.Error("failed to trigger initial reconciliation",
				"site_id", siteConn.SiteID,
				"type", reconciliationType,
				"error", err)
		}
	}
}

// GetConnectedSites returns list of currently connected site IDs
func (cm *ConnectionManager) GetConnectedSites() []int64 {
	var siteIDs []int64

	cm.connections.Range(func(key, value interface{}) bool {
		siteID := key.(int64)
		siteIDs = append(siteIDs, siteID)
		return true
	})

	return siteIDs
}

// GetConnectionCount returns the number of active connections
func (cm *ConnectionManager) GetConnectionCount() int {
	count := 0
	cm.connections.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// getRateLimiter gets or creates rate limiter for site
func (cm *ConnectionManager) getRateLimiter(siteID int64) *RateLimiter {
	limiterInterface, loaded := cm.rateLimiters.LoadOrStore(siteID,
		NewRateLimiter(cm.config.MessageBurstCapacity, cm.config.MaxMessagesPerSecond))

	if !loaded {
		slog.Debug("created rate limiter for site", "site_id", siteID)
	}

	return limiterInterface.(*RateLimiter)
}

// decrementConnectionCount decrements connection count
func (cm *ConnectionManager) decrementConnectionCount(siteID int64) {
	for {
		val, loaded := cm.connectionCounts.Load(siteID)
		if !loaded {
			return
		}

		count := val.(int)
		if count <= 1 {
			cm.connectionCounts.Delete(siteID)
			return
		}

		if cm.connectionCounts.CompareAndSwap(siteID, count, count-1) {
			return
		}
	}
}

// trackSecurityEvent logs security events
func (cm *ConnectionManager) trackSecurityEvent(siteID int64, eventType string) {
	slog.Warn("security event",
		"site_id", siteID,
		"event_type", eventType)
	RecordSecurityEvent(eventType)
}

// generateRequestID creates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
