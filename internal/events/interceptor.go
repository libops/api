package events

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
)

// EventInterceptor creates a Connect interceptor that emits events for CUD operations.
type EventInterceptor struct {
	emitter *Emitter
}

// NewEventInterceptor creates a new event interceptor.
func NewEventInterceptor(emitter *Emitter) *EventInterceptor {
	return &EventInterceptor{
		emitter: emitter,
	}
}

// WrapUnary wraps unary RPCs with event emission.
func (i *EventInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		resp, err := next(ctx, req)

		// Only emit events for successful operations
		if err != nil {
			return resp, err
		}

		eventType := i.getEventType(req.Spec().Procedure)
		if eventType == "" {
			return resp, nil
		}

		// Skip delete events - services should emit these manually BEFORE deletion
		// to ensure parent IDs can be looked up from the database
		if strings.Contains(eventType, ".deleted.") || strings.Contains(eventType, ".removed.") {
			return resp, nil
		}

		// Skip SSH key events - these are account-scoped, not org/project/site scoped
		// TODO: Implement account-level reconciliation for SSH keys
		if strings.Contains(eventType, ".ssh_key.") {
			return resp, nil
		}

		subject := i.extractSubject(req.Any(), resp.Any())
		// If we can't extract a subject, we still emit the event, but without a subject
		// or maybe we log a warning. For now, let's log a warning but proceed.
		if subject == "" {
			slog.Warn("failed to extract subject for event",
				"procedure", req.Spec().Procedure,
				"event_type", eventType)
		}

		// Emit the event asynchronously to not block the response
		// Note: The Emitter writes to the DB, so it is synchronous with the request transaction if we shared the transaction,
		// but here we are likely using the pool. It should be fast enough.
		// If strict transactional integrity is needed (event only if commit), we'd need to inject the tx from context.
		// For now, we assume the operation is already committed (since we are in the return path of interceptor).

		_, ok := resp.Any().(proto.Message)
		if !ok {
			slog.Error("response is not a proto message", "procedure", req.Spec().Procedure)
			return resp, nil
		}

		// We emit the RESPONSE as the data for the event, as it usually contains the full resource.
		// For DELETE operations, the response is usually empty, so we emit the REQUEST instead.
		var payload proto.Message
		if strings.Contains(eventType, ".deleted.") || strings.Contains(eventType, ".removed.") {
			if reqMsg, ok := req.Any().(proto.Message); ok {
				payload = reqMsg
			}
		} else {
			if respMsg, ok := resp.Any().(proto.Message); ok {
				payload = respMsg
			}
		}

		if payload == nil {
			slog.Error("failed to determine payload for event", "procedure", req.Spec().Procedure)
			return resp, nil
		}

		orgID, projectID, siteID := i.extractContextIDs(req.Any(), resp.Any())

		if err := i.emitter.SendScopedProtoEvent(ctx, eventType, subject, orgID, projectID, siteID, payload); err != nil {
			slog.Error("failed to emit event", "error", err, "event_type", eventType)
		}

		return resp, nil
	}
}

// WrapStreamingClient wraps client streaming RPCs.
func (i *EventInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler wraps server streaming RPCs.
func (i *EventInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// extractContextIDs extracts organization, project, and site IDs from request or response.
func (i *EventInterceptor) extractContextIDs(req, resp any) (orgID, projID, siteID *string) {
	check := func(msg any) {
		if msg == nil {
			return
		}
		protoMsg, ok := msg.(proto.Message)
		if !ok {
			return
		}

		reflection := protoMsg.ProtoReflect()
		descriptor := reflection.Descriptor()

		// Helper to get field value
		getField := func(name string) *string {
			field := descriptor.Fields().ByTextName(name)
			if field != nil && reflection.Has(field) {
				val := reflection.Get(field).String()
				if val != "" {
					return &val
				}
			}
			return nil
		}

		// Check top-level fields first
		if orgID == nil {
			orgID = getField("organization_id")
		}
		if projID == nil {
			projID = getField("project_id")
		}
		if siteID == nil {
			siteID = getField("site_id")
		}

		// Check nested project field (for CreateProjectResponse, etc.)
		if projectField := descriptor.Fields().ByTextName("project"); projectField != nil && reflection.Has(projectField) {
			nestedProj := reflection.Get(projectField).Message().Interface()
			nestedRefl := nestedProj.ProtoReflect()
			nestedDesc := nestedRefl.Descriptor()

			getNestedField := func(name string) *string {
				field := nestedDesc.Fields().ByTextName(name)
				if field != nil && nestedRefl.Has(field) {
					val := nestedRefl.Get(field).String()
					if val != "" {
						return &val
					}
				}
				return nil
			}

			if orgID == nil {
				orgID = getNestedField("organization_id")
			}
			if projID == nil {
				projID = getNestedField("project_id")
			}
		}

		// Check nested site field (for CreateSiteResponse, etc.)
		if siteField := descriptor.Fields().ByTextName("site"); siteField != nil && reflection.Has(siteField) {
			nestedSite := reflection.Get(siteField).Message().Interface()
			nestedRefl := nestedSite.ProtoReflect()
			nestedDesc := nestedRefl.Descriptor()

			getNestedField := func(name string) *string {
				field := nestedDesc.Fields().ByTextName(name)
				if field != nil && nestedRefl.Has(field) {
					val := nestedRefl.Get(field).String()
					if val != "" {
						return &val
					}
				}
				return nil
			}

			if orgID == nil {
				orgID = getNestedField("organization_id")
			}
			if projID == nil {
				projID = getNestedField("project_id")
			}
			if siteID == nil {
				siteID = getNestedField("site_id")
			}
		}
	}

	check(req)
	check(resp)
	return
}

// extractSubject extracts the subject ID from the response or request message.
func (i *EventInterceptor) extractSubject(req, resp any) string {
	check := func(msg any) string {
		if msg == nil {
			return ""
		}
		protoMsg, ok := msg.(proto.Message)
		if !ok {
			return ""
		}

		reflection := protoMsg.ProtoReflect()
		descriptor := reflection.Descriptor()

		idFieldNames := []string{"id", "internal_id", "entity_id", "key_id", "rule_id", "member_id"}

		for _, fieldName := range idFieldNames {
			field := descriptor.Fields().ByTextName(fieldName)
			if field != nil && reflection.Has(field) {
				value := reflection.Get(field)
				if value.IsValid() {
					return fmt.Sprintf("%v", value.Interface())
				}
			}
		}
		return ""
	}

	if id := check(resp); id != "" {
		return id
	}
	return check(req)
}

// getEventType maps RPC procedures to event types.
func (i *EventInterceptor) getEventType(procedure string) string {
	switch {
	// Account
	case strings.HasSuffix(procedure, "AdminAccountService/CreateAccount"):
		return EventTypeAccountCreated
	case strings.HasSuffix(procedure, "AdminAccountService/UpdateAccount"):
		return EventTypeAccountUpdated
	case strings.HasSuffix(procedure, "AdminAccountService/DeleteAccount"):
		return EventTypeAccountDeleted

	// Organization
	case strings.HasSuffix(procedure, "AdminOrganizationService/CreateOrganization") || strings.HasSuffix(procedure, "OrganizationService/CreateOrganization"):
		return EventTypeOrganizationCreated
	case strings.HasSuffix(procedure, "AdminOrganizationService/UpdateOrganization") || strings.HasSuffix(procedure, "OrganizationService/UpdateOrganization"):
		return EventTypeOrganizationUpdated
	case strings.HasSuffix(procedure, "AdminOrganizationService/DeleteOrganization") || strings.HasSuffix(procedure, "OrganizationService/DeleteOrganization"):
		return EventTypeOrganizationDeleted

	// Project
	case strings.HasSuffix(procedure, "AdminProjectService/CreateProject") || strings.HasSuffix(procedure, "ProjectService/CreateProject"):
		return EventTypeProjectCreated
	case strings.HasSuffix(procedure, "AdminProjectService/UpdateProject") || strings.HasSuffix(procedure, "ProjectService/UpdateProject"):
		return EventTypeProjectUpdated
	case strings.HasSuffix(procedure, "AdminProjectService/DeleteProject") || strings.HasSuffix(procedure, "ProjectService/DeleteProject"):
		return EventTypeProjectDeleted

	// Site
	case strings.HasSuffix(procedure, "AdminSiteService/CreateSite") || strings.HasSuffix(procedure, "SiteService/CreateSite"):
		return EventTypeSiteCreated
	case strings.HasSuffix(procedure, "AdminSiteService/UpdateSite") || strings.HasSuffix(procedure, "SiteService/UpdateSite"):
		return EventTypeSiteUpdated
	case strings.HasSuffix(procedure, "AdminSiteService/DeleteSite") || strings.HasSuffix(procedure, "SiteService/DeleteSite"):
		return EventTypeSiteDeleted

	// SSH Key
	case strings.HasSuffix(procedure, "SshKeyService/CreateSshKey"):
		return EventTypeSshKeyCreated
	case strings.HasSuffix(procedure, "SshKeyService/DeleteSshKey"):
		return EventTypeSshKeyDeleted

	// Members
	case strings.HasSuffix(procedure, "MemberService/AddMember"):
		return EventTypeOrganizationMemberAdded
	case strings.HasSuffix(procedure, "MemberService/UpdateMember"):
		return EventTypeOrganizationMemberUpdated
	case strings.HasSuffix(procedure, "MemberService/RemoveMember"):
		return EventTypeOrganizationMemberRemoved
	case strings.HasSuffix(procedure, "ProjectMemberService/AddProjectMember"):
		return EventTypeProjectMemberAdded
	case strings.HasSuffix(procedure, "ProjectMemberService/UpdateProjectMember"):
		return EventTypeProjectMemberUpdated
	case strings.HasSuffix(procedure, "ProjectMemberService/RemoveProjectMember"):
		return EventTypeProjectMemberRemoved
	case strings.HasSuffix(procedure, "SiteMemberService/AddSiteMember"):
		return EventTypeSiteMemberAdded
	case strings.HasSuffix(procedure, "SiteMemberService/UpdateSiteMember"):
		return EventTypeSiteMemberUpdated
	case strings.HasSuffix(procedure, "SiteMemberService/RemoveSiteMember"):
		return EventTypeSiteMemberRemoved

	// Firewall
	case strings.HasSuffix(procedure, "FirewallService/AddFirewallRule"):
		return EventTypeOrganizationFirewallRuleAdded
	case strings.HasSuffix(procedure, "FirewallService/RemoveFirewallRule"):
		return EventTypeOrganizationFirewallRuleRemoved
	case strings.HasSuffix(procedure, "ProjectFirewallService/AddProjectFirewallRule"):
		return EventTypeProjectFirewallRuleAdded
	case strings.HasSuffix(procedure, "ProjectFirewallService/RemoveProjectFirewallRule"):
		return EventTypeProjectFirewallRuleRemoved
	case strings.HasSuffix(procedure, "SiteFirewallService/AddSiteFirewallRule"):
		return EventTypeSiteFirewallRuleAdded
	case strings.HasSuffix(procedure, "SiteFirewallService/RemoveSiteFirewallRule"):
		return EventTypeSiteFirewallRuleRemoved

	// Secrets
	case strings.HasSuffix(procedure, "OrganizationSecretService/CreateSecret"):
		return EventTypeOrganizationSecretCreated
	case strings.HasSuffix(procedure, "OrganizationSecretService/UpdateSecret"):
		return EventTypeOrganizationSecretUpdated
	case strings.HasSuffix(procedure, "OrganizationSecretService/DeleteSecret"):
		return EventTypeOrganizationSecretDeleted

	case strings.HasSuffix(procedure, "ProjectSecretService/CreateProjectSecret"):
		return EventTypeProjectSecretCreated
	case strings.HasSuffix(procedure, "ProjectSecretService/UpdateProjectSecret"):
		return EventTypeProjectSecretUpdated
	case strings.HasSuffix(procedure, "ProjectSecretService/DeleteProjectSecret"):
		return EventTypeProjectSecretDeleted

	case strings.HasSuffix(procedure, "SiteSecretService/CreateSiteSecret"):
		return EventTypeSiteSecretCreated
	case strings.HasSuffix(procedure, "SiteSecretService/UpdateSiteSecret"):
		return EventTypeSiteSecretUpdated
	case strings.HasSuffix(procedure, "SiteSecretService/DeleteSiteSecret"):
		return EventTypeSiteSecretDeleted

	default:
		return ""
	}
}
