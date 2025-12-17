package onboard

// Step1Request contains the organization name from step 1
type Step1Request struct {
	OrganizationName string `json:"organization_name"`
}

// Step2Request contains machine and disk configuration from step 2
type Step2Request struct {
	MachineType string `json:"machine_type"`
	DiskSizeGB  int    `json:"disk_size_gb"`
}

// StripeCheckoutResponse contains the Stripe checkout URL and billing skip info
type StripeCheckoutResponse struct {
	CheckoutURL string `json:"checkout_url"`           // Empty if billing is disabled
	SkipBilling bool   `json:"skip_billing,omitempty"` // True when DISABLE_BILLING is set
	NextStep    int32  `json:"next_step,omitempty"`    // Next step number (3 for Stripe, 4 to skip)
}

// Step4Request contains the project name from step 4
type Step4Request struct {
	ProjectName string `json:"project_name"`
}

// Step5Request contains GCP region configuration from step 5
type Step5Request struct {
	Country string `json:"country"`
	Region  string `json:"region"`
}

// Step6Request contains site name and GitHub repository selection from step 6
type Step6Request struct {
	SiteName   string `json:"site_name"`
	RepoOption string `json:"repo_option"` // "ojs", "isle-site-template", "custom"
	CustomURL  string `json:"custom_url,omitempty"`
	Port       int    `json:"port"` // Default 80
}

// Step7Request contains firewall IP configuration from step 7
type Step7Request struct {
	FirewallIP string `json:"firewall_ip"`
}

// OnboardingSessionResponse is returned to the frontend
type OnboardingSessionResponse struct {
	SessionID         string  `json:"session_id"`
	CurrentStep       int     `json:"current_step"`
	OrgName           *string `json:"org_name,omitempty"`
	MachineType       *string `json:"machine_type,omitempty"`
	DiskSizeGB        *int    `json:"disk_size_gb,omitempty"`
	ProjectName       *string `json:"project_name,omitempty"`
	GCPCountry        *string `json:"gcp_country,omitempty"`
	GCPRegion         *string `json:"gcp_region,omitempty"`
	SiteName          *string `json:"site_name,omitempty"`
	GitHubRepoURL     *string `json:"github_repo_url,omitempty"`
	Port              *int    `json:"port,omitempty"`
	FirewallIP        *string `json:"firewall_ip,omitempty"`
	OrganizationID    *int64  `json:"organization_id,omitempty"`
	StripeCheckoutID  *string `json:"stripe_checkout_session_id,omitempty"`
	StripeCheckoutURL *string `json:"stripe_checkout_url,omitempty"`
}

// ErrorResponse is a standard error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse is a standard success response
type SuccessResponse struct {
	Message string `json:"message"`
}
