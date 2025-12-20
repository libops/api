package organization

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/db"
	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/service"
	"github.com/libops/api/internal/validation"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// FirewallService implements the LibOps FirewallService API.
type FirewallService struct {
	db db.Querier
}

// Compile-time check.
var _ libopsv1connect.FirewallServiceHandler = (*FirewallService)(nil)

// NewFirewallService creates a new FirewallService instance.
func NewFirewallService(querier db.Querier) *FirewallService {
	return &FirewallService{
		db: querier,
	}
}

// ListOrganizationFirewallRules lists firewall rules for a organization.
func (s *FirewallService) ListOrganizationFirewallRules(
	ctx context.Context,
	req *connect.Request[libopsv1.ListOrganizationFirewallRulesRequest],
) (*connect.Response[libopsv1.ListOrganizationFirewallRulesResponse], error) {
	organizationID := req.Msg.OrganizationId

	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	organization, err := service.GetOrganizationByPublicID(ctx, s.db, organizationID)
	if err != nil {
		return nil, err
	}

	rules, err := s.db.ListOrganizationFirewallRules(ctx, sql.NullInt64{Int64: organization.ID, Valid: true})
	if err != nil {
		slog.Error("Failed to list organization firewall rules", "error", err, "organization_id", organization.ID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	protoRules := make([]*libopsv1.OrganizationFirewallRule, 0, len(rules))
	for _, rule := range rules {
		protoRules = append(protoRules, &libopsv1.OrganizationFirewallRule{
			RuleId:         rule.PublicID, // Use public_id UUID, not internal integer ID
			OrganizationId: organizationID,
			RuleType:       ConvertFirewallRuleTypeToProto(string(rule.RuleType)),
			Cidr:           rule.Cidr,
			Name:           rule.Name,
			Status:         service.DbOrganizationFirewallRuleStatusToProto(rule.Status),
		})
	}

	// Include firewall rules from related organizations if the caller has access
	relatedRules, err := s.getRelatedOrganizationFirewallRules(ctx, organization.ID)
	if err != nil {
		// Log error but don't fail the request
		// We still return the org's own rules
		fmt.Printf("Warning: failed to fetch related org rules: %v\n", err)
	} else {
		protoRules = append(protoRules, relatedRules...)
	}

	return connect.NewResponse(&libopsv1.ListOrganizationFirewallRulesResponse{
		Rules:         protoRules,
		NextPageToken: "",
	}), nil
}

// CreateOrganizationFirewallRule creates a new firewall rule for a organization.
func (s *FirewallService) CreateOrganizationFirewallRule(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateOrganizationFirewallRuleRequest],
) (*connect.Response[libopsv1.CreateOrganizationFirewallRuleResponse], error) {
	organizationID := req.Msg.OrganizationId
	slog.Debug("CreateOrganizationFirewallRule called",
		"organization_id", organizationID,
		"cidr", req.Msg.Cidr,
		"rule_type", req.Msg.RuleType)

	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.CIDR(req.Msg.Cidr); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.CIDR(req.Msg.Cidr); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.RequiredString("name", req.Msg.Name); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.StringLength("name", req.Msg.Name, 1, 255); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if req.Msg.RuleType == libopsv1.FirewallRuleType_FIREWALL_RULE_TYPE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rule_type is required"))
	}

	organization, err := service.GetOrganizationByPublicID(ctx, s.db, organizationID)
	if err != nil {
		return nil, err
	}

	params := db.CreateOrganizationFirewallRuleParams{
		OrganizationID: sql.NullInt64{Int64: organization.ID, Valid: true},
		RuleType:       db.OrganizationFirewallRulesRuleType(ConvertProtoFirewallRuleTypeToString(req.Msg.RuleType)),
		Cidr:           req.Msg.Cidr,
		Name:           req.Msg.Name,
	}

	err = s.db.CreateOrganizationFirewallRule(ctx, params)
	if err != nil {
		slog.Error("Failed to create firewall rule in DB", "error", err, "params", params)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Note: We'd need to query back to get the ID, but for now return the input
	rule := &libopsv1.OrganizationFirewallRule{
		RuleId:         "0", // Would need to query back to get actual ID
		OrganizationId: organizationID,
		RuleType:       req.Msg.RuleType,
		Cidr:           req.Msg.Cidr,
		Name:           req.Msg.Name,
	}

	return connect.NewResponse(&libopsv1.CreateOrganizationFirewallRuleResponse{
		Rule: rule,
	}), nil
}

// DeleteOrganizationFirewallRule deletes a firewall rule from a organization.
func (s *FirewallService) DeleteOrganizationFirewallRule(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteOrganizationFirewallRuleRequest],
) (*connect.Response[emptypb.Empty], error) {
	organizationID := req.Msg.OrganizationId
	ruleID := req.Msg.RuleId

	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.UUID(ruleID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid rule_id: %w", err))
	}

	err := s.db.DeleteOrganizationFirewallRuleByPublicID(ctx, ruleID)
	if err != nil {
		slog.Error("Failed to delete organization firewall rule", "error", err, "rule_id", ruleID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// Helper functions

// ConvertFirewallRuleTypeToProto converts a database firewall rule type string to its protobuf representation.
func ConvertFirewallRuleTypeToProto(dbType string) libopsv1.FirewallRuleType {
	switch dbType {
	case "https_allowed":
		return libopsv1.FirewallRuleType_FIREWALL_RULE_TYPE_HTTPS_ALLOWED
	case "ssh_allowed":
		return libopsv1.FirewallRuleType_FIREWALL_RULE_TYPE_SSH_ALLOWED
	case "blocked":
		return libopsv1.FirewallRuleType_FIREWALL_RULE_TYPE_BLOCKED
	default:
		return libopsv1.FirewallRuleType_FIREWALL_RULE_TYPE_UNSPECIFIED
	}
}

// ConvertProtoFirewallRuleTypeToString converts a protobuf firewall rule type to its database string representation.
func ConvertProtoFirewallRuleTypeToString(protoType libopsv1.FirewallRuleType) string {
	switch protoType {
	case libopsv1.FirewallRuleType_FIREWALL_RULE_TYPE_HTTPS_ALLOWED:
		return "https_allowed"
	case libopsv1.FirewallRuleType_FIREWALL_RULE_TYPE_SSH_ALLOWED:
		return "ssh_allowed"
	case libopsv1.FirewallRuleType_FIREWALL_RULE_TYPE_BLOCKED:
		return "blocked"
	default:
		return ""
	}
}

// getRelatedOrganizationFirewallRules fetches firewall rules from related organizations
// that the caller has access to.
func (s *FirewallService) getRelatedOrganizationFirewallRules(
	ctx context.Context,
	sourceOrgID int64,
) ([]*libopsv1.OrganizationFirewallRule, error) {
	// Get account ID from context
	accountID, ok := auth.ExtractAccountIDFromContext(ctx)
	if !ok {
		// No account in context, skip related rules
		return nil, nil
	}

	// Get approved relationships where the user has access to the target org
	relationships, err := s.db.ListApprovedRelatedOrganizationsForAccount(ctx, db.ListApprovedRelatedOrganizationsForAccountParams{
		AccountID:            accountID,
		SourceOrganizationID: sourceOrgID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list related organizations: %w", err)
	}

	var allRelatedRules []*libopsv1.OrganizationFirewallRule

	// For each related org, fetch its firewall rules
	for _, rel := range relationships {
		rules, err := s.db.ListOrganizationFirewallRules(ctx, sql.NullInt64{Int64: rel.TargetOrganizationID, Valid: true})
		if err != nil {
			// Log error but continue with other related orgs
			fmt.Printf("Warning: failed to fetch rules for related org %d: %v\n", rel.TargetOrganizationID, err)
			continue
		}

		for _, rule := range rules {
			allRelatedRules = append(allRelatedRules, &libopsv1.OrganizationFirewallRule{
				RuleId:         rule.PublicID,         // Use public_id UUID, not internal integer ID
				OrganizationId: rel.TargetOrgPublicID, // Use the related org's public ID
				RuleType:       ConvertFirewallRuleTypeToProto(string(rule.RuleType)),
				Cidr:           rule.Cidr,
				Name:           rule.Name,
				Status:         service.DbOrganizationFirewallRuleStatusToProto(rule.Status),
			})
		}
	}

	return allRelatedRules, nil
}
