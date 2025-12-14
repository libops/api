// Package audit provides audit logging functionality for tracking user actions and system events.
package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/libops/api/internal/db"
)

// Event represents an audit event type.
type Event string

// Audit event constants define the types of events that can be logged.
const (
	UserLoginSuccess     Event = "user.login.success"
	UserLoginFailure     Event = "user.login.failure"
	APIKeyCreate         Event = "apikey.create"
	APIKeyDelete         Event = "apikey.delete"
	OrganizationCreate   Event = "organization.create"
	OrganizationUpdate   Event = "organization.update"
	OrganizationDelete   Event = "organization.delete"
	ProjectCreate        Event = "project.create"
	ProjectUpdate        Event = "project.update"
	ProjectDelete        Event = "project.delete"
	AccountCreate        Event = "account.create"
	AccountUpdate        Event = "account.update"
	AccountDelete        Event = "account.delete"
	SiteCreate           Event = "site.create"
	SiteUpdate           Event = "site.update"
	SiteDelete           Event = "site.delete"
	DeploymentSuccess    Event = "deployment.success"
	DeploymentFailure    Event = "deployment.failure"
	SSHKeyCreate         Event = "sshkey.create"
	SSHKeyDelete         Event = "sshkey.delete"
	AuthorizationFailure Event = "authorization.failure"

	// Organization Secret Events.
	OrganizationSecretCreateSuccess Event = "organization.secret.create.success"
	OrganizationSecretCreateFailed  Event = "organization.secret.create.failed"
	OrganizationSecretUpdateSuccess Event = "organization.secret.update.success"
	OrganizationSecretUpdateFailed  Event = "organization.secret.update.failed"
	OrganizationSecretDeleteSuccess Event = "organization.secret.delete.success"
	OrganizationSecretDeleteFailed  Event = "organization.secret.delete.failed"

	// Project Secret Events.
	ProjectSecretCreateSuccess Event = "project.secret.create.success"
	ProjectSecretCreateFailed  Event = "project.secret.create.failed"
	ProjectSecretUpdateSuccess Event = "project.secret.update.success"
	ProjectSecretUpdateFailed  Event = "project.secret.update.failed"
	ProjectSecretDeleteSuccess Event = "project.secret.delete.success"
	ProjectSecretDeleteFailed  Event = "project.secret.delete.failed"

	// Site Secret Events.
	SiteSecretCreateSuccess Event = "site.secret.create.success"
	SiteSecretCreateFailed  Event = "site.secret.create.failed"
	SiteSecretUpdateSuccess Event = "site.secret.update.success"
	SiteSecretUpdateFailed  Event = "site.secret.update.failed"
	SiteSecretDeleteSuccess Event = "site.secret.delete.success"
	SiteSecretDeleteFailed  Event = "site.secret.delete.failed"
)

// EntityType represents the type of entity being audited.
type EntityType string

// Entity type constants define the types of entities that can be audited.
const (
	AccountEntityType      EntityType = "accounts"
	OrganizationEntityType EntityType = "organizations"
	ProjectEntityType      EntityType = "projects"
	SiteEntityType         EntityType = "sites"
	SSHKeyEntityType       EntityType = "ssh_keys"
	APIKeyEntityType       EntityType = "api_keys"
)

// Logger handles audit event logging to the database and structured logging output.
type Logger struct {
	q db.Querier
}

// New creates a new audit logger instance.
func New(q db.Querier) *Logger {
	return &Logger{q: q}
}

// Log records an audit event to the database and structured logging output.
// It enriches the event with source IP, user agent, and request ID from the context.
func (l *Logger) Log(ctx context.Context, accountID, entityID int64, entityType EntityType, event Event, data map[string]any) {
	sourceIP := ExtractSourceIP(ctx)

	userAgent := ExtractUserAgent(ctx)

	if data == nil {
		data = make(map[string]any)
	}
	data["source_ip"] = sourceIP
	if userAgent != "" {
		data["user_agent"] = userAgent
	}

	if reqID := ctx.Value("request_id"); reqID != nil {
		data["request_id"] = reqID
	}

	eventData, err := json.Marshal(data)
	if err != nil {
		slog.Error("failed to marshal audit event data", "err", err)
		return
	}

	// Emit audit event to stdout for capture by logging agents
	// Sensitive fields are redacted via protobuf field options in the interceptor
	slog.Info("audit event",
		"event", string(event),
		"account_id", accountID,
		"entity_id", entityID,
		"entity_type", string(entityType),
		"source_ip", sourceIP,
		"data", data,
	)

	err = l.q.CreateAuditEvent(ctx, db.CreateAuditEventParams{
		AccountID:  accountID,
		EntityID:   entityID,
		EntityType: db.AuditEntityType(entityType),
		EventName:  string(event),
		EventData:  eventData,
	})
	if err != nil {
		slog.Error("failed to create audit event", "err", err)
	}
}

// ExtractSourceIP gets the source IP from HTTP request in context.
// Priority: X-Forwarded-For > X-Real-IP > RemoteAddr.
func ExtractSourceIP(ctx context.Context) string {
	if req, ok := ctx.Value("http_request").(*http.Request); ok && req != nil {
		xff := req.Header.Get("X-Forwarded-For")
		if xff != "" {
			// X-Forwarded-For may contain multiple IPs, take the first (client)
			ips := strings.Split(xff, ",")
			if len(ips) > 0 && strings.TrimSpace(ips[0]) != "" {
				return strings.TrimSpace(ips[0])
			}
		}

		xri := req.Header.Get("X-Real-IP")
		if xri != "" {
			return xri
		}

		if req.RemoteAddr != "" {
			if idx := strings.LastIndex(req.RemoteAddr, ":"); idx != -1 {
				return req.RemoteAddr[:idx]
			}
			return req.RemoteAddr
		}
	}

	return "unknown"
}

// ExtractUserAgent gets the user agent from HTTP request in context.
func ExtractUserAgent(ctx context.Context) string {
	if req, ok := ctx.Value("http_request").(*http.Request); ok && req != nil {
		return req.Header.Get("User-Agent")
	}
	return ""
}
