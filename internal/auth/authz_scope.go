package auth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/libops/api/internal/audit"
	optionsv1 "github.com/libops/api/proto/libops/v1/options"
)

// ScopeAuthzInterceptor enforces scope-based authorization for API keys.
// This interceptor ONLY checks if a scope was passed for an API key.
// If a scope is present, it must exactly match the scope required for the operation.
// If the scope doesn't match, the request fails with 403.
// If no scope is present (OAuth users), the request passes through to the RBAC interceptor.
type ScopeAuthzInterceptor struct {
	authorizer  *Authorizer
	auditLogger *audit.Logger
}

// NewScopeAuthzInterceptor creates a new scope authorization interceptor.
func NewScopeAuthzInterceptor(authorizer *Authorizer, auditLogger *audit.Logger) *ScopeAuthzInterceptor {
	return &ScopeAuthzInterceptor{
		authorizer:  authorizer,
		auditLogger: auditLogger,
	}
}

// WrapUnary wraps unary RPCs with scope validation.
func (i *ScopeAuthzInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx = StoreRequestMessage(ctx, req)

		userInfo, ok := GetUserFromContext(ctx)
		if !ok {
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
		}

		// Check if user is a global admin - admins bypass scope checks
		if i.authorizer.IsAdmin(ctx, userInfo) {
			slog.Debug("Admin access granted, bypassing scope check",
				"email", userInfo.Email,
				"procedure", req.Spec().Procedure)
			return next(ctx, req)
		}

		// Extract scope rule from method annotations
		scopeRule, err := i.extractScopeRule(req.Spec().Procedure)
		if err != nil {
			// If there's an error extracting scope, log it and deny access
			slog.Error("Failed to extract scope rule",
				"procedure", req.Spec().Procedure,
				"error", err)
			i.auditLogger.Log(ctx, userInfo.AccountID, 0, audit.AccountEntityType, audit.AuthorizationFailure, map[string]any{
				"error":     "failed to extract scope rule",
				"procedure": req.Spec().Procedure,
			})
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("authorization configuration error"))
		}

		// If no scope rule is defined, the endpoint doesn't require specific scopes
		// (authentication is still required, which we already checked)
		if scopeRule == nil {
			slog.Debug("No scope rule defined for endpoint, allowing authenticated access",
				"procedure", req.Spec().Procedure)
			return next(ctx, req)
		}

		// System resources require strict checking
		// Users who passed the IsAdmin check above are already allowed.
		// Here we are dealing with non-admins.
		if scopeRule.Resource == optionsv1.ResourceType_RESOURCE_TYPE_SYSTEM {
			if len(userInfo.Scopes) > 0 && HasScope(userInfo.Scopes, scopeRule) {
				slog.Debug("System access granted via scope",
					"email", userInfo.Email,
					"procedure", req.Spec().Procedure)
				return next(ctx, req)
			}

			// OAuth users (no scopes) or API keys without the specific scope are denied
			slog.Warn("System access denied for non-admin",
				"email", userInfo.Email,
				"procedure", req.Spec().Procedure)

			i.auditLogger.Log(ctx, userInfo.AccountID, 0, resourceTypeToEntityType(scopeRule.Resource), audit.AuthorizationFailure, map[string]any{
				"error":     "system access denied",
				"procedure": req.Spec().Procedure,
			})

			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("requires system administrative privileges"))
		}

		// Scopes act as a RESTRICTION for API keys
		// If user has scopes defined (API key with scopes), check if the scope allows this operation
		// If no scopes are defined (OAuth user or API key with empty scopes), allow through to RBAC check
		if len(userInfo.Scopes) > 0 && !HasScope(userInfo.Scopes, scopeRule) {
			slog.Warn("Scope authorization failed",
				"email", userInfo.Email,
				"account_id", userInfo.AccountID,
				"required_scope", fmt.Sprintf("%s:%s", scopeRule.Resource, scopeRule.Level),
				"user_scopes", ScopesToStrings(userInfo.Scopes),
				"procedure", req.Spec().Procedure)

			i.auditLogger.Log(ctx, userInfo.AccountID, 0, resourceTypeToEntityType(scopeRule.Resource), audit.AuthorizationFailure, map[string]any{
				"error":          "insufficient scopes",
				"required_scope": fmt.Sprintf("%s:%s", scopeRule.Resource, scopeRule.Level),
				"user_scopes":    ScopesToStrings(userInfo.Scopes),
				"procedure":      req.Spec().Procedure,
			})

			return nil, connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("insufficient permissions: requires %s:%s",
					resourceTypeToString(scopeRule.Resource),
					accessLevelToString(scopeRule.Level)))
		}

		// Scope check passed (or no scopes to check)
		// Proceed to the next interceptor (RBAC)
		if len(userInfo.Scopes) > 0 {
			slog.Debug("Scope restriction check passed, proceeding to RBAC checks",
				"email", userInfo.Email,
				"required_scope", fmt.Sprintf("%s:%s", scopeRule.Resource, scopeRule.Level),
				"procedure", req.Spec().Procedure)
		} else {
			slog.Debug("No scope restrictions, proceeding to RBAC checks",
				"email", userInfo.Email,
				"procedure", req.Spec().Procedure)
		}

		return next(ctx, req)
	}
}

// WrapStreamingClient wraps client streaming RPCs.
func (i *ScopeAuthzInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	}
}

// WrapStreamingHandler wraps server streaming RPCs.
func (i *ScopeAuthzInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(ctx, conn)
	}
}

// extractScopeRule extracts the required_scope annotation from a method.
func (i *ScopeAuthzInterceptor) extractScopeRule(procedure string) (*optionsv1.ScopeRule, error) {
	// ConnectRPC procedures look like: /libops.v1.OrganizationService/GetOrganization
	// We need to convert this to the proto descriptor name
	procedure = strings.TrimPrefix(procedure, "/")
	procedure = strings.ReplaceAll(procedure, "/", ".")

	// Find the method descriptor
	methodDesc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(procedure))
	if err != nil {
		// Method not found in registry - this could be okay for dynamically registered services
		// Return nil scope (no requirement) rather than error
		slog.Debug("Method not found in proto registry, allowing access",
			"procedure", procedure)
		return nil, nil
	}

	md, ok := methodDesc.(protoreflect.MethodDescriptor)
	if !ok {
		return nil, fmt.Errorf("descriptor is not a method: %s", procedure)
	}

	methodOpts, ok := md.Options().(*descriptorpb.MethodOptions)
	if !ok {
		// No options defined, no scope requirement
		return nil, nil
	}

	// Extract the required_scope extension
	if !proto.HasExtension(methodOpts, optionsv1.E_RequiredScope) {
		// No scope requirement defined
		return nil, nil
	}

	scopeRule := proto.GetExtension(methodOpts, optionsv1.E_RequiredScope).(*optionsv1.ScopeRule)
	return scopeRule, nil
}

// resourceTypeToEntityType converts proto ResourceType to audit EntityType
func resourceTypeToEntityType(rt optionsv1.ResourceType) audit.EntityType {
	switch rt {
	case optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION:
		return audit.OrganizationEntityType
	case optionsv1.ResourceType_RESOURCE_TYPE_PROJECT:
		return audit.ProjectEntityType
	case optionsv1.ResourceType_RESOURCE_TYPE_SITE:
		return audit.SiteEntityType
	case optionsv1.ResourceType_RESOURCE_TYPE_ACCOUNT:
		return audit.AccountEntityType
	default:
		// Default to account if unknown, as it's the safest valid enum value
		return audit.AccountEntityType
	}
}
