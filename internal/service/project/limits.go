package project

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"connectrpc.com/connect"

	"github.com/libops/api/db"
)

const (
	// DefaultMaxProjectsPerOrg is the default limit if no setting exists
	DefaultMaxProjectsPerOrg = 10
)

// GetMaxProjectsForOrganization retrieves the max_projects setting for an organization
func (r *Repository) GetMaxProjectsForOrganization(ctx context.Context, organizationID int64) (int, error) {
	setting, err := r.db.GetOrganizationSetting(ctx, db.GetOrganizationSettingParams{
		OrganizationID: organizationID,
		SettingKey:     "max_projects",
	})

	if err != nil {
		// If setting doesn't exist (sql.ErrNoRows), return default
		if err == sql.ErrNoRows {
			return DefaultMaxProjectsPerOrg, nil
		}
		// For other errors, log but return default
		return DefaultMaxProjectsPerOrg, nil
	}

	// Parse string value to int
	maxProjects, err := strconv.Atoi(setting.SettingValue)
	if err != nil {
		// If parsing fails, return default
		return DefaultMaxProjectsPerOrg, nil
	}

	return maxProjects, nil
}

// ValidateProjectLimit checks if organization can create a new project
func (r *Repository) ValidateProjectLimit(ctx context.Context, organizationID int64) error {
	maxProjects, err := r.GetMaxProjectsForOrganization(ctx, organizationID)
	if err != nil {
		return err
	}

	count, err := r.db.CountOrganizationProjects(ctx, organizationID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to count organization projects: %w", err))
	}

	if count >= int64(maxProjects) {
		return connect.NewError(
			connect.CodeResourceExhausted,
			fmt.Errorf("project limit reached: this organization can have up to %d projects", maxProjects),
		)
	}

	return nil
}
