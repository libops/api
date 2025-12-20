package workflows

import "time"

// Event represents a control plane event from the event_queue table
type Event struct {
	EventID        string
	EventType      string
	EventSource    string
	EventSubject   string
	EventData      []byte
	ContentType    string
	OrganizationID int64
	ProjectID      *int64
	SiteID         *int64
	CreatedAt      time.Time
}

// EventScope determines the scope of reconciliation needed
type EventScope int

const (
	ScopeUnknown EventScope = iota
	ScopeSite               // Affects a single site
	ScopeProject            // Affects a project (and all its sites)
	ScopeOrg                // Affects the entire organization
)

func (s EventScope) String() string {
	switch s {
	case ScopeUnknown:
		return "unknown"
	case ScopeSite:
		return "site"
	case ScopeProject:
		return "project"
	case ScopeOrg:
		return "organization"
	default:
		return "unknown"
	}
}

// ReconciliationType determines what type of reconciliation to perform
type ReconciliationType string

const (
	ReconcileSSHKeys   ReconciliationType = "ssh_keys"
	ReconcileSecrets   ReconciliationType = "secrets"
	ReconcileFirewall  ReconciliationType = "firewall"
	ReconcileDeployment ReconciliationType = "deployment"
	ReconcileFull      ReconciliationType = "full"
)

// DetermineScope analyzes an event to determine its reconciliation scope
func DetermineScope(event Event) EventScope {
	// Org-level events
	if isOrgLevelEvent(event.EventType) {
		return ScopeOrg
	}

	// Project-level events
	if isProjectLevelEvent(event.EventType) {
		return ScopeProject
	}

	// SSH-related events typically require org-level terraform
	if isSSHFirewallEvent(event.EventType) {
		return ScopeOrg
	}

	// Site-level events
	if event.SiteID != nil {
		return ScopeSite
	}

	// Default to org scope if unclear
	return ScopeOrg
}

func isOrgLevelEvent(eventType string) bool {
	orgEvents := []string{
		"io.libops.organization.created",
		"io.libops.organization.updated",
		"io.libops.organization.deleted",
		"io.libops.organization_firewall.created",
		"io.libops.organization_firewall.updated",
		"io.libops.organization_firewall.deleted",
		"io.libops.organization_member.created",
		"io.libops.organization_member.removed",
	}

	for _, prefix := range orgEvents {
		if len(eventType) >= len(prefix) && eventType[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func isProjectLevelEvent(eventType string) bool {
	projectEvents := []string{
		"io.libops.project.created",
		"io.libops.project.updated",
		"io.libops.project.deleted",
		"io.libops.project_firewall.created",
		"io.libops.project_firewall.updated",
		"io.libops.project_firewall.deleted",
		"io.libops.project_member.created",
		"io.libops.project_member.removed",
	}

	for _, prefix := range projectEvents {
		if len(eventType) >= len(prefix) && eventType[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func isSSHFirewallEvent(eventType string) bool {
	sshEvents := []string{
		"io.libops.site_firewall.created",
		"io.libops.site_firewall.updated",
		"io.libops.site_firewall.deleted",
	}

	for _, prefix := range sshEvents {
		if len(eventType) >= len(prefix) && eventType[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// DetermineReconciliationType determines what type of reconciliation to perform based on event types
func DetermineReconciliationType(eventTypes []string) ReconciliationType {
	hasSSHKeys := false
	hasSecrets := false
	hasFirewall := false
	hasDeployment := false

	for _, eventType := range eventTypes {
		switch {
		// Member events → SSH key reconciliation
		case contains(eventType, "member.created"),
			contains(eventType, "member.removed"),
			contains(eventType, "member.updated"):
			hasSSHKeys = true

		// Secret events → Secrets reconciliation
		case contains(eventType, "secret.created"),
			contains(eventType, "secret.updated"),
			contains(eventType, "secret.deleted"):
			hasSecrets = true

		// Firewall events → Firewall reconciliation
		case contains(eventType, "firewall.created"),
			contains(eventType, "firewall.updated"),
			contains(eventType, "firewall.deleted"):
			hasFirewall = true

		// Deployment events → Deployment reconciliation
		case contains(eventType, "deployment.created"),
			contains(eventType, "deployment.triggered"),
			contains(eventType, "github.push"):
			hasDeployment = true
		}
	}

	// If deployment event, always do deployment (takes priority)
	if hasDeployment {
		return ReconcileDeployment
	}

	// If multiple types, do full reconciliation
	typeCount := 0
	if hasSSHKeys {
		typeCount++
	}
	if hasSecrets {
		typeCount++
	}
	if hasFirewall {
		typeCount++
	}

	if typeCount > 1 {
		return ReconcileFull
	}

	// Single type
	if hasSSHKeys {
		return ReconcileSSHKeys
	}
	if hasSecrets {
		return ReconcileSecrets
	}
	if hasFirewall {
		return ReconcileFirewall
	}

	// Default to full reconciliation for unknown event types
	return ReconcileFull
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s[:len(substr)] == substr ||
		len(s) > len(substr) && s[len(s)-len(substr):] == substr)
}
