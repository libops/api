package onboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/billing"
	"github.com/libops/api/internal/config"
	"github.com/libops/api/internal/dash"
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/service/organization"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/subscriptionitem"
	"github.com/stripe/stripe-go/v84/webhook"
)

// Handler provides HTTP handlers for the onboarding flow
type Handler struct {
	db               db.Querier
	orgRepo          *organization.Repository
	config           *config.Config
	stripeKey        string
	stripeWebhookKey string
	baseURL          string
	sessionMgr       *SessionManager
	billingMgr       billing.Manager
	disableBilling   bool
}

// NewHandler creates a new onboarding handler
// Deprecated: Use NewHandlerWithConfig instead to support billing configuration
func NewHandler(querier db.Querier, stripeKey, stripeWebhookKey, baseURL string) *Handler {
	// Set the Stripe API key
	stripe.Key = stripeKey

	return &Handler{
		db:               querier,
		orgRepo:          organization.NewRepository(querier),
		config:           nil, // Config not provided in deprecated constructor
		stripeKey:        stripeKey,
		stripeWebhookKey: stripeWebhookKey,
		baseURL:          baseURL,
		sessionMgr:       NewSessionManager(querier),
		billingMgr:       billing.NewStripeManager(querier),
		disableBilling:   false,
	}
}

// NewHandlerWithConfig creates a new onboarding handler with billing and organization configuration
func NewHandlerWithConfig(querier db.Querier, cfg *config.Config, stripeKey, stripeWebhookKey, baseURL string, disableBilling bool) *Handler {
	var billingMgr billing.Manager
	if disableBilling {
		billingMgr = billing.NewNoOpBillingManager()
	} else {
		// Set the Stripe API key for production
		stripe.Key = stripeKey
		billingMgr = billing.NewStripeManager(querier)
	}

	return &Handler{
		db:               querier,
		orgRepo:          organization.NewRepository(querier),
		config:           cfg,
		stripeKey:        stripeKey,
		stripeWebhookKey: stripeWebhookKey,
		baseURL:          baseURL,
		sessionMgr:       NewSessionManager(querier),
		billingMgr:       billingMgr,
		disableBilling:   disableBilling,
	}
}

// RenderOnboarding renders the onboarding page
func (h *Handler) RenderOnboarding(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Get account
	account, err := h.db.GetAccountByID(r.Context(), userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Render the onboarding template
	dash.RenderTemplate(w, "onboarding.html", map[string]any{
		"Email": account.Email,
	})
}

// HandleGetSession returns the current onboarding session state
func (h *Handler) HandleGetSession(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	session, err := h.sessionMgr.GetOrCreateSession(r.Context(), userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get onboarding session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get session"})
		return
	}

	writeJSON(w, http.StatusOK, ToResponse(session))
}

// HandleStep1 handles step 1: organization name
func (h *Handler) HandleStep1(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req Step1Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request"})
		return
	}

	if req.OrganizationName == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Organization name is required"})
		return
	}

	// Get or create session
	session, err := h.sessionMgr.GetOrCreateSession(r.Context(), userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get session"})
		return
	}

	// Create organization immediately
	var organizationID = session.OrganizationID
	if !organizationID.Valid {
		orgID, err := h.createOrganization(r.Context(), userInfo.AccountID, req.OrganizationName)
		if err != nil {
			slog.Error("Failed to create organization", "error", err)
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to create organization"})
			return
		}
		organizationID = sql.NullInt64{Int64: orgID, Valid: true}
		slog.Info("Created organization for onboarding", "org_id", orgID, "org_name", req.OrganizationName)
	}

	// Update session with org name, org ID, and advance to step 2
	err = h.db.UpdateOnboardingSession(r.Context(), db.UpdateOnboardingSessionParams{
		OrgName:                 sql.NullString{String: req.OrganizationName, Valid: true},
		MachineType:             session.MachineType,
		MachinePriceID:          session.MachinePriceID,
		DiskSizeGb:              session.DiskSizeGb,
		StripeCheckoutSessionID: session.StripeCheckoutSessionID,
		StripeSubscriptionID:    session.StripeSubscriptionID,
		OrganizationID:          organizationID,
		ProjectName:             session.ProjectName,
		GcpCountry:              session.GcpCountry,
		GcpRegion:               session.GcpRegion,
		SiteName:                session.SiteName,
		GithubRepoUrl:           session.GithubRepoUrl,
		Port:                    session.Port,
		FirewallIp:              session.FirewallIp,
		CurrentStep:             sql.NullInt32{Int32: 2, Valid: true},
		Completed:               sql.NullBool{Bool: false, Valid: true},
		ID:                      session.ID,
	})

	if err != nil {
		slog.Error("Failed to update session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update session"})
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{Message: "Step 1 completed"})
}

// HandleStep2 handles step 2: create Stripe checkout session
func (h *Handler) HandleStep2(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req Step2Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request"})
		return
	}

	// Validate machine type using billing manager
	if err := h.billingMgr.ValidateMachineType(r.Context(), req.MachineType); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid machine type"})
		return
	}

	// Validate disk size using billing manager
	if err := h.billingMgr.ValidateDiskSize(r.Context(), req.DiskSizeGB); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Get session
	session, err := h.sessionMgr.GetOrCreateSession(r.Context(), userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get session"})
		return
	}

	// Get account for email
	account, err := h.db.GetAccountByID(r.Context(), userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get account", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get account"})
		return
	}

	// Create Stripe checkout session using billing manager
	checkoutResult, err := h.billingMgr.CreateCheckoutSession(r.Context(), account.Email, session.PublicID, req.MachineType, req.DiskSizeGB, h.baseURL)
	if err != nil {
		slog.Error("Failed to create checkout session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to create checkout session"})
		return
	}

	// Get machine price ID from database for storage
	machinePriceID, err := h.billingMgr.GetMachineTypePriceID(r.Context(), req.MachineType)
	if err != nil {
		slog.Error("Failed to get machine price ID", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get machine price ID"})
		return
	}

	// Determine next step: if billing is disabled or no checkout URL, skip to step 4
	var nextStep int32 = 3 // Default: go to Stripe checkout
	if h.disableBilling || checkoutResult.URL == "" {
		nextStep = 4 // Skip Stripe, go directly to step 4
		slog.Info("Skipping Stripe checkout - billing disabled", "next_step", nextStep)
	}

	// Update session with machine type, disk size, and checkout info
	err = h.db.UpdateOnboardingSession(r.Context(), db.UpdateOnboardingSessionParams{
		OrgName:                 session.OrgName,
		MachineType:             sql.NullString{String: req.MachineType, Valid: true},
		MachinePriceID:          sql.NullString{String: machinePriceID, Valid: true},
		DiskSizeGb:              sql.NullInt32{Int32: int32(req.DiskSizeGB), Valid: true},
		StripeCheckoutSessionID: sql.NullString{String: checkoutResult.SessionID, Valid: true},
		StripeCheckoutUrl:       sql.NullString{String: checkoutResult.URL, Valid: true},
		StripeSubscriptionID:    session.StripeSubscriptionID,
		OrganizationID:          session.OrganizationID,
		ProjectName:             session.ProjectName,
		GcpCountry:              session.GcpCountry,
		GcpRegion:               session.GcpRegion,
		SiteName:                session.SiteName,
		GithubRepoUrl:           session.GithubRepoUrl,
		Port:                    session.Port,
		FirewallIp:              session.FirewallIp,
		CurrentStep:             sql.NullInt32{Int32: nextStep, Valid: true},
		Completed:               sql.NullBool{Bool: false, Valid: true},
		ID:                      session.ID,
	})

	if err != nil {
		slog.Error("Failed to update session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update session"})
		return
	}

	writeJSON(w, http.StatusOK, StripeCheckoutResponse{
		CheckoutURL: checkoutResult.URL,
		SkipBilling: h.disableBilling,
		NextStep:    nextStep,
	})
}

// createOrganization creates an organization using the organization service's shared logic
// This ensures consistent behavior with the organization service (adds owner member and relationship)
// Returns the organization ID
func (h *Handler) createOrganization(ctx context.Context, accountID int64, orgName string) (int64, error) {
	orgPublicID := uuid.New().String()

	// Get config values (fallback to empty if config not set)
	var gcpOrgID, gcpBillingAccount, gcpParent string
	var rootOrgID int64
	if h.config != nil {
		gcpOrgID = h.config.GcpOrgID
		gcpBillingAccount = h.config.GcpBillingAccount
		gcpParent = h.config.GcpParent
		rootOrgID = h.config.RootOrganizationID
	}

	// Use shared repository method that creates org, adds owner with active status, and creates relationship
	orgID, err := h.orgRepo.CreateOrganizationWithOwner(
		ctx,
		orgPublicID,
		orgName,
		gcpOrgID,
		gcpBillingAccount,
		gcpParent,
		accountID,
		rootOrgID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create organization: %w", err)
	}

	return orgID, nil
}

// HandleStripeSuccess handles the return from successful Stripe checkout
func (h *Handler) HandleStripeSuccess(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Redirect(w, r, "/onboarding?error=missing_session_id", http.StatusSeeOther)
		return
	}

	// For now, just redirect back to onboarding
	// The webhook will handle creating the organization
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

// HandleStripeCancel handles the return from cancelled Stripe checkout
func (h *Handler) HandleStripeCancel(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/onboarding?step=2&error=checkout_cancelled", http.StatusSeeOther)
}

// HandleStep4 handles step 4: project name
func (h *Handler) HandleStep4(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req Step4Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request"})
		return
	}

	if req.ProjectName == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Project name is required"})
		return
	}

	session, err := h.sessionMgr.GetOrCreateSession(r.Context(), userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get session"})
		return
	}

	err = h.db.UpdateOnboardingSession(r.Context(), db.UpdateOnboardingSessionParams{
		OrgName:                 session.OrgName,
		MachineType:             session.MachineType,
		MachinePriceID:          session.MachinePriceID,
		DiskSizeGb:              session.DiskSizeGb,
		StripeCheckoutSessionID: session.StripeCheckoutSessionID,
		StripeSubscriptionID:    session.StripeSubscriptionID,
		OrganizationID:          session.OrganizationID,
		ProjectName:             sql.NullString{String: req.ProjectName, Valid: true},
		GcpCountry:              session.GcpCountry,
		GcpRegion:               session.GcpRegion,
		SiteName:                session.SiteName,
		GithubRepoUrl:           session.GithubRepoUrl,
		Port:                    session.Port,
		FirewallIp:              session.FirewallIp,
		CurrentStep:             sql.NullInt32{Int32: 5, Valid: true},
		Completed:               sql.NullBool{Bool: false, Valid: true},
		ID:                      session.ID,
	})

	if err != nil {
		slog.Error("Failed to update session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update session"})
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{Message: "Step 4 completed"})
}

// HandleStep5 handles step 5: GCP region selection
func (h *Handler) HandleStep5(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req Step5Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request"})
		return
	}

	if req.Country == "" || req.Region == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Country and region are required"})
		return
	}

	// Validate region for country
	if !ValidateRegion(req.Country, req.Region) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid region for selected country"})
		return
	}

	session, err := h.sessionMgr.GetOrCreateSession(r.Context(), userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get session"})
		return
	}

	err = h.db.UpdateOnboardingSession(r.Context(), db.UpdateOnboardingSessionParams{
		OrgName:                 session.OrgName,
		MachineType:             session.MachineType,
		MachinePriceID:          session.MachinePriceID,
		DiskSizeGb:              session.DiskSizeGb,
		StripeCheckoutSessionID: session.StripeCheckoutSessionID,
		StripeSubscriptionID:    session.StripeSubscriptionID,
		OrganizationID:          session.OrganizationID,
		ProjectName:             session.ProjectName,
		GcpCountry:              sql.NullString{String: req.Country, Valid: true},
		GcpRegion:               sql.NullString{String: req.Region, Valid: true},
		SiteName:                session.SiteName,
		GithubRepoUrl:           session.GithubRepoUrl,
		Port:                    session.Port,
		FirewallIp:              session.FirewallIp,
		CurrentStep:             sql.NullInt32{Int32: 6, Valid: true},
		Completed:               sql.NullBool{Bool: false, Valid: true},
		ID:                      session.ID,
	})

	if err != nil {
		slog.Error("Failed to update session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update session"})
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{Message: "Step 5 completed"})
}

// HandleStep6 handles step 6: Site name and GitHub repository selection
func (h *Handler) HandleStep6(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req Step6Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request"})
		return
	}

	if req.SiteName == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Site name is required"})
		return
	}

	// Default port to 80 if not provided
	if req.Port == 0 {
		req.Port = 80
	}

	// Validate port range
	if req.Port < 1 || req.Port > 65535 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Port must be between 1 and 65535"})
		return
	}

	var repoURL string
	switch req.RepoOption {
	case "ojs":
		repoURL = TemplateOJS
	case "isle-site-template":
		repoURL = TemplateIsleSite
	case "custom":
		if req.CustomURL == "" {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Custom URL is required"})
			return
		}
		repoURL = req.CustomURL
	case "new-from-template":
		// User will create repo manually and come back
		repoURL = ""
	default:
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid repository option"})
		return
	}

	session, err := h.sessionMgr.GetOrCreateSession(r.Context(), userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get session"})
		return
	}

	err = h.db.UpdateOnboardingSession(r.Context(), db.UpdateOnboardingSessionParams{
		OrgName:                 session.OrgName,
		MachineType:             session.MachineType,
		MachinePriceID:          session.MachinePriceID,
		DiskSizeGb:              session.DiskSizeGb,
		StripeCheckoutSessionID: session.StripeCheckoutSessionID,
		StripeSubscriptionID:    session.StripeSubscriptionID,
		OrganizationID:          session.OrganizationID,
		ProjectName:             session.ProjectName,
		GcpCountry:              session.GcpCountry,
		GcpRegion:               session.GcpRegion,
		SiteName:                sql.NullString{String: req.SiteName, Valid: true},
		GithubRepoUrl:           sql.NullString{String: repoURL, Valid: repoURL != ""},
		Port:                    sql.NullInt32{Int32: int32(req.Port), Valid: true},
		FirewallIp:              session.FirewallIp,
		CurrentStep:             sql.NullInt32{Int32: 7, Valid: true},
		Completed:               sql.NullBool{Bool: false, Valid: true},
		ID:                      session.ID,
	})

	if err != nil {
		slog.Error("Failed to update session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to update session"})
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{Message: "Step 6 completed"})
}

// HandleStep7 handles step 7: firewall IP and completes onboarding
// Creates project, site, and firewall resources
func (h *Handler) HandleStep7(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req Step7Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request"})
		return
	}

	if req.FirewallIP == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Firewall IP is required"})
		return
	}

	session, err := h.sessionMgr.GetOrCreateSession(r.Context(), userInfo.AccountID)
	if err != nil {
		slog.Error("Failed to get session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get session"})
		return
	}

	// Verify organization was created (by webhook)
	if !session.OrganizationID.Valid {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Organization not yet created. Please wait for payment processing."})
		return
	}

	// Validate we have all required data
	if !session.ProjectName.Valid || !session.MachineType.Valid || !session.DiskSizeGb.Valid {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Missing project configuration"})
		return
	}

	if !session.GcpRegion.Valid {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Missing region configuration"})
		return
	}

	if !session.SiteName.Valid || !session.Port.Valid {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Missing site configuration"})
		return
	}

	// Get the organization's Stripe subscription to find machine item ID (if billing enabled)
	machineItemID := ""
	if !h.disableBilling {
		subscription, err := h.db.GetStripeSubscriptionByOrganizationID(r.Context(), session.OrganizationID.Int64)
		if err != nil {
			slog.Error("Failed to get subscription", "error", err)
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to get subscription"})
			return
		}

		// Get the machine subscription item ID that was stored in the webhook
		// The MachinePriceID field is reused to store the subscription item ID
		if session.MachinePriceID.Valid {
			machineItemID = session.MachinePriceID.String
			slog.Info("Using machine subscription item ID from webhook", "item_id", machineItemID)
		} else {
			// Fallback: try to find it from Stripe if webhook didn't store it
			machineItemID, err = h.findMachineSubscriptionItem(r.Context(), subscription.StripeSubscriptionID, session.MachineType.String)
			if err != nil {
				slog.Error("Failed to find machine subscription item", "error", err)
				// Don't fail - we can still create the project without the item ID
				machineItemID = ""
			}
		}

		_ = subscription // Use subscription if needed later
	} else {
		slog.Info("Billing disabled - skipping subscription check")
	}

	// Create project
	projectPublicID := uuid.New().String()
	err = h.db.CreateProject(r.Context(), db.CreateProjectParams{
		PublicID:                  projectPublicID,
		OrganizationID:            session.OrganizationID.Int64,
		Name:                      session.ProjectName.String,
		GcpRegion:                 sql.NullString{String: session.GcpRegion.String, Valid: true},
		GcpZone:                   sql.NullString{String: session.GcpRegion.String + "-a", Valid: true}, // Default to zone -a
		MachineType:               sql.NullString{String: session.MachineType.String, Valid: true},
		DiskSizeGb:                session.DiskSizeGb,
		StripeSubscriptionItemID:  sql.NullString{String: machineItemID, Valid: machineItemID != ""},
		MonitoringEnabled:         sql.NullBool{Bool: false, Valid: true},
		MonitoringLogLevel:        sql.NullString{String: "INFO", Valid: true},
		MonitoringMetricsEnabled:  sql.NullBool{Bool: false, Valid: true},
		MonitoringHealthCheckPath: sql.NullString{String: "/", Valid: true},
		GcpProjectID:              sql.NullString{Valid: false}, // Will be set by terraform
		GcpProjectNumber:          sql.NullString{Valid: false},
		CreateBranchSites:         sql.NullBool{Bool: false, Valid: true},
		Status:                    db.NullProjectsStatus{ProjectsStatus: db.ProjectsStatusProvisioning, Valid: true},
		CreatedBy:                 sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
		UpdatedBy:                 sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})

	if err != nil {
		slog.Error("Failed to create project", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to create project"})
		return
	}

	// Get project ID
	project, err := h.db.GetProject(r.Context(), projectPublicID)
	if err != nil {
		slog.Error("Failed to get project", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to create project"})
		return
	}

	// Create site
	githubRepo := session.GithubRepoUrl.String
	if githubRepo == "" {
		githubRepo = "https://github.com/libops/isle-site-template" // Default
	}

	err = h.db.CreateSite(r.Context(), db.CreateSiteParams{
		ProjectID:        project.ID,
		Name:             session.SiteName.String,
		GithubRepository: githubRepo,
		GithubRef:        "heads/main",
		GithubTeamID:     sql.NullString{Valid: false},
		ComposePath:      sql.NullString{String: "/mnt/disks/data/compose", Valid: true},
		ComposeFile:      sql.NullString{String: "docker-compose.yml", Valid: true},
		Port:             session.Port,
		ApplicationType:  sql.NullString{String: "generic", Valid: true},
		UpCmd:            json.RawMessage(`["docker compose up --remove-orphans -d"]`),
		InitCmd:          json.RawMessage(`[]`),
		RolloutCmd:       json.RawMessage(`["docker compose pull", "docker compose up --remove-orphans -d"]`),
		GcpExternalIp:    sql.NullString{Valid: false}, // Will be set by terraform
		Status:           db.NullSitesStatus{SitesStatus: db.SitesStatusProvisioning, Valid: true},
		CreatedBy:        sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
		UpdatedBy:        sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})

	if err != nil {
		slog.Error("Failed to create site", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to create site"})
		return
	}

	// Create organization firewall rule
	err = h.db.CreateOrganizationFirewallRule(r.Context(), db.CreateOrganizationFirewallRuleParams{
		OrganizationID: sql.NullInt64{Int64: session.OrganizationID.Int64, Valid: true},
		Name:           "Onboarding IP",
		RuleType:       db.OrganizationFirewallRulesRuleTypeHttpsAllowed,
		Cidr:           req.FirewallIP + "/32",
		CreatedBy:      sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
		UpdatedBy:      sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})

	if err != nil {
		slog.Error("Failed to create firewall rule", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to create firewall rule"})
		return
	}

	// Update session as completed
	err = h.db.UpdateOnboardingSession(r.Context(), db.UpdateOnboardingSessionParams{
		OrgName:                 session.OrgName,
		MachineType:             session.MachineType,
		MachinePriceID:          session.MachinePriceID,
		DiskSizeGb:              session.DiskSizeGb,
		StripeCheckoutSessionID: session.StripeCheckoutSessionID,
		StripeSubscriptionID:    session.StripeSubscriptionID,
		OrganizationID:          session.OrganizationID,
		ProjectName:             session.ProjectName,
		GcpCountry:              session.GcpCountry,
		GcpRegion:               session.GcpRegion,
		SiteName:                session.SiteName,
		GithubRepoUrl:           session.GithubRepoUrl,
		Port:                    session.Port,
		FirewallIp:              sql.NullString{String: req.FirewallIP, Valid: true},
		CurrentStep:             sql.NullInt32{Int32: 7, Valid: true},
		Completed:               sql.NullBool{Bool: true, Valid: true},
		ID:                      session.ID,
	})

	if err != nil {
		slog.Error("Failed to update session", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to complete onboarding"})
		return
	}

	// Mark account as onboarding complete
	err = h.db.UpdateAccountOnboarding(r.Context(), db.UpdateAccountOnboardingParams{
		OnboardingCompleted: true,
		OnboardingSessionID: sql.NullString{String: session.PublicID, Valid: true},
		ID:                  userInfo.AccountID,
	})

	if err != nil {
		slog.Error("Failed to update account", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to complete onboarding"})
		return
	}

	slog.Info("Onboarding completed successfully",
		"account_id", userInfo.AccountID,
		"organization_id", session.OrganizationID.Int64,
		"project_id", project.ID,
		"project_name", session.ProjectName.String)

	writeJSON(w, http.StatusOK, SuccessResponse{Message: "Onboarding completed successfully"})
}

// HandleStripeWebhook handles Stripe webhook events
func (h *Handler) HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Failed to read webhook payload", "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	event, err := webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), h.stripeWebhookKey)
	if err != nil {
		slog.Error("Failed to verify webhook signature", "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			slog.Error("Failed to parse checkout session", "error", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		if err := h.handleCheckoutSessionCompleted(r.Context(), &session); err != nil {
			slog.Error("Failed to handle checkout session completed", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

	default:
		slog.Info("Unhandled webhook event type", "type", event.Type)
	}

	w.WriteHeader(http.StatusOK)
}

// handleCheckoutSessionCompleted creates the organization after successful payment
func (h *Handler) handleCheckoutSessionCompleted(ctx context.Context, session *stripe.CheckoutSession) error {
	// Get onboarding session by client reference ID (onboarding session public ID)
	onboardSession, err := h.db.GetOnboardingSession(ctx, session.ClientReferenceID)
	if err != nil {
		return fmt.Errorf("failed to get onboarding session: %w", err)
	}

	// Check if organization already created
	var orgID int64
	if onboardSession.OrganizationID.Valid {
		slog.Info("Organization already created for this session", "session_id", onboardSession.PublicID)
		orgID = onboardSession.OrganizationID.Int64
	} else {
		// Create organization using helper
		var err error
		orgID, err = h.createOrganization(ctx, onboardSession.AccountID, onboardSession.OrgName.String)
		if err != nil {
			return fmt.Errorf("failed to create organization: %w", err)
		}
	}

	// Get organization for further processing
	org, err := h.db.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return fmt.Errorf("failed to get organization: %w", err)
	}

	// Create Stripe subscription record
	subscriptionID := session.Subscription.ID
	customerID := session.Customer.ID

	var trialStart, trialEnd sql.NullTime
	if session.Subscription != nil {
		if session.Subscription.TrialStart != 0 {
			trialStart = sql.NullTime{Time: time.Unix(session.Subscription.TrialStart, 0), Valid: true}
		}
		if session.Subscription.TrialEnd != 0 {
			trialEnd = sql.NullTime{Time: time.Unix(session.Subscription.TrialEnd, 0), Valid: true}
		}
	}

	_, err = h.db.CreateStripeSubscription(ctx, db.CreateStripeSubscriptionParams{
		OrganizationID:          org.ID,
		StripeSubscriptionID:    subscriptionID,
		StripeCustomerID:        customerID,
		StripeCheckoutSessionID: sql.NullString{String: session.ID, Valid: true},
		Status:                  db.StripeSubscriptionsStatusTrialing,
		CurrentPeriodStart:      sql.NullTime{Valid: false},
		CurrentPeriodEnd:        sql.NullTime{Valid: false},
		TrialStart:              trialStart,
		TrialEnd:                trialEnd,
		CancelAtPeriodEnd:       sql.NullBool{Bool: false, Valid: true},
		CanceledAt:              sql.NullTime{Valid: false},
		MachineType:             onboardSession.MachineType,
		DiskSizeGb:              onboardSession.DiskSizeGb,
	})

	if err != nil {
		return fmt.Errorf("failed to create stripe subscription: %w", err)
	}

	// Update onboarding session with organization ID and subscription ID
	err = h.db.UpdateOnboardingSession(ctx, db.UpdateOnboardingSessionParams{
		OrgName:                 onboardSession.OrgName,
		MachineType:             onboardSession.MachineType,
		MachinePriceID:          onboardSession.MachinePriceID,
		DiskSizeGb:              onboardSession.DiskSizeGb,
		StripeCheckoutSessionID: sql.NullString{String: session.ID, Valid: true},
		StripeSubscriptionID:    sql.NullString{String: subscriptionID, Valid: true},
		OrganizationID:          sql.NullInt64{Int64: org.ID, Valid: true},
		ProjectName:             onboardSession.ProjectName,
		GcpCountry:              onboardSession.GcpCountry,
		GcpRegion:               onboardSession.GcpRegion,
		SiteName:                onboardSession.SiteName,
		GithubRepoUrl:           onboardSession.GithubRepoUrl,
		Port:                    onboardSession.Port,
		FirewallIp:              onboardSession.FirewallIp,
		CurrentStep:             sql.NullInt32{Int32: 4, Valid: true}, // Move to step 4 (project configuration)
		Completed:               sql.NullBool{Bool: false, Valid: true},
		ID:                      onboardSession.ID,
	})

	if err != nil {
		return fmt.Errorf("failed to update onboarding session: %w", err)
	}

	slog.Info("Organization created successfully from Stripe checkout",
		"organization_id", org.PublicID,
		"subscription_id", subscriptionID,
		"account_id", onboardSession.AccountID)

	return nil
}

// HandleGetClientIP returns the client's IP address
func (h *Handler) HandleGetClientIP(w http.ResponseWriter, r *http.Request) {
	ip := getClientIP(r)
	writeJSON(w, http.StatusOK, map[string]string{"ip": ip})
}

// HandleGetRegions returns available regions for a country
func (h *Handler) HandleGetRegions(w http.ResponseWriter, r *http.Request) {
	country := r.URL.Query().Get("country")
	if country == "" {
		// Return all mappings
		writeJSON(w, http.StatusOK, GetRegionMappings())
		return
	}

	// Return regions for specific country
	regions := GetRegionsByCountry(country)
	writeJSON(w, http.StatusOK, regions)
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Try X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take first IP if multiple
		if idx := len(xff); idx > 0 {
			if commaIdx := 0; commaIdx < idx {
				for i, c := range xff {
					if c == ',' {
						return xff[:i]
					}
				}
			}
			return xff
		}
	}

	// Try X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// findMachineSubscriptionItem finds the machine subscription item in a Stripe subscription
func (h *Handler) findMachineSubscriptionItem(ctx context.Context, subscriptionID, machineType string) (string, error) {
	machinePriceID, err := h.billingMgr.GetMachineTypePriceID(ctx, machineType)
	if err != nil {
		return "", fmt.Errorf("invalid machine type: %s", machineType)
	}

	params := &stripe.SubscriptionItemListParams{
		Subscription: stripe.String(subscriptionID),
	}

	iter := subscriptionitem.List(params)
	for iter.Next() {
		item := iter.SubscriptionItem()
		// Find the item with matching price ID
		if item.Price.ID == machinePriceID {
			return item.ID, nil
		}
	}

	if err := iter.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("machine subscription item not found for machine type: %s", machineType)
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode JSON response", "error", err)
	}
}
