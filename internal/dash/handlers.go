package dash

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/libops/api/db"
	"github.com/libops/api/internal/auth"
)

// Handler provides HTTP handlers for dashboard pages
type Handler struct {
	db             db.Querier
	sessionManager *auth.SessionManager
}

// NewHandler creates a new dashboard handler
func NewHandler(queries db.Querier, sessionManager *auth.SessionManager) *Handler {
	return &Handler{
		db:             queries,
		sessionManager: sessionManager,
	}
}

// canUserPerformOnOrganization checks if user has permission to perform action on an organization
func (h *Handler) canUserPerformOnOrganization(ctx context.Context, userInfo *auth.UserInfo, orgID string, permission auth.Permission) bool {
	// Try to get authorizer from context first (if set by interceptor)
	authorizer, err := auth.GetAuthorizer(ctx)
	if err != nil {
		// If not in context, create a new authorizer
		authorizer = auth.NewAuthorizer(h.db)
	}

	id, err := uuid.Parse(orgID)
	if err != nil {
		return false
	}

	return authorizer.CheckOrganizationAccess(ctx, userInfo, id, permission) == nil
}

// canUserPerformOnProject checks if user has permission to perform action on a project
func (h *Handler) canUserPerformOnProject(ctx context.Context, userInfo *auth.UserInfo, projectID string, permission auth.Permission) bool {
	// Try to get authorizer from context first (if set by interceptor)
	authorizer, err := auth.GetAuthorizer(ctx)
	if err != nil {
		// If not in context, create a new authorizer
		authorizer = auth.NewAuthorizer(h.db)
	}

	id, err := uuid.Parse(projectID)
	if err != nil {
		return false
	}

	return authorizer.CheckProjectAccess(ctx, userInfo, id, permission) == nil
}

// canUserPerformOnSite checks if user has permission to perform action on a site
func (h *Handler) canUserPerformOnSite(ctx context.Context, userInfo *auth.UserInfo, siteID string, permission auth.Permission) bool {
	// Try to get authorizer from context first (if set by interceptor)
	authorizer, err := auth.GetAuthorizer(ctx)
	if err != nil {
		// If not in context, create a new authorizer
		authorizer = auth.NewAuthorizer(h.db)
	}

	id, err := uuid.Parse(siteID)
	if err != nil {
		return false
	}

	return authorizer.CheckSiteAccess(ctx, userInfo, id, permission) == nil
}

// HandleLoginPage handles requests to the login page
func (h *Handler) HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	data := LoginPageData{}

	// Check for query parameters
	if verified := r.URL.Query().Get("verified"); verified == "true" {
		data.Verified = true
	}

	if msg := r.URL.Query().Get("message"); msg != "" {
		data.Message = msg
	}

	if err := r.URL.Query().Get("error"); err != "" {
		data.Error = err
	}

	// Preserve OAuth parameters if present
	if redirectURI := r.URL.Query().Get("redirect_uri"); redirectURI != "" {
		data.RedirectURI = redirectURI
	}

	if state := r.URL.Query().Get("state"); state != "" {
		data.State = state
	}

	RenderLoginPage(w, data)
}

// HandleDashboard handles requests to the main dashboard page
func (h *Handler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	// Get user from context (set by auth middleware)
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		// Not authenticated, redirect to login
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	ctx := context.Background()

	// Get account details
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Get organizations for the user (including relationships)
	dbOrgs, err := h.db.ListUserOrganizations(ctx, db.ListUserOrganizationsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list organizations for dashboard", "account_id", account.ID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Convert to dashboard organizations
	organizations := make([]Organization, 0, len(dbOrgs))
	for _, org := range dbOrgs {
		organizations = append(organizations, Organization{
			ID:          org.PublicID,
			Name:        org.Name,
			Description: "", // Not available in ListUserOrganizationsRow
			Role:        string(org.Role),
		})
	}

	// Render dashboard
	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	RenderDashboard(w, DashboardPageData{
		Email:         account.Email,
		Name:          name,
		Organizations: organizations,
	})
}

// HandleOrganizations handles requests to the organizations page
func (h *Handler) HandleOrganizations(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	ctx := context.Background()
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	// Fetch organizations for the user (including relationships)
	dbOrgs, err := h.db.ListUserOrganizations(ctx, db.ListUserOrganizationsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list organizations", "account_id", account.ID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Convert to ResourceItems
	items := make([]ResourceItem, 0, len(dbOrgs))
	for _, org := range dbOrgs {
		items = append(items, ResourceItem{
			ID:          org.PublicID,
			Name:        org.Name,
			Description: "Role: " + string(org.Role), // Add role to description
			Status:      "Active",                    // Assuming active for all listed orgs
			CreatedAt:   "",                          // Not available in ListUserOrganizationsRow
		})
	}

	data := ResourcePageData{
		Email:        account.Email,
		Name:         name,
		ActivePage:   "organizations",
		ResourceName: "Organizations",
		Items:        items,
	}

	RenderOrganizations(w, data)
}

// HandleProjects handles requests to the projects page
func (h *Handler) HandleProjects(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	ctx := context.Background()
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	// Fetch projects for the user (including organization projects)
	dbProjects, err := h.db.ListUserProjectsWithOrg(ctx, db.ListUserProjectsWithOrgParams{
		AccountID:            account.ID,
		FilterOrganizationID: sql.NullInt64{},
		Limit:                100,
		Offset:               0,
	})
	if err != nil {
		slog.Error("Failed to list projects", "account_id", account.ID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Convert to ResourceItems
	items := make([]ResourceItem, 0, len(dbProjects))
	for _, proj := range dbProjects {
		items = append(items, ResourceItem{
			ID:          proj.PublicID,
			Name:        proj.Name,
			Description: "",
			Status:      "Active",
			CreatedAt:   "",
			ParentName:  proj.OrganizationName,
			ParentID:    proj.OrganizationPublicID,
			ParentType:  "organization",
		})
	}

	data := ResourcePageData{
		Email:        account.Email,
		Name:         name,
		ActivePage:   "projects",
		ResourceName: "Projects",
		Items:        items,
	}

	RenderProjects(w, data)
}

// HandleSites handles requests to the sites page
func (h *Handler) HandleSites(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	ctx := context.Background()
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	// Fetch sites for the user
	dbSites, err := h.db.ListUserSitesWithProject(ctx, db.ListUserSitesWithProjectParams{
		AccountID:            account.ID,
		FilterOrganizationID: sql.NullInt64{},
		FilterProjectID:      sql.NullInt64{},
		Limit:                100,
		Offset:               0,
	})
	if err != nil {
		slog.Error("Failed to list sites", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	items := make([]ResourceItem, 0, len(dbSites))
	for _, site := range dbSites {
		createdAt := ""
		if site.CreatedAt.Valid {
			createdAt = site.CreatedAt.Time.Format("2006-01-02")
		}
		status := ""
		if site.Status.Valid {
			status = string(site.Status.SitesStatus)
		}
		items = append(items, ResourceItem{
			ID:          site.PublicID,
			Name:        site.Name,
			Description: "",
			Status:      status,
			CreatedAt:   createdAt,
			ParentName:  site.ProjectName,
			ParentID:    site.ProjectPublicID,
			ParentType:  "project",
		})
	}

	data := ResourcePageData{
		Email:        account.Email,
		Name:         name,
		ActivePage:   "sites",
		ResourceName: "Sites",
		Items:        items,
	}

	RenderSites(w, data)
}

// HandleSecrets handles requests to the secrets page
func (h *Handler) HandleSecrets(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	ctx := context.Background()
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	// Fetch secrets for the user
	dbSecrets, err := h.db.ListUserSecrets(ctx, db.ListUserSecretsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list user secrets", "account_id", account.ID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	items := make([]ResourceItem, 0, len(dbSecrets))
	for _, secret := range dbSecrets {
		createdAt := ""
		if secret.CreatedAt != 0 { // Secrets timestamps are int64
			createdAt = time.Unix(secret.CreatedAt, 0).Format("2006-01-02")
		}
		items = append(items, ResourceItem{
			ID:          secret.PublicID,
			Name:        secret.Name,
			Description: "",
			Status:      string(secret.Status.OrganizationSecretsStatus),
			CreatedAt:   createdAt,
			ParentName:  secret.ParentName,
			ParentID:    secret.ParentPublicID,
			ParentType:  secret.ParentType,
		})
	}

	data := ResourcePageData{
		Email:        account.Email,
		Name:         name,
		ActivePage:   "secrets",
		ResourceName: "Secrets",
		Items:        items,
	}

	RenderSecrets(w, data)
}

// HandleFirewall handles requests to the firewall page
func (h *Handler) HandleFirewall(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	ctx := context.Background()
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	// Fetch firewall rules for the user
	dbRules, err := h.db.ListUserFirewallRules(ctx, db.ListUserFirewallRulesParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list user firewall rules", "account_id", account.ID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	items := make([]ResourceItem, 0, len(dbRules))
	for _, rule := range dbRules {
		createdAt := ""
		if rule.CreatedAt.Valid {
			createdAt = rule.CreatedAt.Time.Format("2006-01-02")
		}
		items = append(items, ResourceItem{
			ID:          rule.PublicID,
			Name:        rule.Name,
			Description: rule.Cidr + " (" + string(rule.RuleType) + ")", // More descriptive
			Status:      string(rule.Status.OrganizationFirewallRulesStatus),
			CreatedAt:   createdAt,
			ParentName:  rule.ParentName,
			ParentID:    rule.ParentPublicID,
			ParentType:  rule.ParentType,
		})
	}

	data := ResourcePageData{
		Email:        account.Email,
		Name:         name,
		ActivePage:   "firewall",
		ResourceName: "Firewall Rules",
		Items:        items,
	}

	RenderFirewall(w, data)
}

// HandleMembers handles requests to the members page
func (h *Handler) HandleMembers(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	ctx := context.Background()
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	// Fetch memberships for the user
	dbMemberships, err := h.db.ListUserMemberships(ctx, db.ListUserMembershipsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list user memberships", "account_id", account.ID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	items := make([]ResourceItem, 0, len(dbMemberships))
	for _, member := range dbMemberships {
		createdAt := ""
		if member.CreatedAt.Valid {
			createdAt = member.CreatedAt.Time.Format("2006-01-02")
		}
		items = append(items, ResourceItem{
			ID:          member.PublicID, // This is the public_id of the membership record
			Name:        member.Email,    // The email of the member
			Description: member.UserName.String + " (" + string(member.Role) + ")",
			Status:      string(member.Status.OrganizationMembersStatus),
			CreatedAt:   createdAt,
			ParentName:  member.ParentName,
			ParentID:    member.ParentPublicID,
			ParentType:  member.ParentType,
		})
	}

	data := ResourcePageData{
		Email:        account.Email,
		Name:         name,
		ActivePage:   "members",
		ResourceName: "Members",
		Items:        items,
	}

	RenderMembers(w, data)
}

// HandleOrganizationDetail handles requests to individual organization detail pages
func (h *Handler) HandleOrganizationDetail(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Extract organization ID from URL path
	orgID := r.PathValue("id")
	if orgID == "" {
		http.Error(w, "Organization ID required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	// Get organization details
	org, err := h.db.GetOrganization(ctx, orgID)
	if err != nil {
		slog.Error("Failed to get organization", "org_id", orgID, "err", err)
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	// Get projects for this organization
	dbProjects, err := h.db.ListOrganizationProjects(ctx, db.ListOrganizationProjectsParams{
		OrganizationID: org.ID,
		Limit:          100,
		Offset:         0,
	})
	if err != nil {
		slog.Error("Failed to list projects", "org_id", orgID, "err", err)
	}

	projects := make([]ResourceItem, 0, len(dbProjects))
	for _, proj := range dbProjects {
		projects = append(projects, ResourceItem{
			ID:     proj.PublicID,
			Name:   proj.Name,
			Status: "Active",
		})
	}

	// Get members with inheritance (includes relationships)
	dbMemberships, err := h.db.ListUserMemberships(ctx, db.ListUserMembershipsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list memberships", "org_id", orgID, "err", err)
	}

	members := make([]Member, 0)
	for _, membership := range dbMemberships {
		// Only include memberships for this specific organization
		if membership.ParentPublicID != org.PublicID {
			continue
		}

		// Check if user can edit/delete this membership
		canEdit := h.canUserPerformOnOrganization(r.Context(), userInfo, membership.ParentPublicID, auth.PermissionWrite)
		canDelete := h.canUserPerformOnOrganization(r.Context(), userInfo, membership.ParentPublicID, auth.PermissionOwner)

		members = append(members, Member{
			MemberID:   membership.AccountPublicID, // Use account_id for API endpoints
			Email:      membership.Email,
			Role:       string(membership.Role),
			ParentName: membership.ParentName,
			ParentID:   membership.ParentPublicID,
			ParentType: membership.ParentType,
			Permissions: ResourcePermissions{
				CanEdit:   canEdit,
				CanDelete: canDelete,
			},
		})
	}

	// Get firewall rules with inheritance (includes relationships)
	dbRules, err := h.db.ListUserFirewallRules(ctx, db.ListUserFirewallRulesParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list firewall rules", "org_id", orgID, "err", err)
	}

	firewallRules := make([]ResourceItem, 0)
	for _, rule := range dbRules {
		// Only include rules for this specific organization
		if rule.ParentPublicID != org.PublicID {
			continue
		}

		// Check if user can edit/delete this firewall rule
		canEdit := h.canUserPerformOnOrganization(r.Context(), userInfo, rule.ParentPublicID, auth.PermissionWrite)
		canDelete := h.canUserPerformOnOrganization(r.Context(), userInfo, rule.ParentPublicID, auth.PermissionOwner)

		firewallRules = append(firewallRules, ResourceItem{
			ID:         rule.PublicID,
			Name:       rule.Name,
			Status:     "Active",
			ParentName: rule.ParentName,
			ParentID:   rule.ParentPublicID,
			ParentType: rule.ParentType,
			Permissions: ResourcePermissions{
				CanEdit:   canEdit,
				CanDelete: canDelete,
			},
		})
	}

	// Get secrets with inheritance (includes relationships)
	dbSecrets, err := h.db.ListUserSecrets(ctx, db.ListUserSecretsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list secrets", "org_id", orgID, "err", err)
	}

	secrets := make([]ResourceItem, 0)
	for _, secret := range dbSecrets {
		// Only include secrets for this specific organization
		if secret.ParentPublicID != org.PublicID {
			continue
		}

		// Check if user can edit/delete this secret
		canEdit := h.canUserPerformOnOrganization(r.Context(), userInfo, secret.ParentPublicID, auth.PermissionWrite)
		canDelete := h.canUserPerformOnOrganization(r.Context(), userInfo, secret.ParentPublicID, auth.PermissionOwner)

		secrets = append(secrets, ResourceItem{
			ID:         secret.PublicID,
			Name:       secret.Name,
			Status:     "Active",
			ParentName: secret.ParentName,
			ParentID:   secret.ParentPublicID,
			ParentType: secret.ParentType,
			Permissions: ResourcePermissions{
				CanEdit:   canEdit,
				CanDelete: canDelete,
			},
		})
	}

	// Get settings with inheritance (includes relationships)
	dbSettings, err := h.db.ListUserSettings(ctx, db.ListUserSettingsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list settings", "org_id", orgID, "err", err)
	}

	settings := make([]Setting, 0)
	for _, setting := range dbSettings {
		// Only include settings for this specific organization
		if setting.ParentPublicID != org.PublicID {
			continue
		}

		// Check if user can edit/delete this setting
		canEdit := h.canUserPerformOnOrganization(r.Context(), userInfo, setting.ParentPublicID, auth.PermissionWrite)
		canDelete := h.canUserPerformOnOrganization(r.Context(), userInfo, setting.ParentPublicID, auth.PermissionOwner)

		description := ""
		if setting.Description.Valid {
			description = setting.Description.String
		}

		editable := false
		if setting.Editable.Valid {
			editable = setting.Editable.Bool
		}

		settings = append(settings, Setting{
			ID:          setting.PublicID,
			Key:         setting.SettingKey,
			Value:       setting.SettingValue,
			Description: description,
			Editable:    editable,
			ParentName:  setting.ParentName,
			ParentID:    setting.ParentPublicID,
			ParentType:  setting.ParentType,
			Permissions: ResourcePermissions{
				CanEdit:   canEdit,
				CanDelete: canDelete,
			},
		})
	}

	// TODO: Get audit log entries
	auditLog := []AuditLogEntry{}

	data := OrganizationDetailData{
		Email:      account.Email,
		Name:       name,
		ActivePage: "organizations",
		Organization: Organization{
			ID:          org.PublicID,
			Name:        org.Name,
			Description: "",
		},
		Projects:      projects,
		Members:       members,
		FirewallRules: firewallRules,
		Secrets:       secrets,
		Settings:      settings,
		AuditLog:      auditLog,
	}

	RenderOrganizationDetail(w, data)
}

// HandleProjectDetail handles requests to individual project detail pages
func (h *Handler) HandleProjectDetail(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	projectID := r.PathValue("id")
	if projectID == "" {
		http.Error(w, "Project ID required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	// Get project details
	project, err := h.db.GetProject(ctx, projectID)
	if err != nil {
		slog.Error("Failed to get project", "project_id", projectID, "err", err)
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// Get organization to access org public ID
	org, err := h.db.GetOrganizationByID(ctx, project.OrganizationID)
	if err != nil {
		slog.Error("Failed to get organization for project", "project_id", projectID, "org_id", project.OrganizationID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	orgPublicID := org.PublicID

	// Fetch sites for the project
	dbSites, err := h.db.ListProjectSites(ctx, db.ListProjectSitesParams{
		ProjectID: project.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list project sites", "project_id", projectID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sites := make([]ResourceItem, 0, len(dbSites))
	for _, site := range dbSites {
		createdAt := ""
		if site.CreatedAt.Valid {
			createdAt = site.CreatedAt.Time.Format("2006-01-02")
		}
		status := ""
		if site.Status.Valid {
			status = string(site.Status.SitesStatus)
		}
		sites = append(sites, ResourceItem{
			ID:          site.PublicID,
			Name:        site.Name,
			Description: "",
			Status:      status,
			CreatedAt:   createdAt,
			ParentName:  project.Name,
			ParentID:    project.PublicID,
			ParentType:  "project",
		})
	}

	// Get members with inheritance (includes org + project members)
	dbMemberships, err := h.db.ListUserMemberships(ctx, db.ListUserMembershipsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list memberships", "project_id", projectID, "err", err)
	}

	members := make([]Member, 0)
	for _, membership := range dbMemberships {
		// Include memberships for this project OR its parent organization
		if membership.ParentPublicID != project.PublicID && membership.ParentPublicID != orgPublicID {
			continue
		}

		// Check permissions - use project or org depending on where the membership is
		var canEdit, canDelete bool
		if membership.ParentType == "project" {
			canEdit = h.canUserPerformOnProject(r.Context(), userInfo, membership.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnProject(r.Context(), userInfo, membership.ParentPublicID, auth.PermissionOwner)
		} else {
			canEdit = h.canUserPerformOnOrganization(r.Context(), userInfo, membership.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnOrganization(r.Context(), userInfo, membership.ParentPublicID, auth.PermissionOwner)
		}

		members = append(members, Member{
			MemberID:   membership.AccountPublicID, // Use account_id for API endpoints
			Email:      membership.Email,
			Role:       string(membership.Role),
			ParentName: membership.ParentName,
			ParentID:   membership.ParentPublicID,
			ParentType: membership.ParentType,
			Permissions: ResourcePermissions{
				CanEdit:   canEdit,
				CanDelete: canDelete,
			},
		})
	}

	// Get firewall rules with inheritance (includes org + project rules)
	dbRules, err := h.db.ListUserFirewallRules(ctx, db.ListUserFirewallRulesParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list firewall rules", "project_id", projectID, "err", err)
	}

	firewallRules := make([]ResourceItem, 0)
	for _, rule := range dbRules {
		// Include rules for this project OR its parent organization
		if rule.ParentPublicID != project.PublicID && rule.ParentPublicID != orgPublicID {
			continue
		}

		// Check permissions - use project or org depending on where the rule is
		var canEdit, canDelete bool
		if rule.ParentType == "project" {
			canEdit = h.canUserPerformOnProject(r.Context(), userInfo, rule.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnProject(r.Context(), userInfo, rule.ParentPublicID, auth.PermissionOwner)
		} else {
			canEdit = h.canUserPerformOnOrganization(r.Context(), userInfo, rule.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnOrganization(r.Context(), userInfo, rule.ParentPublicID, auth.PermissionOwner)
		}

		createdAt := ""
		if rule.CreatedAt.Valid {
			createdAt = rule.CreatedAt.Time.Format("2006-01-02")
		}

		firewallRules = append(firewallRules, ResourceItem{
			ID:         rule.PublicID,
			Name:       rule.Name,
			Status:     "Active",
			CreatedAt:  createdAt,
			ParentName: rule.ParentName,
			ParentID:   rule.ParentPublicID,
			ParentType: rule.ParentType,
			Permissions: ResourcePermissions{
				CanEdit:   canEdit,
				CanDelete: canDelete,
			},
		})
	}

	// Get secrets with inheritance (includes org + project secrets)
	dbSecrets, err := h.db.ListUserSecrets(ctx, db.ListUserSecretsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list secrets", "project_id", projectID, "err", err)
	}

	secrets := make([]ResourceItem, 0)
	for _, secret := range dbSecrets {
		// Include secrets for this project OR its parent organization
		if secret.ParentPublicID != project.PublicID && secret.ParentPublicID != orgPublicID {
			continue
		}

		// Check permissions - use project or org depending on where the secret is
		var canEdit, canDelete bool
		if secret.ParentType == "project" {
			canEdit = h.canUserPerformOnProject(r.Context(), userInfo, secret.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnProject(r.Context(), userInfo, secret.ParentPublicID, auth.PermissionOwner)
		} else {
			canEdit = h.canUserPerformOnOrganization(r.Context(), userInfo, secret.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnOrganization(r.Context(), userInfo, secret.ParentPublicID, auth.PermissionOwner)
		}

		secrets = append(secrets, ResourceItem{
			ID:         secret.PublicID,
			Name:       secret.Name,
			Status:     "Active",
			ParentName: secret.ParentName,
			ParentID:   secret.ParentPublicID,
			ParentType: secret.ParentType,
			Permissions: ResourcePermissions{
				CanEdit:   canEdit,
				CanDelete: canDelete,
			},
		})
	}

	// Get settings with inheritance (includes org + project settings)
	dbSettings, err := h.db.ListUserSettings(ctx, db.ListUserSettingsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list settings", "project_id", projectID, "err", err)
	}

	settings := make([]Setting, 0)
	for _, setting := range dbSettings {
		// Include settings for this project OR its parent organization
		if setting.ParentPublicID != project.PublicID && setting.ParentPublicID != orgPublicID {
			continue
		}

		// Check permissions - use project or org depending on where the setting is
		var canEdit, canDelete bool
		if setting.ParentType == "project" {
			canEdit = h.canUserPerformOnProject(r.Context(), userInfo, setting.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnProject(r.Context(), userInfo, setting.ParentPublicID, auth.PermissionOwner)
		} else {
			canEdit = h.canUserPerformOnOrganization(r.Context(), userInfo, setting.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnOrganization(r.Context(), userInfo, setting.ParentPublicID, auth.PermissionOwner)
		}

		description := ""
		if setting.Description.Valid {
			description = setting.Description.String
		}

		editable := false
		if setting.Editable.Valid {
			editable = setting.Editable.Bool
		}

		settings = append(settings, Setting{
			ID:          setting.PublicID,
			Key:         setting.SettingKey,
			Value:       setting.SettingValue,
			Description: description,
			Editable:    editable,
			ParentName:  setting.ParentName,
			ParentID:    setting.ParentPublicID,
			ParentType:  setting.ParentType,
			Permissions: ResourcePermissions{
				CanEdit:   canEdit,
				CanDelete: canDelete,
			},
		})
	}

	// TODO: Get audit log entries
	auditLog := []AuditLogEntry{}

	projectStatus := ""
	if project.Status.Valid {
		projectStatus = string(project.Status.ProjectsStatus)
	}

	data := ProjectDetailData{
		Email:      account.Email,
		Name:       name,
		ActivePage: "projects",
		Project: ResourceItem{
			ID:          project.PublicID,
			Name:        project.Name,
			Description: "",
			Status:      projectStatus,
			CreatedAt:   project.CreatedAt.Time.Format("2006-01-02"),
			ParentID:    orgPublicID,
			ParentName:  org.Name,
			ParentType:  "organization",
		},
		Sites:         sites,
		Members:       members,
		FirewallRules: firewallRules,
		Secrets:       secrets,
		Settings:      settings,
		AuditLog:      auditLog,
	}

	RenderProjectDetail(w, data)
}

// HandleSiteDetail handles requests to individual site detail pages
func (h *Handler) HandleSiteDetail(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	siteID := r.PathValue("id")
	if siteID == "" {
		http.Error(w, "Site ID required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	// Get site details
	site, err := h.db.GetSite(ctx, siteID)
	if err != nil {
		slog.Error("Failed to get site", "site_id", siteID, "err", err)
		http.Error(w, "Site not found", http.StatusNotFound)
		return
	}

	// Get project to access org public ID
	project, err := h.db.GetProjectByID(ctx, site.ProjectID)
	if err != nil {
		slog.Error("Failed to get project for site", "site_id", siteID, "project_id", site.ProjectID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Get organization to access org public ID
	org, err := h.db.GetOrganizationByID(ctx, project.OrganizationID)
	if err != nil {
		slog.Error("Failed to get organization for site", "site_id", siteID, "org_id", project.OrganizationID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	projectPublicID := project.PublicID
	orgPublicID := org.PublicID

	// Get members with inheritance (includes org + project + site members)
	dbMemberships, err := h.db.ListUserMemberships(ctx, db.ListUserMembershipsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list memberships", "site_id", siteID, "err", err)
	}

	members := make([]Member, 0)
	for _, membership := range dbMemberships {
		// Include memberships for this site OR its parent project OR parent organization
		if membership.ParentPublicID != site.PublicID && membership.ParentPublicID != projectPublicID && membership.ParentPublicID != orgPublicID {
			continue
		}

		// Check permissions based on where the membership is
		var canEdit, canDelete bool
		switch membership.ParentType {
		case "site":
			canEdit = h.canUserPerformOnSite(r.Context(), userInfo, membership.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnSite(r.Context(), userInfo, membership.ParentPublicID, auth.PermissionOwner)
		case "project":
			canEdit = h.canUserPerformOnProject(r.Context(), userInfo, membership.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnProject(r.Context(), userInfo, membership.ParentPublicID, auth.PermissionOwner)
		case "organization":
			canEdit = h.canUserPerformOnOrganization(r.Context(), userInfo, membership.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnOrganization(r.Context(), userInfo, membership.ParentPublicID, auth.PermissionOwner)
		}

		members = append(members, Member{
			MemberID:   membership.AccountPublicID, // Use account_id for API endpoints
			Email:      membership.Email,
			Role:       string(membership.Role),
			ParentName: membership.ParentName,
			ParentID:   membership.ParentPublicID,
			ParentType: membership.ParentType,
			Permissions: ResourcePermissions{
				CanEdit:   canEdit,
				CanDelete: canDelete,
			},
		})
	}

	// Get firewall rules with inheritance (includes org + project + site rules)
	dbRules, err := h.db.ListUserFirewallRules(ctx, db.ListUserFirewallRulesParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list firewall rules", "site_id", siteID, "err", err)
	}

	firewallRules := make([]ResourceItem, 0)
	for _, rule := range dbRules {
		// Include rules for this site OR its parent project OR parent organization
		if rule.ParentPublicID != site.PublicID && rule.ParentPublicID != projectPublicID && rule.ParentPublicID != orgPublicID {
			continue
		}

		// Check permissions based on where the rule is
		var canEdit, canDelete bool
		switch rule.ParentType {
		case "site":
			canEdit = h.canUserPerformOnSite(r.Context(), userInfo, rule.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnSite(r.Context(), userInfo, rule.ParentPublicID, auth.PermissionOwner)
		case "project":
			canEdit = h.canUserPerformOnProject(r.Context(), userInfo, rule.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnProject(r.Context(), userInfo, rule.ParentPublicID, auth.PermissionOwner)
		case "organization":
			canEdit = h.canUserPerformOnOrganization(r.Context(), userInfo, rule.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnOrganization(r.Context(), userInfo, rule.ParentPublicID, auth.PermissionOwner)
		}

		createdAt := ""
		if rule.CreatedAt.Valid {
			createdAt = rule.CreatedAt.Time.Format("2006-01-02")
		}

		firewallRules = append(firewallRules, ResourceItem{
			ID:         rule.PublicID,
			Name:       rule.Name,
			Status:     "Active",
			CreatedAt:  createdAt,
			ParentName: rule.ParentName,
			ParentID:   rule.ParentPublicID,
			ParentType: rule.ParentType,
			Permissions: ResourcePermissions{
				CanEdit:   canEdit,
				CanDelete: canDelete,
			},
		})
	}

	// Get secrets with inheritance (includes org + project + site secrets)
	dbSecrets, err := h.db.ListUserSecrets(ctx, db.ListUserSecretsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list secrets", "site_id", siteID, "err", err)
	}

	secrets := make([]ResourceItem, 0)
	for _, secret := range dbSecrets {
		// Include secrets for this site OR its parent project OR parent organization
		if secret.ParentPublicID != site.PublicID && secret.ParentPublicID != projectPublicID && secret.ParentPublicID != orgPublicID {
			continue
		}

		// Check permissions based on where the secret is
		var canEdit, canDelete bool
		switch secret.ParentType {
		case "site":
			canEdit = h.canUserPerformOnSite(r.Context(), userInfo, secret.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnSite(r.Context(), userInfo, secret.ParentPublicID, auth.PermissionOwner)
		case "project":
			canEdit = h.canUserPerformOnProject(r.Context(), userInfo, secret.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnProject(r.Context(), userInfo, secret.ParentPublicID, auth.PermissionOwner)
		case "organization":
			canEdit = h.canUserPerformOnOrganization(r.Context(), userInfo, secret.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnOrganization(r.Context(), userInfo, secret.ParentPublicID, auth.PermissionOwner)
		}

		secrets = append(secrets, ResourceItem{
			ID:         secret.PublicID,
			Name:       secret.Name,
			Status:     "Active",
			ParentName: secret.ParentName,
			ParentID:   secret.ParentPublicID,
			ParentType: secret.ParentType,
			Permissions: ResourcePermissions{
				CanEdit:   canEdit,
				CanDelete: canDelete,
			},
		})
	}

	// Get settings with inheritance (includes org + project + site settings)
	dbSettings, err := h.db.ListUserSettings(ctx, db.ListUserSettingsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list settings", "site_id", siteID, "err", err)
	}

	settings := make([]Setting, 0)
	for _, setting := range dbSettings {
		// Include settings for this site OR its parent project OR parent organization
		if setting.ParentPublicID != site.PublicID && setting.ParentPublicID != projectPublicID && setting.ParentPublicID != orgPublicID {
			continue
		}

		// Check permissions based on where the setting is
		var canEdit, canDelete bool
		switch setting.ParentType {
		case "site":
			canEdit = h.canUserPerformOnSite(r.Context(), userInfo, setting.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnSite(r.Context(), userInfo, setting.ParentPublicID, auth.PermissionOwner)
		case "project":
			canEdit = h.canUserPerformOnProject(r.Context(), userInfo, setting.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnProject(r.Context(), userInfo, setting.ParentPublicID, auth.PermissionOwner)
		case "organization":
			canEdit = h.canUserPerformOnOrganization(r.Context(), userInfo, setting.ParentPublicID, auth.PermissionWrite)
			canDelete = h.canUserPerformOnOrganization(r.Context(), userInfo, setting.ParentPublicID, auth.PermissionOwner)
		}

		description := ""
		if setting.Description.Valid {
			description = setting.Description.String
		}

		editable := false
		if setting.Editable.Valid {
			editable = setting.Editable.Bool
		}

		settings = append(settings, Setting{
			ID:          setting.PublicID,
			Key:         setting.SettingKey,
			Value:       setting.SettingValue,
			Description: description,
			Editable:    editable,
			ParentName:  setting.ParentName,
			ParentID:    setting.ParentPublicID,
			ParentType:  setting.ParentType,
			Permissions: ResourcePermissions{
				CanEdit:   canEdit,
				CanDelete: canDelete,
			},
		})
	}

	// TODO: Get audit log entries
	auditLog := []AuditLogEntry{}

	status := ""
	if site.Status.Valid {
		status = string(site.Status.SitesStatus)
	}
	createdAt := ""
	if site.CreatedAt.Valid {
		createdAt = site.CreatedAt.Time.Format("2006-01-02")
	}

	data := SiteDetailData{
		Email:          account.Email,
		Name:           name,
		ActivePage:     "sites",
		OrganizationID: orgPublicID,
		ProjectID:      projectPublicID,
		Site: ResourceItem{
			ID:          site.PublicID,
			Name:        site.Name,
			Description: "",
			Status:      status,
			CreatedAt:   createdAt,
			ParentID:    projectPublicID,
		},
		Members:       members,
		FirewallRules: firewallRules,
		Secrets:       secrets,
		Settings:      settings,
		AuditLog:      auditLog,
	}

	RenderSiteDetail(w, data)
}

// HandleAPIKeys handles requests to the API keys page
func (h *Handler) HandleAPIKeys(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	ctx := context.Background()
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	data := APIKeysPageData{
		Email:      account.Email,
		Name:       name,
		ActivePage: "api-keys",
	}

	RenderAPIKeys(w, data)
}

// HandleSSHKeys handles requests to the SSH keys page
func (h *Handler) HandleSSHKeys(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	ctx := context.Background()
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	data := SSHKeysPageData{
		Email:      account.Email,
		Name:       name,
		AccountID:  account.PublicID,
		ActivePage: "ssh-keys",
	}

	RenderSSHKeys(w, data)
}

// HandleSettings handles requests to the settings page
func (h *Handler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	ctx := context.Background()
	account, err := h.db.GetAccountByID(ctx, userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "account_id", userInfo.AccountID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	// Fetch settings for the user
	dbSettings, err := h.db.ListUserSettings(ctx, db.ListUserSettingsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to list user settings", "account_id", account.ID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	items := make([]ResourceItem, 0, len(dbSettings))
	for _, setting := range dbSettings {
		description := ""
		if setting.Description.Valid {
			description = setting.Description.String
		}
		items = append(items, ResourceItem{
			ID:          setting.PublicID,
			Name:        setting.SettingKey,
			Description: description,
			Status:      "Active",
			CreatedAt:   "",
			ParentName:  setting.ParentName,
			ParentID:    setting.ParentPublicID,
			ParentType:  setting.ParentType,
		})
	}

	data := ResourcePageData{
		Email:        account.Email,
		Name:         name,
		ActivePage:   "settings",
		ResourceName: "Settings",
		Items:        items,
	}

	RenderSettings(w, data)
}
