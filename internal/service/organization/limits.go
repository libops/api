package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
)

const (
	// MaxOrganizationsPerUser is the hardcoded limit for organizations per user
	MaxOrganizationsPerUser = 3
)

// ValidateOrganizationLimit checks if user can create a new organization
func (r *Repository) ValidateOrganizationLimit(ctx context.Context, accountID int64) error {
	count, err := r.db.CountUserOrganizations(ctx, accountID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to count user organizations: %w", err))
	}

	if count >= MaxOrganizationsPerUser {
		return connect.NewError(
			connect.CodeResourceExhausted,
			fmt.Errorf("organization limit reached: you can create up to %d organizations", MaxOrganizationsPerUser),
		)
	}

	return nil
}
