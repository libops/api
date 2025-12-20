package core

// TerraformRun represents a pending terraform run
type TerraformRun struct {
	RunID          string
	OrganizationID *int64
	ProjectID      *int64
	SiteID         *int64
	Modules        []string // ["organization", "project", "site"]
	EventIDs       []string
}

// ReconciliationRun represents a pending reconciliation run for VMs
type ReconciliationRun struct {
	RunID              string
	OrganizationID     *int64
	ProjectID          *int64
	SiteID             *int64
	ReconciliationType string  // ssh_keys, secrets, firewall, general
	TargetSiteIDs      []int64 // List of site IDs that need reconciliation
	EventIDs           []string
}
