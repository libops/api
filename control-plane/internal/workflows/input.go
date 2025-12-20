package workflows

// ReconciliationInput is the input for the PublishSiteReconciliation activity
type ReconciliationInput struct {
	OrgID              int64
	ProjectID          *int64
	SiteID             *int64
	EventIDs           []string
	EventTypes         []string
	Scope              EventScope
	ReconciliationType ReconciliationType
}

// ReconciliationResult is the result of the PublishSiteReconciliation activity
type ReconciliationResult struct {
	Status        string
	Message       string
	SitesAffected int
}
