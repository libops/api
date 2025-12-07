// Package router sets up and configures the HTTP router and all API endpoints.
package router

import (
	"embed"
	"html/template"
	"log/slog"
	"net/http"

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
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/events"
	"github.com/libops/api/internal/middleware"
	"github.com/libops/api/internal/service/account"
	"github.com/libops/api/internal/service/organization"
	"github.com/libops/api/internal/service/project"
	"github.com/libops/api/internal/service/site"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// Dependencies holds all the dependencies needed to create routes.
type Dependencies struct {
	Queries           db.Querier
	Emitter           *events.Emitter
	Authorizer        *auth.Authorizer
	JWTValidator      auth.JWTValidator
	LibopsTokenIssuer *auth.LibopsTokenIssuer
	APIKeyManager     *auth.APIKeyManager
	AuthHandler       *auth.Handler
	UserpassClient    *auth.UserpassClient
	AllowedOrigins    []string
}

// swaggerTemplateFS embeds the swagger.html.tmpl file for serving API documentation.
//
//go:embed templates/swagger.html.tmpl
var swaggerTemplateFS embed.FS

// New creates a new HTTP handler with all routes configured.
func New(deps *Dependencies) http.Handler {
	mux := http.NewServeMux()

	// Per-route rate limiters (these apply IN ADDITION to the global limiter)
	// The global limiter (100 req/min) will be applied to all routes in the middleware chain
	// These per-route limiters add stricter limits where needed
	authLimiter := NewRateLimiter(rate.Limit(20), 50)   // 20 rps, burst 50 (auth endpoints)
	apiKeyLimiter := NewRateLimiter(rate.Limit(10), 50) // 10 rps, burst 50 (API key management)

	accountService := account.NewAccountService(deps.Queries, deps.APIKeyManager)
	adminAccountService := account.NewAdminAccountService(deps.Queries, deps.Emitter)

	organizationService := organization.NewOrganizationService(deps.Queries)
	adminOrganizationService := organization.NewAdminOrganizationService(deps.Queries)
	memberService := organization.NewMemberService(deps.Queries)
	firewallService := organization.NewFirewallService(deps.Queries)
	sshKeyService := organization.NewSshKeyService(deps.Queries)

	projectService := project.NewProjectService(deps.Queries)
	adminProjectService := project.NewAdminProjectService(deps.Queries)
	projectMemberService := project.NewProjectMemberService(deps.Queries)
	projectFirewallService := project.NewProjectFirewallService(deps.Queries)

	siteService := site.NewSiteService(deps.Queries)
	adminSiteService := site.NewAdminSiteService(deps.Queries)
	siteMemberService := site.NewSiteMemberService(deps.Queries)
	siteFirewallService := site.NewSiteFirewallService(deps.Queries)
	siteOpsService := site.NewSiteOperationsService(deps.Queries)

	var handlerOptions []connect.HandlerOption

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		slog.Error("Failed to create OpenTelemetry interceptor", "err", err)
	} else {
		handlerOptions = append(handlerOptions, connect.WithInterceptors(otelInterceptor))
	}

	auditLogger := audit.New(deps.Queries)

	organizationSecretService := organization.NewOrganizationSecretService(deps.Queries, auditLogger)
	projectSecretService := project.NewProjectSecretService(deps.Queries, auditLogger)
	siteSecretService := site.NewSiteSecretService(deps.Queries, auditLogger)
	auditInterceptor := audit.NewAuditInterceptor(auditLogger, auth.ExtractAccountIDFromContext)
	handlerOptions = append(handlerOptions, connect.WithInterceptors(auditInterceptor))

	accountLookupRateLimiter := NewRateLimiter(10, 20)

	if deps.Authorizer != nil {
		// First interceptor: Check if scope matches exactly (for API keys)
		scopeAuthzInterceptor := auth.NewScopeAuthzInterceptor(deps.Authorizer, auditLogger)
		handlerOptions = append(handlerOptions, connect.WithInterceptors(scopeAuthzInterceptor))

		// Second interceptor: Check RBAC based on hierarchical permissions
		rbacAuthzInterceptor := auth.NewRBACAuthzInterceptor(deps.Authorizer, auditLogger)
		handlerOptions = append(handlerOptions, connect.WithInterceptors(rbacAuthzInterceptor))
	}

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

	if deps.AuthHandler != nil {
		registerAuthRoutes(mux, deps.AuthHandler)
	}
	if deps.LibopsTokenIssuer != nil {
		// Token endpoint
		mux.Handle("POST /auth/token", authLimiter.LimitByIP(http.HandlerFunc(deps.LibopsTokenIssuer.HandleToken)))
	}
	if deps.APIKeyManager != nil {
		registerAPIKeyRoutes(mux, deps.APIKeyManager, apiKeyLimiter) // API key management
	}
	if deps.UserpassClient != nil {
		registerUserpassRoutes(mux, deps.UserpassClient, deps.AuthHandler, authLimiter) // Userpass auth routes
	}

	// Create global rate limiter (100 requests per minute per IP)
	globalRateLimiter := NewRateLimiter(rate.Limit(100), 100)

	var handler http.Handler = mux

	// Apply request ID middleware first
	handler = middleware.RequestIDMiddleware(handler)

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

	// Version
	mux.HandleFunc("/version", handleVersion)

	// Metrics
	mux.Handle("/metrics", promhttp.Handler())

	// OpenAPI specs
	mux.HandleFunc("/openapi.yaml", handlePublicOpenAPISpec)
	mux.HandleFunc("/openapi-public.yaml", handlePublicOpenAPISpec)
	mux.HandleFunc("/openapi-admin.yaml", handleAdminOpenAPISpec)

	// API documentation homepages
	mux.HandleFunc("/openapi", handlePublicOpenAPISpec)
	mux.HandleFunc("/admin", handleAdminDocs)

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
}

// registerAuthRoutes adds authentication endpoints.
func registerAuthRoutes(mux *http.ServeMux, authHandler *auth.Handler) {
	mux.HandleFunc("/login", authHandler.HandleHome)          // Unified login/register page
	mux.HandleFunc("/dashboard", authHandler.HandleDashboard) // User dashboard
	mux.HandleFunc("/auth/login", authHandler.HandleLogin)
	mux.HandleFunc("/auth/google", authHandler.HandleGoogleLogin) // Simplified Google login
	mux.HandleFunc("/auth/callback", authHandler.HandleCallback)
	mux.HandleFunc("/auth/logout", authHandler.HandleLogout)
	mux.HandleFunc("/auth/me", authHandler.HandleMe)
	mux.HandleFunc("GET /auth/verify", authHandler.HandleVerifyEmail) // Email verification endpoint
}

// registerAPIKeyRoutes adds API key management endpoints.
func registerAPIKeyRoutes(mux *http.ServeMux, apiKeyMgr *auth.APIKeyManager, apiKeyLimiter *RateLimiter) {
	// API key management (requires authentication)
	mux.Handle("POST /auth/apikeys", apiKeyLimiter.LimitByUser(http.HandlerFunc(apiKeyMgr.HandleCreateAPIKey)))
	mux.HandleFunc("GET /auth/apikeys", apiKeyMgr.HandleListAPIKeys)
	mux.HandleFunc("DELETE /auth/apikeys/{key_id}", apiKeyMgr.HandleDeleteAPIKey)
}

// registerUserpassRoutes adds userpass authentication endpoints.
func registerUserpassRoutes(mux *http.ServeMux, userpassClient *auth.UserpassClient, authHandler *auth.Handler, authLimiter *RateLimiter) {
	// Userpass login
	mux.HandleFunc("POST /auth/userpass/login", authHandler.HandleUserpassLogin)
	// Userpass registration and verification
	mux.HandleFunc("POST /auth/userpass/register", userpassClient.HandleRegister)
	mux.Handle("POST /auth/userpass/resend-verification", authLimiter.LimitByIP(http.HandlerFunc(userpassClient.HandleResendVerification)))
}

// HTTP Handlers

// handleHealth responds to health check requests.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// handleVersion responds with the API version.
func handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"version":"v2.0.0","api":"connectrpc"}`))
}

// handleOrganizationOpenAPISpec serves the organization-specific OpenAPI specification file.
func handlePublicOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	http.ServeFile(w, r, "./openapi/openapi-organization.yaml")
}

// handleAdminOpenAPISpec serves the admin-specific OpenAPI specification file.
func handleAdminOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	http.ServeFile(w, r, "./openapi/openapi-admin.yaml")
}

// handleAdminDocs serves the API documentation homepage for administrators.
func handleAdminDocs(w http.ResponseWriter, r *http.Request) {
	swagger(w, "/openapi-admin.yaml")
}

// swagger renders the Swagger UI page with the given OpenAPI specification URI.
func swagger(w http.ResponseWriter, route string) {
	tmpl, err := template.ParseFS(swaggerTemplateFS, "templates/swagger.html.tmpl")
	if err != nil {
		slog.Error("Error parsing swagger template", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := struct {
		OpenApiYamlUri string
	}{
		OpenApiYamlUri: route,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		slog.Error("Error executing swagger template", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
