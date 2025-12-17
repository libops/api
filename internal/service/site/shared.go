package site

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/service"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
)

// Repository encapsulates shared site business logic.
type Repository struct {
	db db.Querier
}

// NewRepository creates a new site repository.
func NewRepository(querier db.Querier) *Repository {
	return &Repository{db: querier}
}

// GetSiteByPublicID retrieves a site by public ID.
func (r *Repository) GetSiteByPublicID(ctx context.Context, publicID uuid.UUID) (db.GetSiteRow, error) {
	site, err := r.db.GetSite(ctx, publicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetSiteRow{}, connect.NewError(
				connect.CodeNotFound,
				fmt.Errorf("site with ID '%s' not found", publicID.String()),
			)
		}
		return db.GetSiteRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return site, nil
}

// GetSiteByProjectAndName retrieves a site by project ID and name.
func (r *Repository) GetSiteByProjectAndName(ctx context.Context, projectID int64, siteName string) (db.GetSiteByProjectAndNameRow, error) {
	site, err := r.db.GetSiteByProjectAndName(ctx, db.GetSiteByProjectAndNameParams{
		ProjectID: projectID,
		Name:      siteName,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetSiteByProjectAndNameRow{}, connect.NewError(
				connect.CodeNotFound,
				fmt.Errorf("site '%s' not found", siteName),
			)
		}
		return db.GetSiteByProjectAndNameRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return site, nil
}

// CreateSite creates a new site.
func (r *Repository) CreateSite(ctx context.Context, params db.CreateSiteParams) error {
	err := r.db.CreateSite(ctx, params)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return nil
}

// UpdateSite updates a site.
func (r *Repository) UpdateSite(ctx context.Context, params db.UpdateSiteParams) error {
	err := r.db.UpdateSite(ctx, params)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return nil
}

// DeleteSite deletes a site.
func (r *Repository) DeleteSite(ctx context.Context, publicID string) error {
	parsedID, err := uuid.Parse(publicID)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site ID format: %w", err))
	}
	err = r.db.DeleteSite(ctx, parsedID.String())
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return nil
}

// ListProjectSites lists sites for a project.
func (r *Repository) ListProjectSites(ctx context.Context, params db.ListProjectSitesParams) ([]db.ListProjectSitesRow, error) {
	sites, err := r.db.ListProjectSites(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return sites, nil
}

// ListUserSites lists sites for a user.
func (r *Repository) ListUserSites(ctx context.Context, params db.ListUserSitesParams) ([]db.ListUserSitesRow, error) {
	sites, err := r.db.ListUserSites(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return sites, nil
}

// GetOrganizationByPublicID retrieves a organization by public ID.
func (r *Repository) GetOrganizationByPublicID(ctx context.Context, publicID uuid.UUID) (db.GetOrganizationRow, error) {
	organization, err := r.db.GetOrganization(ctx, publicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetOrganizationRow{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return db.GetOrganizationRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return organization, nil
}

// GetProjectByPublicID retrieves a project by public ID (for site creation).
func (r *Repository) GetProjectByPublicID(ctx context.Context, publicID uuid.UUID) (db.GetProjectRow, error) {
	project, err := r.db.GetProject(ctx, publicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetProjectRow{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return db.GetProjectRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return project, nil
}

// GetProjectByID retrieves a project by ID.
func (r *Repository) GetProjectByID(ctx context.Context, id int64) (db.GetProjectByIDRow, error) {
	project, err := r.db.GetProjectByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetProjectByIDRow{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return db.GetProjectByIDRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return project, nil
}

// GetOrganizationByID retrieves an organization by ID.
func (r *Repository) GetOrganizationByID(ctx context.Context, id int64) (db.GetOrganizationByIDRow, error) {
	org, err := r.db.GetOrganizationByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetOrganizationByIDRow{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return db.GetOrganizationByIDRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return org, nil
}

// Helper functions

// FromNullStringPtr converts a sql.NullString to an optional pointer to a string, returning nil if not valid.
func FromNullStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// ShouldUpdateField checks if a field should be updated based on the field mask.
// If the field mask is nil or empty, all fields should be updated; otherwise, only fields present in the mask should be updated.
func ShouldUpdateField(mask *fieldmaskpb.FieldMask, field string) bool {
	if mask == nil {
		return true
	}
	for _, path := range mask.Paths {
		if path == field {
			return true
		}
	}
	return false
}

// Status conversion helpers.
func DbSiteStatusToProto(status db.NullSitesStatus) commonv1.Status {
	return service.DbSiteStatusToProto(status)
}
