package events

// Event type constants following CloudEvents naming conventions
// Format: <reverse-dns>.<resource>.<action>.<version>

const (
	// Event source.
	EventSourceLibOpsAPI = "io.libops.api"

	// Account events.
	EventTypeAccountCreated = "io.libops.account.created.v1"
	EventTypeAccountUpdated = "io.libops.account.updated.v1"
	EventTypeAccountDeleted = "io.libops.account.deleted.v1"

	// Organization events.
	EventTypeOrganizationCreated = "io.libops.organization.created.v1"
	EventTypeOrganizationUpdated = "io.libops.organization.updated.v1"
	EventTypeOrganizationDeleted = "io.libops.organization.deleted.v1"

	// Project events.
	EventTypeProjectCreated = "io.libops.project.created.v1"
	EventTypeProjectUpdated = "io.libops.project.updated.v1"
	EventTypeProjectDeleted = "io.libops.project.deleted.v1"

	// Site events.
	EventTypeSiteCreated = "io.libops.site.created.v1"
	EventTypeSiteUpdated = "io.libops.site.updated.v1"
	EventTypeSiteDeleted = "io.libops.site.deleted.v1"

	// SSH Key events.
	EventTypeSshKeyCreated = "io.libops.sshkey.created.v1"
	EventTypeSshKeyUpdated = "io.libops.sshkey.updated.v1"
	EventTypeSshKeyDeleted = "io.libops.sshkey.deleted.v1"

	// Member events.
	EventTypeMemberAdded   = "io.libops.member.added.v1"
	EventTypeMemberUpdated = "io.libops.member.updated.v1"
	EventTypeMemberRemoved = "io.libops.member.removed.v1"

	// Relationship events.
	EventTypeRelationshipCreated  = "io.libops.relationship.created.v1"
	EventTypeRelationshipApproved = "io.libops.relationship.approved.v1"
	EventTypeRelationshipRejected = "io.libops.relationship.rejected.v1"

	// Firewall rule events.
	EventTypeFirewallRuleAdded   = "io.libops.firewall.added.v1"
	EventTypeFirewallRuleRemoved = "io.libops.firewall.removed.v1"

	// Developer events.
	EventTypeDeveloperAdded   = "io.libops.developer.added.v1"
	EventTypeDeveloperRemoved = "io.libops.developer.removed.v1"
)
