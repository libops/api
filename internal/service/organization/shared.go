package organization

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"github.com/libops/api/db"
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

// AddMember adds a member to a organization.
func (r *Repository) AddMember(ctx context.Context, params db.CreateOrganizationMemberParams) error {
	err := r.db.CreateOrganizationMember(ctx, params)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return nil
}

// CreateRelationship creates a relationship between organizations.
func (r *Repository) CreateRelationship(ctx context.Context, params db.CreateRelationshipParams) (sql.Result, error) {
	result, err := r.db.CreateRelationship(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}
	return result, nil
}

// CreateOrganizationWithOwner creates an organization, adds the creator as owner (with active status),
// and optionally creates a relationship with a root organization.
// Returns the created organization's internal ID.
func (r *Repository) CreateOrganizationWithOwner(
	ctx context.Context,
	orgPublicID string,
	orgName string,
	gcpOrgID string,
	gcpBillingAccount string,
	gcpParent string,
	accountID int64,
	rootOrgID int64, // 0 means no root org relationship
) (int64, error) {
	// Create organization
	orgParams := db.CreateOrganizationParams{
		PublicID:          orgPublicID,
		Name:              orgName,
		GcpOrgID:          gcpOrgID,
		GcpBillingAccount: gcpBillingAccount,
		GcpParent:         gcpParent,
		GcpFolderID:       sql.NullString{Valid: false},
		Status:            db.NullOrganizationsStatus{OrganizationsStatus: db.OrganizationsStatusProvisioning, Valid: true},
		CreatedBy:         sql.NullInt64{Int64: accountID, Valid: true},
		UpdatedBy:         sql.NullInt64{Int64: accountID, Valid: true},
	}

	if err := r.CreateOrganization(ctx, orgParams); err != nil {
		return 0, err
	}

	// Get the created organization to retrieve its internal ID
	createdOrg, err := r.GetOrganizationByPublicID(ctx, uuid.MustParse(orgPublicID))
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve created organization: %w", err)
	}

	// Add the creator as an owner with active status
	// The owner is immediately active since they're creating the organization
	memberParams := db.CreateOrganizationMemberParams{
		OrganizationID: createdOrg.ID,
		AccountID:      accountID,
		Role:           db.OrganizationMembersRoleOwner,
		Status:         db.NullOrganizationMembersStatus{OrganizationMembersStatus: db.OrganizationMembersStatusActive, Valid: true},
		CreatedBy:      sql.NullInt64{Int64: accountID, Valid: true},
		UpdatedBy:      sql.NullInt64{Int64: accountID, Valid: true},
	}
	if err := r.AddMember(ctx, memberParams); err != nil {
		return 0, fmt.Errorf("failed to add creator as organization owner: %w", err)
	}

	// Create relationship with root organization if specified
	if rootOrgID > 0 {
		relationshipParams := db.CreateRelationshipParams{
			SourceOrganizationID: rootOrgID,
			TargetOrganizationID: createdOrg.ID,
			RelationshipType:     db.RelationshipsRelationshipTypeAccess,
		}
		if _, err := r.CreateRelationship(ctx, relationshipParams); err != nil {
			return 0, fmt.Errorf("failed to create relationship: %w", err)
		}
	}

	// Create default organization settings
	defaultSettings := []struct {
		key         string
		value       string
		editable    bool
		description string
	}{
		{
			key:         "max_projects",
			value:       "10",
			editable:    false,
			description: "Maximum number of projects allowed in this organization",
		},
	}

	for _, setting := range defaultSettings {
		settingPublicID := uuid.New().String()
		err := r.db.CreateOrganizationSetting(ctx, db.CreateOrganizationSettingParams{
			PublicID:       settingPublicID,
			OrganizationID: createdOrg.ID,
			SettingKey:     setting.key,
			SettingValue:   setting.value,
			Editable:       sql.NullBool{Bool: setting.editable, Valid: true},
			Description:    sql.NullString{String: setting.description, Valid: true},
			Status:         db.NullOrganizationSettingsStatus{OrganizationSettingsStatus: db.OrganizationSettingsStatusActive, Valid: true},
			CreatedBy:      sql.NullInt64{Int64: accountID, Valid: true},
			UpdatedBy:      sql.NullInt64{Int64: accountID, Valid: true},
		})
		if err != nil {
			// Log but don't fail org creation if setting creation fails
			slog.Warn("Failed to create default organization setting", "setting", setting.key, "error", err)
		}
	}

	return createdOrg.ID, nil
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
