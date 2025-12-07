package site

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/libops/api/internal/db"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// SiteOperationsService implements the LibOps SiteOperationsService API.
type SiteOperationsService struct {
	db db.Querier
}

// Compile-time check.
var _ libopsv1connect.SiteOperationsServiceHandler = (*SiteOperationsService)(nil)

// NewSiteOperationsService creates a new SiteOperationsService instance with DI.
func NewSiteOperationsService(querier db.Querier) *SiteOperationsService {
	return &SiteOperationsService{
		db: querier,
	}
}

// DeploySite triggers a deployment for a site.
func (s *SiteOperationsService) DeploySite(
	ctx context.Context,
	req *connect.Request[libopsv1.DeploySiteRequest],
) (*connect.Response[libopsv1.DeploySiteResponse], error) {
	siteID := req.Msg.SiteId

	if siteID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("site_id is required"))
	}

	siteIDInt, err := strconv.ParseInt(siteID, 10, 64)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id: %w", err))
	}

	_, err = s.db.GetSiteByID(ctx, siteIDInt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get site: %w", err))
	}

	deploymentID := uuid.New().String()

	err = s.db.CreateDeployment(ctx, db.CreateDeploymentParams{
		DeploymentID: deploymentID,
		SiteID:       siteID,
		Status:       "pending",
		GithubRunID:  sql.NullString{Valid: false},
		GithubRunUrl: sql.NullString{Valid: false},
		StartedAt:    0,
		CompletedAt:  sql.NullInt64{Valid: false},
		ErrorMessage: sql.NullString{Valid: false},
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create deployment: %w", err))
	}

	// TODO: Trigger GitHub Actions workflow via API

	return connect.NewResponse(&libopsv1.DeploySiteResponse{
		DeploymentId: deploymentID,
		Status: &libopsv1.SiteStatus{
			SiteId: siteID,
			Status: "deploying",
		},
	}), nil
}

// GetSiteStatus retrieves the current status of a site.
func (s *SiteOperationsService) GetSiteStatus(
	ctx context.Context,
	req *connect.Request[libopsv1.GetSiteStatusRequest],
) (*connect.Response[libopsv1.GetSiteStatusResponse], error) {
	siteID := req.Msg.SiteId

	if siteID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("site_id is required"))
	}

	siteIDInt, err := strconv.ParseInt(siteID, 10, 64)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id: %w", err))
	}

	site, err := s.db.GetSiteByID(ctx, siteIDInt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get site: %w", err))
	}

	// TODO: Get real-time status from GCE instance
	// For now, return status from database

	status := &libopsv1.SiteStatus{
		SiteId: siteID,
		Status: string(site.Status.SitesStatus),
	}

	deployment, err := s.db.GetLatestSiteDeployment(ctx, siteID)
	if err == nil {
		if deployment.CompletedAt.Valid {
			deployedAt := time.Unix(deployment.CompletedAt.Int64, 0).Format(time.RFC3339)
			status.DeployedAt = &deployedAt
		}
		if deployment.ErrorMessage.Valid {
			status.Message = &deployment.ErrorMessage.String
		}
	}

	return connect.NewResponse(&libopsv1.GetSiteStatusResponse{
		Status: status,
	}), nil
}
