package organization

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/service"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
)

// Repository encapsulates shared organization business logic.
type Repository struct {
	db db.Querier
}

// NewRepository creates a new organization repository.
func NewRepository(querier db.Querier) *Repository {
	return &Repository{db: querier}
}

// GetOrganizationByPublicID retrieves a organization by public ID.
func (r *Repository) GetOrganizationByPublicID(ctx context.Context, publicID uuid.UUID) (db.GetOrganizationRow, error) {
	organization, err := r.db.GetOrganization(ctx, publicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetOrganizationRow{}, connect.NewError(
				connect.CodeNotFound,
				fmt.Errorf("organization with ID '%s' not found", publicID.String()),
			)
		}
		return db.GetOrganizationRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return organization, nil
}

// GetOrganizationByInternalID retrieves a organization by internal ID.
func (r *Repository) GetOrganizationByInternalID(ctx context.Context, id int64) (db.GetOrganizationByIDRow, error) {
	organization, err := r.db.GetOrganizationByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetOrganizationByIDRow{}, connect.NewError(
				connect.CodeNotFound,
				fmt.Errorf("organization not found"),
			)
		}
		return db.GetOrganizationByIDRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return organization, nil
}

// CreateOrganization creates a new organization.
func (r *Repository) CreateOrganization(ctx context.Context, params db.CreateOrganizationParams) error {
	err := r.db.CreateOrganization(ctx, params)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) {
			if mysqlErr.Number == 1062 {
				return connect.NewError(
					connect.CodeAlreadyExists,
					fmt.Errorf("a organization with this name already exists"),
				)
			}
		}
		return connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return nil
}

// UpdateOrganization updates a organization.
func (r *Repository) UpdateOrganization(ctx context.Context, params db.UpdateOrganizationParams) error {
	err := r.db.UpdateOrganization(ctx, params)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return nil
}

// DeleteOrganization deletes a organization.
func (r *Repository) DeleteOrganization(ctx context.Context, publicID uuid.UUID) error {
	err := r.db.DeleteOrganization(ctx, publicID.String())
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) {
			if mysqlErr.Number == 1451 {
				return connect.NewError(
					connect.CodeFailedPrecondition,
					fmt.Errorf("cannot delete organization: organization has associated projects"),
				)
			}
		}
		return connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return nil
}

// ListOrganizations lists all organizations with pagination.
func (r *Repository) ListOrganizations(ctx context.Context, params db.ListOrganizationsParams) ([]db.ListOrganizationsRow, error) {
	organizations, err := r.db.ListOrganizations(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return organizations, nil
}

// ListOrganizationProjects lists projects for a organization.
func (r *Repository) ListOrganizationProjects(ctx context.Context, params db.ListOrganizationProjectsParams) ([]db.ListOrganizationProjectsRow, error) {
	projects, err := r.db.ListOrganizationProjects(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return projects, nil
}

// Helper functions

// toNullString converts a string to a sql.NullString, setting Valid to false if the string is empty.
func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// FromNullString extracts the string value from a sql.NullString, returning an empty string if not valid.
func FromNullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// FromNullStringPtr converts a sql.NullString to an optional proto field (*string).
func FromNullStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// ptrToString converts a *string to a string, returning an empty string if the pointer is nil.
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Status conversion helpers.
func DbOrganizationStatusToProto(status db.NullOrganizationsStatus) commonv1.Status {
	return service.DbOrganizationStatusToProto(status)
}
