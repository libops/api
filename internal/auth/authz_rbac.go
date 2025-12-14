package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/libops/api/internal/audit"
	optionsv1 "github.com/libops/api/proto/libops/v1/options"
)

// RBACAuthzInterceptor enforces role-based access control (RBAC) using hierarchical permissions.
// This interceptor checks if the user has the required hierarchical permission based on their
// role membership in organizations, projects, or sites.
type RBACAuthzInterceptor struct {
	authorizer  *Authorizer
	auditLogger *audit.Logger
}

// NewRBACAuthzInterceptor creates a new RBAC authorization interceptor.
func NewRBACAuthzInterceptor(authorizer *Authorizer, auditLogger *audit.Logger) *RBACAuthzInterceptor {
	return &RBACAuthzInterceptor{
		authorizer:  authorizer,
		auditLogger: auditLogger,
	}
}

// WrapUnary wraps unary RPCs with RBAC authorization.
func (i *RBACAuthzInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx = context.WithValue(ctx, authorizerKey, i.authorizer)

		userInfo, ok := GetUserFromContext(ctx)
		if !ok {
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
		}

		// Extract scope rule from method annotations to get resource information
		scopeRule, err := i.extractScopeRule(req.Spec().Procedure)
		if err != nil {
			slog.Error("Failed to extract scope rule for RBAC check",
				"procedure", req.Spec().Procedure,
				"error", err)
			i.auditLogger.Log(ctx, userInfo.AccountID, 0, audit.AccountEntityType, audit.AuthorizationFailure, map[string]any{
				"error":     "failed to extract scope rule for RBAC",
				"procedure": req.Spec().Procedure,
			})
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("authorization configuration error"))
		}

		// If no scope rule is defined, no RBAC check is needed
		if scopeRule == nil {
			slog.Debug("No scope rule defined for endpoint, skipping RBAC check",
				"procedure", req.Spec().Procedure)
			return next(ctx, req)
		}

		// Check membership if resource_id_field or parent_resource_id_field is specified
		if err := i.checkMembership(ctx, req, scopeRule, userInfo); err != nil {
			slog.Error("RBAC membership check failed",
				"account_id", userInfo.AccountID,
				"email", userInfo.Email,
				"procedure", req.Spec().Procedure,
				"error", err)

			entityType := resourceTypeToEntityType(scopeRule.Resource)
			i.auditLogger.Log(ctx, userInfo.AccountID, 0, entityType, audit.AuthorizationFailure, map[string]any{
				"error":     "RBAC membership check failed",
				"details":   err.Error(),
				"procedure": req.Spec().Procedure,
			})
			// Return NotFound instead of PermissionDenied to avoid leaking resource IDs
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("resource not found"))
		}

		slog.Debug("RBAC check passed",
			"email", userInfo.Email,
			"procedure", req.Spec().Procedure)

		return next(ctx, req)
	}
}

// WrapStreamingClient wraps client streaming RPCs.
func (i *RBACAuthzInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		ctx = context.WithValue(ctx, authorizerKey, i.authorizer)
		return next(ctx, spec)
	}
}

// WrapStreamingHandler wraps server streaming RPCs.
func (i *RBACAuthzInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx = context.WithValue(ctx, authorizerKey, i.authorizer)
		return next(ctx, conn)
	}
}

// ContextKey for storing authorizer in context.
type contextKey string

const authorizerKey contextKey = "authorizer"

// GetAuthorizer retrieves the authorizer from context
// Service methods should use this to get the authorizer for permission checks.
func GetAuthorizer(ctx context.Context) (*Authorizer, error) {
	auth, ok := ctx.Value(authorizerKey).(*Authorizer)
	if !ok {
		return nil, fmt.Errorf("authorizer not found in context")
	}
	return auth, nil
}

// WithAuthorizer adds the authorizer to the context
// Used primarily for testing.
func WithAuthorizer(ctx context.Context, auth *Authorizer) context.Context {
	return context.WithValue(ctx, authorizerKey, auth)
}

// extractScopeRule extracts the required_scope annotation from a method.
func (i *RBACAuthzInterceptor) extractScopeRule(procedure string) (*optionsv1.ScopeRule, error) {
	// ConnectRPC procedures look like: /libops.v1.OrganizationService/GetOrganization
	// We need to convert this to the proto descriptor name
	procedure = strings.TrimPrefix(procedure, "/")
	procedure = strings.ReplaceAll(procedure, "/", ".")

	// Find the method descriptor
	methodDesc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(procedure))
	if err != nil {
		// Method not found in registry - this indicates a configuration error or invalid procedure name
		slog.Error("Method not found in proto registry for RBAC check, denying access",
			"procedure", procedure,
			"error", err)
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("authorization configuration error: method not found"))
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

// toCamelCase converts snake_case to camelCase (e.g., "organization_id" -> "organizationId")
func toCamelCase(s string) string {
	if s == "" {
		return s
	}
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// checkMembership checks if the user is a member of the resource specified in the request.
func (i *RBACAuthzInterceptor) checkMembership(ctx context.Context, req connect.AnyRequest, scopeRule *optionsv1.ScopeRule, userInfo *UserInfo) error {
	// No membership check needed if neither field is specified
	if scopeRule.ResourceIdField == "" && scopeRule.ParentResourceIdField == "" {
		return nil
	}

	bodyBytes, ok := GetRequestMessageAsJSON(ctx)
	if !ok {
		// No request body, skip membership check
		return nil
	}

	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return fmt.Errorf("failed to parse request body: %w", err)
	}

	slog.Info("RBAC checkMembership", "body", string(bodyBytes))

	var resourceIDStr string
	var resourceType optionsv1.ResourceType
	var permission Permission

	if scopeRule.ParentResourceIdField != "" {
		// For create operations, check parent resource
		// Convert snake_case to camelCase for JSON lookup (protobuf uses camelCase in JSON)
		fieldName := toCamelCase(scopeRule.ParentResourceIdField)
		resourceIDStr, _ = body[fieldName].(string)
		resourceType = scopeRule.ParentResource
	} else if scopeRule.ResourceIdField != "" {
		// For other operations, check the resource itself
		// Convert snake_case to camelCase for JSON lookup (protobuf uses camelCase in JSON)
		fieldName := toCamelCase(scopeRule.ResourceIdField)
		resourceIDStr, _ = body[fieldName].(string)
		resourceType = scopeRule.Resource
	}

	if resourceIDStr == "" {
		// No resource ID in request, skip membership check
		return nil
	}

	resourceID, err := uuid.Parse(resourceIDStr)
	if err != nil {
		return fmt.Errorf("invalid resource ID format: %w", err)
	}

	switch scopeRule.Level {
	case optionsv1.AccessLevel_ACCESS_LEVEL_READ:
		permission = PermissionRead
	case optionsv1.AccessLevel_ACCESS_LEVEL_WRITE:
		permission = PermissionWrite
	case optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN:
		permission = PermissionOwner
	default:
		permission = PermissionRead
	}

	switch resourceType {
	case optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION:
		return i.authorizer.CheckOrganizationAccess(ctx, userInfo, resourceID, permission)
	case optionsv1.ResourceType_RESOURCE_TYPE_PROJECT:
		return i.authorizer.CheckProjectAccess(ctx, userInfo, resourceID, permission)
	case optionsv1.ResourceType_RESOURCE_TYPE_SITE:
		return i.authorizer.CheckSiteAccess(ctx, userInfo, resourceID, permission)
	case optionsv1.ResourceType_RESOURCE_TYPE_ACCOUNT:
		// Account resources don't need membership check
		return nil
	default:
		// Unknown resource type, allow (fail open for compatibility)
		slog.Warn("Unknown resource type for RBAC membership check",
			"resource_type", resourceType,
			"resource_id", resourceID)
		return nil
	}
}
