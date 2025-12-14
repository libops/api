// Package project provides services related to project management, including firewall rule management.
package project

import (
	"context"
	"database/sql"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/service"
	"github.com/libops/api/internal/service/organization"
	"github.com/libops/api/internal/validation"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// ProjectFirewallService implements the LibOps ProjectFirewallService API.
type ProjectFirewallService struct {
	db db.Querier
}

// Compile-time check.
var _ libopsv1connect.ProjectFirewallServiceHandler = (*ProjectFirewallService)(nil)

// NewProjectFirewallService creates a new ProjectFirewallService instance.
func NewProjectFirewallService(querier db.Querier) *ProjectFirewallService {
	return &ProjectFirewallService{
		db: querier,
	}
}

// ListProjectFirewallRules lists firewall rules for a project.
func (s *ProjectFirewallService) ListProjectFirewallRules(
	ctx context.Context,
	req *connect.Request[libopsv1.ListProjectFirewallRulesRequest],
) (*connect.Response[libopsv1.ListProjectFirewallRulesResponse], error) {
	projectID := req.Msg.ProjectId
	if err := validation.UUID(projectID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	project, err := service.GetProjectByPublicID(ctx, s.db, projectID)
	if err != nil {
		return nil, err
	}

	rules, err := s.db.ListProjectFirewallRules(ctx, sql.NullInt64{Int64: project.ID, Valid: true})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	protoRules := make([]*libopsv1.ProjectFirewallRule, 0, len(rules))
	for _, rule := range rules {
		protoRules = append(protoRules, &libopsv1.ProjectFirewallRule{
			RuleId:    rule.PublicID, // Use public_id UUID, not internal integer ID
			ProjectId: projectID,
			RuleType:  organization.ConvertFirewallRuleTypeToProto(string(rule.RuleType)),
			Cidr:      rule.Cidr,
			Name:      rule.Name,
			Status:    service.DbProjectFirewallRuleStatusToProto(rule.Status),
		})
	}

	return connect.NewResponse(&libopsv1.ListProjectFirewallRulesResponse{
		Rules:         protoRules,
		NextPageToken: "",
	}), nil
}

// CreateProjectFirewallRule creates a new firewall rule for a project.
func (s *ProjectFirewallService) CreateProjectFirewallRule(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateProjectFirewallRuleRequest],
) (*connect.Response[libopsv1.CreateProjectFirewallRuleResponse], error) {
	projectID := req.Msg.ProjectId
	if err := validation.UUID(projectID); err != nil {
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

	project, err := service.GetProjectByPublicID(ctx, s.db, projectID)
	if err != nil {
		return nil, err
	}

	params := db.CreateProjectFirewallRuleParams{
		ProjectID: sql.NullInt64{Int64: project.ID, Valid: true},
		Name:      req.Msg.Name,
		RuleType:  db.ProjectFirewallRulesRuleType(organization.ConvertProtoFirewallRuleTypeToString(req.Msg.RuleType)),
		Cidr:      req.Msg.Cidr,
	}

	err = s.db.CreateProjectFirewallRule(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	rule := &libopsv1.ProjectFirewallRule{
		RuleId:    "0",
		ProjectId: projectID,
		RuleType:  req.Msg.RuleType,
		Cidr:      req.Msg.Cidr,
		Name:      req.Msg.Name,
	}

	return connect.NewResponse(&libopsv1.CreateProjectFirewallRuleResponse{
		Rule: rule,
	}), nil
}

// DeleteProjectFirewallRule deletes a firewall rule from a project.
func (s *ProjectFirewallService) DeleteProjectFirewallRule(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteProjectFirewallRuleRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectID := req.Msg.ProjectId
	ruleID := req.Msg.RuleId

	if err := validation.UUID(projectID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.UUID(ruleID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid rule_id: %w", err))
	}

	err := s.db.DeleteProjectFirewallRuleByPublicID(ctx, ruleID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}
