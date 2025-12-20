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
	EventTypeSshKeyCreated = "io.libops.ssh_key.created.v1"
	EventTypeSshKeyDeleted = "io.libops.ssh_key.deleted.v1"

	// Organization Child Events
	EventTypeOrganizationMemberAdded         = "io.libops.organization.member.added.v1"
	EventTypeOrganizationMemberUpdated       = "io.libops.organization.member.updated.v1"
	EventTypeOrganizationMemberRemoved       = "io.libops.organization.member.removed.v1"
	EventTypeOrganizationFirewallRuleAdded   = "io.libops.organization.firewall_rule.added.v1"
	EventTypeOrganizationFirewallRuleRemoved = "io.libops.organization.firewall_rule.removed.v1"
	EventTypeOrganizationSecretCreated       = "io.libops.organization.secret.created.v1"
	EventTypeOrganizationSecretUpdated       = "io.libops.organization.secret.updated.v1"
	EventTypeOrganizationSecretDeleted       = "io.libops.organization.secret.deleted.v1"

	// Project Child Events
	EventTypeProjectMemberAdded         = "io.libops.project.member.added.v1"
	EventTypeProjectMemberUpdated       = "io.libops.project.member.updated.v1"
	EventTypeProjectMemberRemoved       = "io.libops.project.member.removed.v1"
	EventTypeProjectFirewallRuleAdded   = "io.libops.project.firewall_rule.added.v1"
	EventTypeProjectFirewallRuleRemoved = "io.libops.project.firewall_rule.removed.v1"
	EventTypeProjectSecretCreated       = "io.libops.project.secret.created.v1"
	EventTypeProjectSecretUpdated       = "io.libops.project.secret.updated.v1"
	EventTypeProjectSecretDeleted       = "io.libops.project.secret.deleted.v1"

	// Site Child Events
	EventTypeSiteMemberAdded         = "io.libops.site.member.added.v1"
	EventTypeSiteMemberUpdated       = "io.libops.site.member.updated.v1"
	EventTypeSiteMemberRemoved       = "io.libops.site.member.removed.v1"
	EventTypeSiteFirewallRuleAdded   = "io.libops.site.firewall_rule.added.v1"
	EventTypeSiteFirewallRuleRemoved = "io.libops.site.firewall_rule.removed.v1"
	EventTypeSiteSecretCreated       = "io.libops.site.secret.created.v1"
	EventTypeSiteSecretUpdated       = "io.libops.site.secret.updated.v1"
	EventTypeSiteSecretDeleted       = "io.libops.site.secret.deleted.v1"

	// Relationship events.
	EventTypeRelationshipCreated  = "io.libops.relationship.created.v1"
	EventTypeRelationshipApproved = "io.libops.relationship.approved.v1"
	EventTypeRelationshipRejected = "io.libops.relationship.rejected.v1"
)
