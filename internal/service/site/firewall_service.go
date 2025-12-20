package site

import (
	"context"
	"database/sql"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/db"
	"github.com/libops/api/internal/service"
	"github.com/libops/api/internal/service/organization"
	"github.com/libops/api/internal/validation"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// SiteFirewallService implements the LibOps SiteFirewallService API.
type SiteFirewallService struct {
	repo *Repository
}

// Compile-time check.
var _ libopsv1connect.SiteFirewallServiceHandler = (*SiteFirewallService)(nil)

// NewSiteFirewallService creates a new SiteFirewallService instance.
func NewSiteFirewallService(querier db.Querier) *SiteFirewallService {
	return &SiteFirewallService{
		repo: NewRepository(querier),
	}
}

// ListSiteFirewallRules lists firewall rules for a site.
func (s *SiteFirewallService) ListSiteFirewallRules(
	ctx context.Context,
	req *connect.Request[libopsv1.ListSiteFirewallRulesRequest],
) (*connect.Response[libopsv1.ListSiteFirewallRulesResponse], error) {
	siteID := req.Msg.SiteId

	if err := validation.UUID(siteID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	siteUUID, err := uuid.Parse(siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id format: %w", err))
	}

	site, err := s.repo.GetSiteByPublicID(ctx, siteUUID)
	if err != nil {
		return nil, err
	}

	rules, err := s.repo.db.ListSiteFirewallRules(ctx, sql.NullInt64{Int64: site.ID, Valid: true})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	protoRules := make([]*libopsv1.SiteFirewallRule, 0, len(rules))
	for _, rule := range rules {
		protoRules = append(protoRules, &libopsv1.SiteFirewallRule{
			RuleId:   rule.PublicID, // Use public_id UUID, not internal integer ID
			SiteId:   site.PublicID,
			RuleType: organization.ConvertFirewallRuleTypeToProto(string(rule.RuleType)),
			Cidr:     rule.Cidr,
			Name:     rule.Name,
			Status:   service.DbSiteFirewallRuleStatusToProto(rule.Status),
		})
	}

	return connect.NewResponse(&libopsv1.ListSiteFirewallRulesResponse{
		Rules:         protoRules,
		NextPageToken: "",
	}), nil
}

// CreateSiteFirewallRule creates a new firewall rule for a site.
func (s *SiteFirewallService) CreateSiteFirewallRule(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateSiteFirewallRuleRequest],
) (*connect.Response[libopsv1.CreateSiteFirewallRuleResponse], error) {
	siteID := req.Msg.SiteId

	if err := validation.UUID(siteID); err != nil {
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

	siteUUID, err := uuid.Parse(siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id format: %w", err))
	}

	site, err := s.repo.GetSiteByPublicID(ctx, siteUUID)
	if err != nil {
		return nil, err
	}

	params := db.CreateSiteFirewallRuleParams{
		SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
		Name:     req.Msg.Name,
		RuleType: db.SiteFirewallRulesRuleType(organization.ConvertProtoFirewallRuleTypeToString(req.Msg.RuleType)),
		Cidr:     req.Msg.Cidr,
	}

	err = s.repo.db.CreateSiteFirewallRule(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	rule := &libopsv1.SiteFirewallRule{
		RuleId:   "0",
		SiteId:   site.PublicID,
		RuleType: req.Msg.RuleType,
		Cidr:     req.Msg.Cidr,
		Name:     req.Msg.Name,
	}

	return connect.NewResponse(&libopsv1.CreateSiteFirewallRuleResponse{
		Rule: rule,
	}), nil
}

// DeleteSiteFirewallRule deletes a firewall rule from a site.
func (s *SiteFirewallService) DeleteSiteFirewallRule(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteSiteFirewallRuleRequest],
) (*connect.Response[emptypb.Empty], error) {
	siteID := req.Msg.SiteId
	ruleID := req.Msg.RuleId

	if err := validation.UUID(siteID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id: %w", err))
	}

	if err := validation.UUID(ruleID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid rule_id: %w", err))
	}

	err := s.repo.db.DeleteSiteFirewallRuleByPublicID(ctx, ruleID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}
