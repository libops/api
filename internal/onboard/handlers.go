package onboard

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"

	"github.com/libops/api/db"
	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/billing"
	"github.com/libops/api/internal/config"
	"github.com/libops/api/internal/dash"
	"github.com/libops/api/internal/service/organization"
	"github.com/stripe/stripe-go/v84"
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

	// Get or create session
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

	// Organization is created via API call from frontend
	// Update session with org name, organization public ID, and advance to step 2
	organizationPublicID := ""
	var organizationID sql.NullInt64
	if req.OrganizationPublicID != "" {
		organizationPublicID = req.OrganizationPublicID

		// Look up the organization's internal ID
		org, err := h.db.GetOrganization(r.Context(), req.OrganizationPublicID)
		if err != nil {
			slog.Error("Failed to get organization by public ID", "error", err, "public_id", req.OrganizationPublicID)
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Failed to find organization"})
			return
		}
		organizationID = sql.NullInt64{Int64: org.ID, Valid: true}
	}

	err = h.db.UpdateOnboardingSession(r.Context(), db.UpdateOnboardingSessionParams{
		OrgName:                 sql.NullString{String: req.OrganizationName, Valid: true},
		OrgUuid:                 organizationPublicID,
		MachineType:             session.MachineType,
		MachinePriceID:          session.MachinePriceID,
		DiskSizeGb:              session.DiskSizeGb,
		StripeCheckoutSessionID: session.StripeCheckoutSessionID,
		StripeSubscriptionID:    session.StripeSubscriptionID,
		OrganizationID:          organizationID, // Set org ID from lookup
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
	// First-time onboarding always gets a 7-day trial
	checkoutResult, err := h.billingMgr.CreateCheckoutSession(r.Context(), account.Email, session.PublicID, req.MachineType, req.DiskSizeGB, h.baseURL, true)
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
		OrgUuid:                 getOrgPublicID(session.OrganizationPublicID),
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
		OrgUuid:                 getOrgPublicID(session.OrganizationPublicID),
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
		OrgUuid:                 getOrgPublicID(session.OrganizationPublicID),
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
		OrgUuid:                 getOrgPublicID(session.OrganizationPublicID),
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

	// Verify organization was created (by webhook or API)
	// Skip this check if billing is disabled (for testing)
	if !h.disableBilling && !session.OrganizationID.Valid {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Organization not yet created. Please wait for payment processing."})
		return
	}

	// Validate we have all required data (relaxed for testing when billing is disabled)
	if !h.disableBilling {
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
	}

	// Project, site, firewall rule, and SSH keys are created via API calls from frontend
	// Just update session as completed
	err = h.db.UpdateOnboardingSession(r.Context(), db.UpdateOnboardingSessionParams{
		OrgName:                 session.OrgName,
		OrgUuid:                 getOrgPublicID(session.OrganizationPublicID),
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
		"project_name", session.ProjectName.String,
		"site_name", session.SiteName.String)

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

	// Note: This webhook handler is not used. The actual Stripe webhooks are handled
	// by the billing service in internal/billing/webhook.go
	slog.Info("Received Stripe webhook event", "type", event.Type)
	w.WriteHeader(http.StatusOK)
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
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode JSON response", "error", err)
	}
}
