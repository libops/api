package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/service"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
)

// Repository encapsulates shared project business logic.
type Repository struct {
	db db.Querier
}

// NewRepository creates a new project repository.
func NewRepository(querier db.Querier) *Repository {
	return &Repository{db: querier}
}

// GetProjectByPublicID retrieves a project by public ID.
func (r *Repository) GetProjectByPublicID(ctx context.Context, publicID uuid.UUID) (db.GetProjectRow, error) {
	project, err := r.db.GetProject(ctx, publicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetProjectRow{}, connect.NewError(
				connect.CodeNotFound,
				fmt.Errorf("project with ID '%s' not found", publicID.String()),
			)
		}
		return db.GetProjectRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return project, nil
}

// GetProjectWithOrganizationByPublicID retrieves a project with organization billing info by public ID.
func (r *Repository) GetProjectWithOrganizationByPublicID(ctx context.Context, publicID uuid.UUID) (db.GetProjectWithOrganizationRow, error) {
	project, err := r.db.GetProjectWithOrganization(ctx, publicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetProjectWithOrganizationRow{}, connect.NewError(
				connect.CodeNotFound,
				fmt.Errorf("project with ID '%s' not found", publicID.String()),
			)
		}
		return db.GetProjectWithOrganizationRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return project, nil
}

// CreateProject creates a new project.
func (r *Repository) CreateProject(ctx context.Context, params db.CreateProjectParams) error {
	err := r.db.CreateProject(ctx, params)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) {
			if mysqlErr.Number == 1062 {
				return connect.NewError(
					connect.CodeAlreadyExists,
					fmt.Errorf("a project with this name already exists"),
				)
			}
		}
		return connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return nil
}

// UpdateProject updates a project.
func (r *Repository) UpdateProject(ctx context.Context, params db.UpdateProjectParams) error {
	err := r.db.UpdateProject(ctx, params)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return nil
}

// DeleteProject deletes a project.
func (r *Repository) DeleteProject(ctx context.Context, publicID uuid.UUID) error {
	err := r.db.DeleteProject(ctx, publicID.String())
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) {
			if mysqlErr.Number == 1451 {
				return connect.NewError(
					connect.CodeFailedPrecondition,
					fmt.Errorf("cannot delete project: project has associated sites"),
				)
			}
		}
		return connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return nil
}

// ListOrganizationProjects lists projects for a organization.
func (r *Repository) ListOrganizationProjects(ctx context.Context, params db.ListOrganizationProjectsParams) ([]db.ListOrganizationProjectsRow, error) {
	projects, err := r.db.ListOrganizationProjects(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return projects, nil
}

// ListUserProjects lists projects for a user.
func (r *Repository) ListUserProjects(ctx context.Context, params db.ListUserProjectsParams) ([]db.ListUserProjectsRow, error) {
	projects, err := r.db.ListUserProjects(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return projects, nil
}

// ListProjectSites lists sites for a project.
func (r *Repository) ListProjectSites(ctx context.Context, params db.ListProjectSitesParams) ([]db.ListProjectSitesRow, error) {
	sites, err := r.db.ListProjectSites(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return sites, nil
}

// GetOrganizationByPublicID retrieves a organization by public ID (for project creation).
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

// Helper functions

// toNullString converts a string to a sql.NullString, setting Valid to false if the string is empty.
func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// toNullInt32 converts an int32 to a sql.NullInt32, setting Valid to false if the int32 is 0.
func toNullInt32(i int32) sql.NullInt32 {
	if i == 0 {
		return sql.NullInt32{Valid: false}
	}
	return sql.NullInt32{Int32: i, Valid: true}
}

// FromNullString extracts the string value from a sql.NullString, returning an empty string if not valid.
func FromNullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// FromNullStringPtr converts a sql.NullString to an optional pointer to a string, returning nil if not valid.
func FromNullStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// fromNullInt32 extracts the int32 value from a sql.NullInt32, returning 0 if not valid.
func fromNullInt32(ni sql.NullInt32) int32 {
	if ni.Valid {
		return ni.Int32
	}
	return 0
}

// ptrToString converts a *string to a string, returning an empty string if the pointer is nil.
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
func DbProjectStatusToProto(status db.NullProjectsStatus) commonv1.Status {
	return service.DbProjectStatusToProto(status)
}
