package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	optionsv1 "github.com/libops/api/proto/libops/v1/options"
)

// AuditInterceptor creates a Connect interceptor that logs CUD operations.
type AuditInterceptor struct {
	auditLogger        *Logger
	accountIDExtractor AccountIDExtractor
}

// AccountIDExtractor is a function that extracts account ID from context
// This is injected to avoid import cycles with the auth package.
type AccountIDExtractor func(ctx context.Context) (int64, bool)

// NewAuditInterceptor creates a new audit interceptor.
func NewAuditInterceptor(auditLogger *Logger, accountIDExtractor AccountIDExtractor) *AuditInterceptor {
	return &AuditInterceptor{
		auditLogger:        auditLogger,
		accountIDExtractor: accountIDExtractor,
	}
}

// WrapUnary wraps unary RPCs with audit logging.
func (i *AuditInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		resp, err := next(ctx, req)

		// Only audit successful operations
		if err != nil {
			return resp, err
		}

		auditInfo := i.getAuditInfo(req.Spec().Procedure)
		if auditInfo == nil {
			return resp, nil
		}

		accountID, ok := i.accountIDExtractor(ctx)
		if !ok {
			// No user context, skip audit (might be an internal call)
			return resp, nil
		}

		entityID := i.extractEntityID(resp.Any(), auditInfo.entityType)
		if entityID == 0 {
			slog.Warn("failed to extract entity ID for audit logging",
				"procedure", req.Spec().Procedure,
				"entity_type", auditInfo.entityType)
			return resp, nil
		}

		auditData := i.createAuditData(req.Any())

		i.auditLogger.Log(ctx, accountID, entityID, auditInfo.entityType, auditInfo.event, auditData)

		return resp, nil
	}
}

// WrapStreamingClient wraps client streaming RPCs.
func (i *AuditInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next // No audit logging for streaming for now
}

// WrapStreamingHandler wraps server streaming RPCs.
func (i *AuditInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next // No audit logging for streaming for now
}

// auditInfo holds information about what to audit for a procedure.
type auditInfo struct {
	entityType EntityType
	event      Event
}

// getAuditInfo maps RPC procedures to audit configuration.
func (i *AuditInterceptor) getAuditInfo(procedure string) *auditInfo {
	switch {
	case strings.HasSuffix(procedure, "AdminOrganizationService/CreateOrganization"):
		return &auditInfo{entityType: OrganizationEntityType, event: OrganizationCreate}
	case strings.HasSuffix(procedure, "AdminOrganizationService/UpdateOrganization"):
		return &auditInfo{entityType: OrganizationEntityType, event: OrganizationUpdate}
	case strings.HasSuffix(procedure, "AdminOrganizationService/DeleteOrganization"):
		return &auditInfo{entityType: OrganizationEntityType, event: OrganizationDelete}

	case strings.HasSuffix(procedure, "AdminProjectService/CreateProject"):
		return &auditInfo{entityType: ProjectEntityType, event: ProjectCreate}
	case strings.HasSuffix(procedure, "AdminProjectService/UpdateProject"):
		return &auditInfo{entityType: ProjectEntityType, event: ProjectUpdate}
	case strings.HasSuffix(procedure, "AdminProjectService/DeleteProject"):
		return &auditInfo{entityType: ProjectEntityType, event: ProjectDelete}

	case strings.HasSuffix(procedure, "AdminSiteService/CreateSite"):
		return &auditInfo{entityType: SiteEntityType, event: SiteCreate}
	case strings.HasSuffix(procedure, "AdminSiteService/UpdateSite"):
		return &auditInfo{entityType: SiteEntityType, event: SiteUpdate}
	case strings.HasSuffix(procedure, "AdminSiteService/DeleteSite"):
		return &auditInfo{entityType: SiteEntityType, event: SiteDelete}

	case strings.HasSuffix(procedure, "AdminAccountService/CreateAccount"):
		return &auditInfo{entityType: AccountEntityType, event: AccountCreate}
	case strings.HasSuffix(procedure, "AdminAccountService/UpdateAccount"):
		return &auditInfo{entityType: AccountEntityType, event: AccountUpdate}
	case strings.HasSuffix(procedure, "AdminAccountService/DeleteAccount"):
		return &auditInfo{entityType: AccountEntityType, event: AccountDelete}

	case strings.HasSuffix(procedure, "SshKeyService/CreateSshKey"):
		return &auditInfo{entityType: SSHKeyEntityType, event: SSHKeyCreate}
	case strings.HasSuffix(procedure, "SshKeyService/DeleteSshKey"):
		return &auditInfo{entityType: SSHKeyEntityType, event: SSHKeyDelete}

	default:
		return nil // Not a CUD operation we want to audit
	}
}

// extractEntityID extracts the entity ID from the response message.
func (i *AuditInterceptor) extractEntityID(msg any, entityType EntityType) int64 {
	protoMsg, ok := msg.(proto.Message)
	if !ok {
		return 0
	}

	// Use protobuf reflection to find ID fields
	// For now, we'll use a simple approach - look for common ID field names
	// In the future, this could be made configurable per entity type
	reflection := protoMsg.ProtoReflect()
	descriptor := reflection.Descriptor()

	idFieldNames := []string{"id", "internal_id", "entity_id"}

	for _, fieldName := range idFieldNames {
		field := descriptor.Fields().ByTextName(fieldName)
		if field != nil && reflection.Has(field) {
			value := reflection.Get(field)
			if value.IsValid() {
				return value.Int()
			}
		}
	}

	return 0
}

// createAuditData creates audit data map from request message
// It uses protobuf reflection to identify fields marked as sensitive via proto options.
func (i *AuditInterceptor) createAuditData(msg any) map[string]any {
	data := make(map[string]any)

	protoMsg, ok := msg.(proto.Message)
	if !ok {
		return data
	}

	sensitiveFieldNames := i.getSensitiveFieldNames(protoMsg)

	jsonBytes, err := json.Marshal(protoMsg)
	if err != nil {
		slog.Error("failed to marshal request for audit", "err", err)
		return data
	}

	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		slog.Error("failed to unmarshal request for audit", "err", err)
		return data
	}

	if len(sensitiveFieldNames) > 0 {
		data = i.redactProtoSensitiveFields(data, sensitiveFieldNames)
	}

	return data
}

// getSensitiveFieldNames extracts field names marked with (libops.v1.options.sensitive) = true.
func (i *AuditInterceptor) getSensitiveFieldNames(msg proto.Message) map[string]bool {
	sensitiveFields := make(map[string]bool)

	reflection := msg.ProtoReflect()
	descriptor := reflection.Descriptor()
	fields := descriptor.Fields()

	for j := 0; j < fields.Len(); j++ {
		field := fields.Get(j)
		opts := field.Options()

		if opts != nil && proto.HasExtension(opts, optionsv1.E_Sensitive) {
			sensitive := proto.GetExtension(opts, optionsv1.E_Sensitive).(bool)
			if sensitive {
				jsonName := field.JSONName()
				if jsonName == "" {
					jsonName = string(field.Name())
				}
				sensitiveFields[jsonName] = true
			}
		}
	}

	return sensitiveFields
}

// redactProtoSensitiveFields redacts fields marked as sensitive in proto definitions.
func (i *AuditInterceptor) redactProtoSensitiveFields(data map[string]any, sensitiveFields map[string]bool) map[string]any {
	redacted := make(map[string]any)

	for key, value := range data {
		if sensitiveFields[key] {
			redacted[key] = "[REDACTED]"
		} else if nestedMap, ok := value.(map[string]any); ok {
			redacted[key] = i.redactProtoSensitiveFields(nestedMap, sensitiveFields)
		} else if nestedArray, ok := value.([]any); ok {
			redactedArray := make([]any, len(nestedArray))
			for idx, item := range nestedArray {
				if itemMap, ok := item.(map[string]any); ok {
					redactedArray[idx] = i.redactProtoSensitiveFields(itemMap, sensitiveFields)
				} else {
					redactedArray[idx] = item
				}
			}
			redacted[key] = redactedArray
		} else {
			redacted[key] = value
		}
	}

	return redacted
}
