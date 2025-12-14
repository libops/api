package dash

import (
	"net/http"
)

// RenderLoginPage renders the login/registration page
func RenderLoginPage(w http.ResponseWriter, data LoginPageData) {
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "login.html", data)
}

// RenderDashboard renders the dashboard page
func RenderDashboard(w http.ResponseWriter, data DashboardPageData) {
	// Set ActivePage for sidebar highlighting
	if data.ActivePage == "" {
		data.ActivePage = "overview"
	}
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "dashboard.html", data)
}

// RenderOrganizations renders the organizations page
func RenderOrganizations(w http.ResponseWriter, data ResourcePageData) {
	data.ActivePage = "organizations"
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "resources.html", data)
}

// RenderProjects renders the projects page
func RenderProjects(w http.ResponseWriter, data ResourcePageData) {
	data.ActivePage = "projects"
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "resources.html", data)
}

// RenderSites renders the sites page
func RenderSites(w http.ResponseWriter, data ResourcePageData) {
	data.ActivePage = "sites"
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "resources.html", data)
}

// RenderSecrets renders the secrets page
func RenderSecrets(w http.ResponseWriter, data ResourcePageData) {
	data.ActivePage = "secrets"
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "resources.html", data)
}

// RenderFirewall renders the firewall page
func RenderFirewall(w http.ResponseWriter, data ResourcePageData) {
	data.ActivePage = "firewall"
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "resources.html", data)
}

// RenderMembers renders the members page
func RenderMembers(w http.ResponseWriter, data ResourcePageData) {
	data.ActivePage = "members"
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "resources.html", data)
}

// RenderOrganizationDetail renders the organization detail page
func RenderOrganizationDetail(w http.ResponseWriter, data OrganizationDetailData) {
	data.ActivePage = "organizations"
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "organization_detail.html", data)
}

// RenderProjectDetail renders the project detail page
func RenderProjectDetail(w http.ResponseWriter, data ProjectDetailData) {
	data.ActivePage = "projects"
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "project_detail.html", data)
}

// RenderSiteDetail renders the site detail page
func RenderSiteDetail(w http.ResponseWriter, data SiteDetailData) {
	data.ActivePage = "sites"
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "site_detail.html", data)
}

// RenderAPIKeys renders the API keys page
func RenderAPIKeys(w http.ResponseWriter, data APIKeysPageData) {
	data.ActivePage = "api-keys"
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "api_keys.html", data)
}

// RenderSSHKeys renders the SSH keys page
func RenderSSHKeys(w http.ResponseWriter, data SSHKeysPageData) {
	data.ActivePage = "ssh-keys"
	data.IsDevelopment = IsDevelopment()
	RenderTemplate(w, "ssh_keys.html", data)
}
