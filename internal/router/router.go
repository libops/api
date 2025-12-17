// Package router sets up and configures the HTTP router and all API endpoints.
package router

import (
	"log/slog"
	"net/http"
	"os"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/otelconnect"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/time/rate"

	"github.com/libops/api/internal/audit"
	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/billing"
	"github.com/libops/api/internal/config"
	"github.com/libops/api/internal/dash"
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/events"
	"github.com/libops/api/internal/middleware"
	"github.com/libops/api/internal/onboard"
	"github.com/libops/api/internal/service/account"
	"github.com/libops/api/internal/service/organization"
	"github.com/libops/api/internal/service/project"
	"github.com/libops/api/internal/service/site"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// Dependencies holds all the dependencies needed to create routes.
type Dependencies struct {
	Config            *config.Config
	Queries           db.Querier
	Emitter           *events.Emitter
	Authorizer        *auth.Authorizer
	JWTValidator      auth.JWTValidator
	LibopsTokenIssuer *auth.LibopsTokenIssuer
	APIKeyManager     *auth.APIKeyManager
	AuthHandler       *auth.Handler
	UserpassClient    *auth.UserpassClient
	SessionManager    *auth.SessionManager
	AllowedOrigins    []string
}

// New creates a new HTTP handler with all routes configured.
func New(deps *Dependencies) http.Handler {
	mux := http.NewServeMux()

	// Per-route rate limiters (these apply IN ADDITION to the global limiter)
	// The global limiter (300 req/min) will be applied to all routes in the middleware chain
	// These per-route limiters add stricter limits where needed
	authLimiter := NewRateLimiter(rate.Limit(20), 50) // 20 rps, burst 50 (auth endpoints)

	accountService := account.NewAccountService(deps.Queries, deps.APIKeyManager)
	adminAccountService := account.NewAdminAccountService(deps.Queries, deps.Emitter)

	organizationService := organization.NewOrganizationService(deps.Queries, deps.Config)
	adminOrganizationService := organization.NewAdminOrganizationService(deps.Queries)
	memberService := organization.NewMemberService(deps.Queries)
	firewallService := organization.NewFirewallService(deps.Queries)
	sshKeyService := organization.NewSshKeyService(deps.Queries)

	projectService := project.NewProjectServiceWithConfig(deps.Queries, deps.Config.DisableBilling)
	adminProjectService := project.NewAdminProjectServiceWithConfig(deps.Queries, deps.Config.DisableBilling)
	projectMemberService := project.NewProjectMemberService(deps.Queries)
	projectFirewallService := project.NewProjectFirewallService(deps.Queries)

	siteService := site.NewSiteService(deps.Queries)
	adminSiteService := site.NewAdminSiteService(deps.Queries)
	siteMemberService := site.NewSiteMemberService(deps.Queries)
	siteFirewallService := site.NewSiteFirewallService(deps.Queries)
	siteOpsService := site.NewSiteOperationsService(deps.Queries)

	var interceptors []connect.Interceptor

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		slog.Error("Failed to create OpenTelemetry interceptor", "err", err)
	} else {
		interceptors = append(interceptors, otelInterceptor)
	}

	auditLogger := audit.New(deps.Queries)

	organizationSecretService := organization.NewOrganizationSecretService(deps.Queries, auditLogger)
	projectSecretService := project.NewProjectSecretService(deps.Queries, auditLogger)
	siteSecretService := site.NewSiteSecretService(deps.Queries, auditLogger)
	auditInterceptor := audit.NewAuditInterceptor(auditLogger, auth.ExtractAccountIDFromContext)
	interceptors = append(interceptors, auditInterceptor)

	accountLookupRateLimiter := NewRateLimiter(10, 20)

	if deps.Authorizer != nil {
		// First interceptor: Check if scope matches exactly (for API keys)
		scopeAuthzInterceptor := auth.NewScopeAuthzInterceptor(deps.Authorizer, auditLogger)
		interceptors = append(interceptors, scopeAuthzInterceptor)

		// Second interceptor: Check RBAC based on hierarchical permissions
		rbacAuthzInterceptor := auth.NewRBACAuthzInterceptor(deps.Authorizer, auditLogger)
		interceptors = append(interceptors, rbacAuthzInterceptor)
	}

	var handlerOptions []connect.HandlerOption
	handlerOptions = append(handlerOptions, connect.WithInterceptors(interceptors...))

	registerConnectServices(mux, handlerOptions, accountLookupRateLimiter,
		organizationService,
		adminOrganizationService,
		projectService,
		adminProjectService,
		siteService,
		adminSiteService,
		accountService,
		adminAccountService,
		memberService,
		siteOpsService,
		sshKeyService,
		firewallService,
		projectFirewallService,
		siteFirewallService,
		projectMemberService,
		siteMemberService,
		organizationSecretService,
		projectSecretService,
		siteSecretService,
	)

	registerReflection(mux)

	registerUtilityRoutes(mux)

	// Register controller routes for SSH keys endpoints (GSA-authenticated)
	registerControllerRoutes(mux, deps.Queries, adminSiteService, adminProjectService)

	// Register onboarding routes and middleware
	onboardHandler := onboard.NewHandlerWithConfig(deps.Queries, deps.Config, deps.Config.StripeSecretKey, deps.Config.StripeWebhookSecret, deps.Config.DashBaseUrl, deps.Config.DisableBilling)
	onboardMiddleware := onboard.NewMiddleware(deps.Queries)

	// Create billing manager for webhook handling
	stripeMgr := billing.NewStripeManagerWithWebhook(deps.Queries, deps.Config.StripeWebhookSecret, deps.Config.StripeSecretKey)

	registerOnboardingRoutes(mux, onboardHandler, stripeMgr)

	// Register dashboard routes
	dashHandler := dash.NewHandler(deps.Queries, deps.SessionManager)
	registerDashboardRoutes(mux, dashHandler, onboardMiddleware)

	if deps.AuthHandler != nil {
		registerAuthRoutes(mux, deps.AuthHandler)
	}
	if deps.LibopsTokenIssuer != nil {
		// Token endpoint
		mux.Handle("POST /auth/token", authLimiter.LimitByIP(http.HandlerFunc(deps.LibopsTokenIssuer.HandleToken)))
	}

	if deps.UserpassClient != nil {
		registerUserpassRoutes(mux, deps.UserpassClient, deps.AuthHandler, authLimiter) // Userpass auth routes
	}

	// Create global rate limiter (100 requests per minute per IP)
	globalRateLimiter := NewRateLimiter(rate.Limit(300), 300)

	var handler http.Handler = mux

	// Apply request ID middleware first
	handler = middleware.RequestIDMiddleware(handler)

	// Apply Connect GET defaults (encoding=proto, message=)
	handler = middleware.ConnectGetDefaultsMiddleware(handler)

	// Apply global rate limiter
	handler = globalRateLimiter.LimitByIP(handler)

	// Apply security headers
	handler = middleware.SecurityHeadersMiddleware(handler)

	// Apply CSRF protection
	handler = middleware.CSRFMiddleware(handler)

	// Validate JWT or API Key
	if deps.JWTValidator != nil {
		handler = deps.JWTValidator.Middleware(handler)
	}

	// Log all HTTP requests with status codes
	handler = middleware.AccessLogger(handler)

	// Apply CORS
	handler = middleware.CorsMiddleware(handler, deps.AllowedOrigins)

	// Add OpenTelemetry instrumentation
	handler = otelhttp.NewHandler(handler, "libops-api")

	// Enable h2c for gRPC-Web
	handler = h2c.NewHandler(handler, &http2.Server{})

	return handler
}

// registerConnectServices registers all ConnectRPC service handlers.
func registerConnectServices(
	mux *http.ServeMux,
	opts []connect.HandlerOption,
	accountLookupRateLimiter *RateLimiter,
	organizationService *organization.OrganizationService,
	adminOrganizationService *organization.AdminOrganizationService,
	projectService *project.ProjectService,
	adminProjectService *project.AdminProjectService,
	siteService *site.SiteService,
	adminSiteService *site.AdminSiteService,
	accountService *account.AccountService,
	adminAccountService *account.AdminAccountService,
	memberService *organization.MemberService,
	siteOpsService *site.SiteOperationsService,
	sshKeyService *organization.SshKeyService,
	firewallService *organization.FirewallService,
	projectFirewallService *project.ProjectFirewallService,
	siteFirewallService *site.SiteFirewallService,
	projectMemberService *project.ProjectMemberService,
	siteMemberService *site.SiteMemberService,
	organizationSecretService *organization.OrganizationSecretService,
	projectSecretService *project.ProjectSecretService,
	siteSecretService *site.SiteSecretService,
) {
	mux.Handle(libopsv1connect.NewOrganizationServiceHandler(organizationService, opts...))
	mux.Handle(libopsv1connect.NewProjectServiceHandler(projectService, opts...))
	mux.Handle(libopsv1connect.NewSiteServiceHandler(siteService, opts...))

	// Register AccountService with rate limiting by authenticated user
	accountServicePath, accountServiceHandler := libopsv1connect.NewAccountServiceHandler(accountService, opts...)
	mux.Handle(accountServicePath, accountLookupRateLimiter.LimitByUser(accountServiceHandler))

	mux.Handle(libopsv1connect.NewAdminOrganizationServiceHandler(adminOrganizationService, opts...))
	mux.Handle(libopsv1connect.NewAdminProjectServiceHandler(adminProjectService, opts...))
	mux.Handle(libopsv1connect.NewAdminSiteServiceHandler(adminSiteService, opts...))
	mux.Handle(libopsv1connect.NewAdminAccountServiceHandler(adminAccountService, opts...))

	mux.Handle(libopsv1connect.NewMemberServiceHandler(memberService, opts...))
	mux.Handle(libopsv1connect.NewProjectMemberServiceHandler(projectMemberService, opts...))
	mux.Handle(libopsv1connect.NewSiteMemberServiceHandler(siteMemberService, opts...))
	mux.Handle(libopsv1connect.NewSiteOperationsServiceHandler(siteOpsService, opts...))
	mux.Handle(libopsv1connect.NewSshKeyServiceHandler(sshKeyService, opts...))
	mux.Handle(libopsv1connect.NewFirewallServiceHandler(firewallService, opts...))
	mux.Handle(libopsv1connect.NewProjectFirewallServiceHandler(projectFirewallService, opts...))
	mux.Handle(libopsv1connect.NewSiteFirewallServiceHandler(siteFirewallService, opts...))

	mux.Handle(libopsv1connect.NewOrganizationSecretServiceHandler(organizationSecretService, opts...))
	mux.Handle(libopsv1connect.NewProjectSecretServiceHandler(projectSecretService, opts...))
	mux.Handle(libopsv1connect.NewSiteSecretServiceHandler(siteSecretService, opts...))
}

// registerReflection adds gRPC reflection endpoints.
func registerReflection(mux *http.ServeMux) {
	reflector := grpcreflect.NewStaticReflector(
		"libops.v1.OrganizationService",
		"libops.v1.ProjectService",
		"libops.v1.SiteService",
		"libops.v1.AccountService",
		"libops.v1.AdminOrganizationService",
		"libops.v1.AdminProjectService",
		"libops.v1.AdminSiteService",
		"libops.v1.AdminAccountService",
		"libops.v1.MemberService",
		"libops.v1.ProjectMemberService",
		"libops.v1.SiteMemberService",
		"libops.v1.SiteOperationsService",
		"libops.v1.SshKeyService",
		"libops.v1.FirewallService",
		"libops.v1.ProjectFirewallService",
		"libops.v1.SiteFirewallService",
		"libops.v1.OrganizationSecretService",
		"libops.v1.ProjectSecretService",
		"libops.v1.SiteSecretService",
	)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))
}

// registerUtilityRoutes adds health, version, and documentation routes.
func registerUtilityRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("/health", handleHealth)

	mux.HandleFunc("/robots.txt", handleRobotsTxt)
	mux.HandleFunc("/version", handleVersion)
	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/openapi.yaml", handlePublicOpenAPISpec)

	// Static files
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "./web/static"
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
}

// registerDashboardRoutes adds dashboard and UI endpoints.
func registerDashboardRoutes(mux *http.ServeMux, dashHandler *dash.Handler, onboardMW *onboard.Middleware) {
	// Public route (no onboarding required)
	mux.HandleFunc("/login", dashHandler.HandleLoginPage)

	// Protected routes (require onboarding completion)
	mux.Handle("/dashboard", onboardMW.RequireOnboardingComplete(http.HandlerFunc(dashHandler.HandleDashboard)))
	mux.Handle("/api-keys", onboardMW.RequireOnboardingComplete(http.HandlerFunc(dashHandler.HandleAPIKeys)))
	mux.Handle("/ssh-keys", onboardMW.RequireOnboardingComplete(http.HandlerFunc(dashHandler.HandleSSHKeys)))
	mux.Handle("/organizations", onboardMW.RequireOnboardingComplete(http.HandlerFunc(dashHandler.HandleOrganizations)))
	mux.Handle("/projects", onboardMW.RequireOnboardingComplete(http.HandlerFunc(dashHandler.HandleProjects)))
	mux.Handle("/sites", onboardMW.RequireOnboardingComplete(http.HandlerFunc(dashHandler.HandleSites)))
	mux.Handle("/secrets", onboardMW.RequireOnboardingComplete(http.HandlerFunc(dashHandler.HandleSecrets)))
	mux.Handle("/firewall", onboardMW.RequireOnboardingComplete(http.HandlerFunc(dashHandler.HandleFirewall)))
	mux.Handle("/members", onboardMW.RequireOnboardingComplete(http.HandlerFunc(dashHandler.HandleMembers)))

	// Detail pages for individual resources (require onboarding completion)
	mux.Handle("GET /organizations/{id}", onboardMW.RequireOnboardingComplete(http.HandlerFunc(dashHandler.HandleOrganizationDetail)))
	mux.Handle("GET /projects/{id}", onboardMW.RequireOnboardingComplete(http.HandlerFunc(dashHandler.HandleProjectDetail)))
	mux.Handle("GET /sites/{id}", onboardMW.RequireOnboardingComplete(http.HandlerFunc(dashHandler.HandleSiteDetail)))
}

// registerOnboardingRoutes adds onboarding endpoints.
func registerOnboardingRoutes(mux *http.ServeMux, handler *onboard.Handler, stripeMgr *billing.StripeManager) {
	// Onboarding page (requires authentication but not onboarding completion)
	mux.HandleFunc("GET /onboarding", handler.RenderOnboarding)

	// Onboarding API endpoints
	mux.HandleFunc("GET /api/onboarding/session", handler.HandleGetSession)
	mux.HandleFunc("POST /api/onboarding/step1", handler.HandleStep1)
	mux.HandleFunc("POST /api/onboarding/step2", handler.HandleStep2)
	mux.HandleFunc("GET /onboarding/stripe/success", handler.HandleStripeSuccess)
	mux.HandleFunc("GET /onboarding/stripe/cancel", handler.HandleStripeCancel)
	mux.HandleFunc("POST /api/onboarding/step4", handler.HandleStep4)
	mux.HandleFunc("POST /api/onboarding/step5", handler.HandleStep5)
	mux.HandleFunc("POST /api/onboarding/step6", handler.HandleStep6)
	mux.HandleFunc("POST /api/onboarding/step7", handler.HandleStep7)

	// Utility endpoints
	mux.HandleFunc("GET /api/onboarding/client-ip", handler.HandleGetClientIP)
	mux.HandleFunc("GET /api/onboarding/regions", handler.HandleGetRegions)

	// Stripe webhook (no authentication required, verified by signature)
	mux.HandleFunc("POST /webhooks/stripe", stripeMgr.HandleStripeWebhook)
}

// registerAuthRoutes adds authentication endpoints.
func registerAuthRoutes(mux *http.ServeMux, authHandler *auth.Handler) {
	// OAuth routes via Goth
	mux.HandleFunc("GET /auth/google", authHandler.HandleGoogleLoginV2)          // Google OAuth
	mux.HandleFunc("GET /auth/github", authHandler.HandleGitHubLogin)            // GitHub OAuth
	mux.HandleFunc("GET /auth/callback/google", authHandler.HandleOAuthCallback) // Google callback
	mux.HandleFunc("GET /auth/callback/github", authHandler.HandleOAuthCallback) // GitHub callback

	// Common auth routes
	mux.HandleFunc("/logout", authHandler.HandleLogout)
	mux.HandleFunc("/auth/me", authHandler.HandleMe)
	mux.HandleFunc("GET /auth/verify", authHandler.HandleVerifyEmail) // Email verification endpoint
}

// registerUserpassRoutes adds userpass authentication endpoints.
func registerUserpassRoutes(mux *http.ServeMux, userpassClient *auth.UserpassClient, authHandler *auth.Handler, authLimiter *RateLimiter) {
	// Userpass login
	mux.HandleFunc("POST /auth/userpass/login", authHandler.HandleUserpassLogin)
	// Userpass registration and verification
	mux.HandleFunc("POST /auth/userpass/register", userpassClient.HandleRegister)
	mux.Handle("POST /auth/userpass/resend-verification", authLimiter.LimitByIP(http.HandlerFunc(userpassClient.HandleResendVerification)))
}

// registerControllerRoutes adds controller reconciliation endpoints.
func registerControllerRoutes(mux *http.ServeMux, queries db.Querier, adminSiteService *site.AdminSiteService, adminProjectService *project.AdminProjectService) {
	gsaAuth := auth.NewGSAMiddleware(queries)

	// SSH keys endpoints - protected by GSA authentication
	mux.Handle("GET /v1/projects/{projectId}/ssh-keys", gsaAuth.Middleware(http.HandlerFunc(adminProjectService.HandleProjectSshKeys)))
	mux.Handle("GET /v1/sites/{siteId}/ssh-keys", gsaAuth.Middleware(http.HandlerFunc(adminSiteService.HandleSiteSshKeys)))
}

// HTTP Handlers

func handleRobotsTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`User-agent: *
Disallow: /`))
}

// handleHealth responds to health check requests.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// handleVersion responds with the API version.
func handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"version":"v1.0.0","api":"connectrpc"}`))
}

// handleOrganizationOpenAPISpec serves the organization-specific OpenAPI specification file.
func handlePublicOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	http.ServeFile(w, r, "./openapi/openapi.yaml")
}
