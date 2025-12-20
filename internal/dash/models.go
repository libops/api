package dash

// LoginPageData holds data for the login page template.
type LoginPageData struct {
	Message       string
	Error         string
	Verified      bool
	RedirectURI   string
	State         string
	IsDevelopment bool
}

// SuccessPageData holds data for the success page template.
type SuccessPageData struct {
	IDToken   string
	ExpiresIn int
	Email     string
	AccountID string
}

// DashboardPageData holds data for the dashboard page template.
type DashboardPageData struct {
	Email         string
	Name          string
	Organizations []Organization
	ActivePage    string
	IsDevelopment bool
}

// Organization represents an organization for the dashboard.
type Organization struct {
	ID          string
	Name        string
	Description string
	Role        string
}

// ResourcePageData holds data for resource pages (organizations, projects, sites, etc.)
type ResourcePageData struct {
	Email         string
	Name          string
	ActivePage    string
	ResourceName  string // e.g., "Organizations", "Projects"
	Items         []ResourceItem
	IsDevelopment bool
}

// ResourceItem represents a generic resource item
type ResourceItem struct {
	ID          string
	Name        string
	Description string
	Status      string
	CreatedAt   string
	UpdatedAt   string
	ParentName  string
	ParentID    string
	ParentType  string // "organization", "project", or "site"
	Permissions ResourcePermissions
}

// OrganizationDetailData holds data for the organization detail page
type OrganizationDetailData struct {
	Email         string
	Name          string
	ActivePage    string
	Organization  Organization
	Projects      []ResourceItem
	Members       []Member
	FirewallRules []ResourceItem
	Secrets       []ResourceItem
	Settings      []Setting
	AuditLog      []AuditLogEntry
	IsDevelopment bool
}

// ProjectDetailData holds data for the project detail page
type ProjectDetailData struct {
	Email         string
	Name          string
	ActivePage    string
	Project       ResourceItem
	Sites         []ResourceItem
	Members       []Member
	FirewallRules []ResourceItem
	Secrets       []ResourceItem
	Settings      []Setting
	AuditLog      []AuditLogEntry
	IsDevelopment bool
}

// SiteDetailData holds data for the site detail page
type SiteDetailData struct {
	Email          string
	Name           string
	ActivePage     string
	Site           ResourceItem
	OrganizationID string
	ProjectID      string
	Members        []Member
	FirewallRules  []ResourceItem
	Secrets        []ResourceItem
	Settings       []Setting
	AuditLog       []AuditLogEntry
	IsDevelopment  bool
}

// Member represents a member with their role
type Member struct {
	MemberID    string
	Email       string
	Role        string
	ParentName  string
	ParentID    string
	ParentType  string // "organization", "project", or "site"
	Permissions ResourcePermissions
}

// ResourcePermissions holds permission information for a resource
type ResourcePermissions struct {
	CanEdit   bool
	CanDelete bool
}

// Setting represents a configuration setting
type Setting struct {
	ID          string
	Key         string
	Value       string
	Description string
	Editable    bool
	ParentName  string
	ParentID    string
	ParentType  string // "organization", "project", or "site"
	Permissions ResourcePermissions
}

// AuditLogEntry represents an audit log entry
type AuditLogEntry struct {
	Action      string
	Description string
	Timestamp   string
}

// APIKeysPageData holds data for the API keys page
type APIKeysPageData struct {
	Email         string
	Name          string
	ActivePage    string
	IsDevelopment bool
}

// SSHKeysPageData holds data for the SSH keys page
type SSHKeysPageData struct {
	Email         string
	Name          string
	AccountID     string
	ActivePage    string
	IsDevelopment bool
}
