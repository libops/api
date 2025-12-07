// Package gcp provides utilities for interacting with Google Cloud Platform services.
package gcp

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/libops/api/internal/db"
)

// ServiceAccountManager handles creation and management of platform service accounts.
type ServiceAccountManager struct {
	db db.Querier
}

// NewServiceAccountManager creates a new service account manager.
func NewServiceAccountManager(querier db.Querier) *ServiceAccountManager {
	return &ServiceAccountManager{
		db: querier,
	}
}

// CreatePlatformServiceAccount creates the platform service account for a organization
// This creates:
// 1. An account in the accounts table with auth_method='gcloud'
// 2. A organization_member entry with role='owner'
//
// The actual GCP service account creation is handled by Terraform.
func (m *ServiceAccountManager) CreatePlatformServiceAccount(
	ctx context.Context,
	organizationID int64,
	projectID string,
	createdBy int64,
) (*db.GetAccountByEmailRow, error) {
	serviceAccountEmail := GetPlatformServiceAccountEmail(projectID)

	slog.Info("Creating platform service account",
		"organization_id", organizationID,
		"project_id", projectID,
		"email", serviceAccountEmail)

	existingAccount, err := m.db.GetAccountByEmail(ctx, serviceAccountEmail)
	if err == nil {
		slog.Info("Platform service account already exists", "account_id", existingAccount.ID)
		return &existingAccount, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check for existing account: %w", err)
	}

	now := sql.NullTime{Time: time.Now(), Valid: true}
	err = m.db.CreateAccount(ctx, db.CreateAccountParams{
		Email:          serviceAccountEmail,
		Name:           sql.NullString{String: "LibOps Platform Service Account", Valid: true},
		GithubUsername: sql.NullString{Valid: false},
		VaultEntityID:  sql.NullString{Valid: false}, // Service accounts don't use Vault entities
		AuthMethod:     "gcloud",
		Verified:       true,
		VerifiedAt:     now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create service account: %w", err)
	}

	// Get the created account
	account, err := m.db.GetAccountByEmail(ctx, serviceAccountEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve created account: %w", err)
	}

	err = m.db.CreateOrganizationMember(ctx, db.CreateOrganizationMemberParams{
		OrganizationID: organizationID,
		AccountID:      account.ID,
		Role:           db.OrganizationMembersRoleOwner,
		CreatedBy:      sql.NullInt64{Int64: createdBy, Valid: true},
		UpdatedBy:      sql.NullInt64{Int64: createdBy, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create organization member: %w", err)
	}

	slog.Info("Platform service account created successfully",
		"account_id", account.ID,
		"organization_id", organizationID)

	return &account, nil
}

// GetPlatformServiceAccountForOrganization retrieves the platform service account for a organization.
func (m *ServiceAccountManager) GetPlatformServiceAccountForOrganization(
	ctx context.Context,
	organizationPublicID uuid.UUID,
) (*db.GetAccountByEmailRow, error) {
	organization, err := m.db.GetOrganization(ctx, organizationPublicID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	if !organization.GcpProjectID.Valid || organization.GcpProjectID.String == "" {
		return nil, fmt.Errorf("organization does not have a GCP project ID")
	}

	serviceAccountEmail := GetPlatformServiceAccountEmail(organization.GcpProjectID.String)
	account, err := m.db.GetAccountByEmail(ctx, serviceAccountEmail)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("platform service account not found for organization")
		}
		return nil, fmt.Errorf("failed to get service account: %w", err)
	}

	return &account, nil
}

// IsServiceAccountForOrganization checks if the given account is the platform service account for a organization.
func (m *ServiceAccountManager) IsServiceAccountForOrganization(
	ctx context.Context,
	accountEmail string,
	organizationID int64,
) (bool, error) {
	organization, err := m.db.GetOrganizationByID(ctx, organizationID)
	if err != nil {
		return false, fmt.Errorf("organization not found: %w", err)
	}

	if !organization.GcpProjectID.Valid || organization.GcpProjectID.String == "" {
		return false, nil
	}

	expectedEmail := GetPlatformServiceAccountEmail(organization.GcpProjectID.String)
	return accountEmail == expectedEmail && IsPlatformServiceAccount(accountEmail), nil
}
